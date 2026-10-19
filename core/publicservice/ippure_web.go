package publicservice

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	ippureWebAPIBase     = "https://api.123169.xyz"
	ippureWebBasicPath   = "/api/info/ip-basic/"
	ippureWebRiskPath    = "/api/info/ip-risk/"
	ippureWebIPifyURL    = "https://api.ipify.org?format=json"
	ippureWebBodyLimit   = 16 * 1024
	ippureWebIPBodyLimit = 1024
	ippureNonceKeyLimit  = 256
	ippureMaxClockSkew   = 24 * time.Hour
)

var ippureNonPublicIPv4Ranges = func() []netip.Prefix {
	blocks := []string{
		"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24",
		"192.88.99.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4",
	}
	prefixes := make([]netip.Prefix, 0, len(blocks))
	for _, block := range blocks {
		prefix, err := netip.ParsePrefix(block)
		if err == nil {
			prefixes = append(prefixes, prefix)
		}
	}
	return prefixes
}()

type ippureNonceState struct {
	mu                 sync.Mutex
	key                []byte
	serverTimeMillis   int64
	serverTimeCaptured time.Time
	localTimeBase      time.Time
	requestCount       int
	firstStatus        int
	firstContentType   string
	firstCFChallenge   bool
	protocolError      string
	rechallenged       bool
}

func newIPPureNonceState() *ippureNonceState {
	return &ippureNonceState{localTimeBase: time.Now()}
}

func (s *ippureNonceState) nextRequest() (int, []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.requestCount
	s.requestCount++
	return index, append([]byte(nil), s.key...)
}

func (s *ippureNonceState) nextTimestamp() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.key) > 0 && !s.serverTimeCaptured.IsZero() {
		return s.serverTimeMillis + time.Since(s.serverTimeCaptured).Milliseconds()
	}
	return s.localTimeBase.UnixMilli() + time.Since(s.localTimeBase).Milliseconds()
}

func (s *ippureNonceState) observeResponse(headers http.Header, status int, requestIndex int, signed bool) {
	key := headers.Get("x-k")
	s.mu.Lock()
	defer s.mu.Unlock()
	if requestIndex == 0 {
		s.firstStatus = status
		s.firstContentType = headers.Get("Content-Type")
		s.firstCFChallenge = ippureHasCFChallengeMarker(headers)
	}
	if key == "" {
		return
	}
	if requestIndex != 0 || signed {
		s.rechallenged = true
		return
	}
	if len([]byte(key)) > ippureNonceKeyLimit {
		s.protocolError = "IPPure server nonce exceeds the 256 byte limit"
		return
	}
	serverTimeText := strings.TrimSpace(headers.Get("x-t"))
	serverTime, err := strconv.ParseInt(serverTimeText, 10, 64)
	if err != nil || serverTime <= 0 {
		s.protocolError = "IPPure server clock header is invalid"
		return
	}

	capturedAt := time.Now()
	localMillis := capturedAt.UnixMilli()
	maximumSkewMillis := ippureMaxClockSkew.Milliseconds()
	if serverTime < localMillis-maximumSkewMillis || serverTime > localMillis+maximumSkewMillis {
		s.protocolError = "IPPure server clock differs too far from local time"
		return
	}
	s.key = []byte(key)
	s.serverTimeMillis = serverTime
	s.serverTimeCaptured = capturedAt
}

func (s *ippureNonceState) status() (hasKey bool, protocolError string, rechallenged bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.key) > 0, s.protocolError, s.rechallenged
}

func (s *ippureNonceState) firstNonceResponseAllowsReplay(body []byte) bool {
	s.mu.Lock()
	status := s.firstStatus
	contentType := s.firstContentType
	cfChallenge := s.firstCFChallenge
	s.mu.Unlock()
	if status != http.StatusOK && status != http.StatusUnauthorized && status != http.StatusForbidden || cfChallenge {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || !(strings.EqualFold(mediaType, "application/json") || strings.HasSuffix(strings.ToLower(mediaType), "+json")) {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(body, &object) == nil && object != nil
}

func ippureHasCFChallengeMarker(headers http.Header) bool {
	if strings.EqualFold(strings.TrimSpace(headers.Get("cf-mitigated")), "challenge") {
		return true
	}
	for name := range headers {
		name = strings.ToLower(name)
		if strings.HasPrefix(name, "cf-chl-") || name == "cf-challenge" {
			return true
		}
	}
	return false
}

type ippureNonceTransport struct {
	base  http.RoundTripper
	state *ippureNonceState
}

func (t *ippureNonceTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	index, key := t.state.nextRequest()
	transportRequest := request
	if len(key) > 0 {
		transportRequest = request.Clone(request.Context())
		transportRequest.Header = request.Header.Clone()
		timestamp := strconv.FormatInt(t.state.nextTimestamp(), 10)
		mac := hmac.New(sha256.New, key)
		_, _ = io.WriteString(mac, "GET-"+request.URL.String()+"--"+timestamp)
		transportRequest.Header.Set("x-k", string(key))
		transportRequest.Header.Set("x-t", timestamp+"-"+hex.EncodeToString(mac.Sum(nil)))
	}
	response, err := t.base.RoundTrip(transportRequest)
	if response != nil {
		t.state.observeResponse(response.Header, response.StatusCode, index, len(key) > 0)
	}
	return response, err
}

func (t *ippureNonceTransport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func ippureWebClient(client *http.Client, state *ippureNonceState, signing bool) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	clone := *client
	clone.Jar = nil
	if clone.CheckRedirect == nil {
		clone.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	}
	if signing {
		base := clone.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		clone.Transport = &ippureNonceTransport{base: base, state: state}
	}
	return &clone
}

func checkIPPureWeb(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result := Result{Details: map[string]string{
		"provider": "IPPure", "api_method": "ippure_web_basic_risk_v1", "api_version": "web_v1",
	}}
	unsignedClient := ippureWebClient(client, nil, false)

	ipifyResult, ipifyBody, ok := boundedGET(ctx, unsignedClient, ippureWebIPifyURL, ippureWebIPBodyLimit, "application/json")
	ippureAccumulate(&result, ipifyResult)
	if !ok {
		ippureCarryFailure(&result, ipifyResult)
		if result.Outcome != "cancelled" && result.Outcome != "rate_limited" {
			result.Outcome = "unknown"
			result.Summary = "IPv4 出口未确认；未查询 IPPure"
		}
		return finish(result, started)
	}
	var ipifyPayload struct {
		IP string `json:"ip"`
	}
	if json.Unmarshal(ipifyBody, &ipifyPayload) != nil {
		ippureUnknown(&result, "ipify_response", "IPv4 出口服务未返回可识别的 JSON")
		return finish(result, started)
	}
	freshIP, validIPv4 := ippurePublicIPv4(ipifyPayload.IP)
	if !validIPv4 {
		ippureUnknown(&result, "ip_family", "未取得有效的公开 IPv4 出口地址；IPPure 不支持 IPv6 风险评分")
		return finish(result, started)
	}
	result.Details["ip"] = freshIP
	result.Details["queried_ip"] = freshIP
	result.Details["ip_family"] = "IPv4"

	state := newIPPureNonceState()
	signedClient := ippureWebClient(client, state, true)
	basicURL := ippureWebAPIBase + ippureWebBasicPath + freshIP
	basicResult, basicBody, basicOK := boundedGETStatuses(ctx, signedClient, basicURL, ippureWebBodyLimit,
		[]int{http.StatusOK, http.StatusUnauthorized, http.StatusForbidden}, "application/json")
	ippureAccumulate(&result, basicResult)
	if ippureTerminalTransportOutcome(basicResult.Outcome) {
		ippureCarryFailure(&result, basicResult)
		return finish(result, started)
	}
	if !basicOK {
		if rule.RuleVersion >= 4 && basicResult.Outcome == "challenge" && ippureStatusCode(basicResult) == http.StatusForbidden {
			return ippureOfficialFallback(ctx, client, rule, result, basicResult, freshIP, started)
		}
		ippureCarryFailure(&result, basicResult)
		return finish(result, started)
	}
	hasNonce, protocolError, rechallenged := state.status()
	if protocolError != "" {
		ippureUnknown(&result, "nonce_protocol", protocolError)
		return finish(result, started)
	}
	if rechallenged {
		ippureChallenge(&result)
		return finish(result, started)
	}
	if hasNonce {
		if !state.firstNonceResponseAllowsReplay(basicBody) {
			if ippureStatusCode(basicResult) >= 400 {
				ippureCarryFailure(&result, basicResult)
				ippureHTTPRejected(&result)
			} else {
				ippureUnknown(&result, "nonce_protocol", "IPPure nonce 响应不是可接受的 JSON API 响应")
			}
			return finish(result, started)
		}
		basicResult, basicBody, basicOK = boundedGETStatuses(ctx, signedClient, basicURL, ippureWebBodyLimit,
			[]int{http.StatusOK, http.StatusUnauthorized, http.StatusForbidden}, "application/json")
		ippureAccumulate(&result, basicResult)
		if ippureTerminalTransportOutcome(basicResult.Outcome) {
			ippureCarryFailure(&result, basicResult)
			return finish(result, started)
		}
		_, protocolError, rechallenged = state.status()
		if rechallenged {
			ippureChallenge(&result)
			return finish(result, started)
		}
		if protocolError != "" {
			ippureUnknown(&result, "nonce_protocol", protocolError)
			return finish(result, started)
		}
		if !basicOK {
			ippureCarryFailure(&result, basicResult)
			return finish(result, started)
		}
	}
	if status := ippureStatusCode(basicResult); status != http.StatusOK {
		ippureCarryFailure(&result, basicResult)
		ippureHTTPRejected(&result)
		return finish(result, started)
	}
	basicData, err := ippureEnvelopeData(basicBody)
	if err != nil {
		ippureUnknown(&result, "basic_response", "IPPure 基础资料响应格式无法识别")
		return finish(result, started)
	}
	basicFields, err := ippureBasicDetails(basicData, freshIP)
	if err != nil {
		ippureUnknown(&result, "basic_ip", "IPPure 基础资料与本次观察到的 IPv4 不一致或格式无效")
		return finish(result, started)
	}
	for key, value := range basicFields {
		result.Details[key] = value
	}

	riskURL := ippureWebAPIBase + ippureWebRiskPath + freshIP
	riskResult, riskBody, riskOK := boundedGET(ctx, signedClient, riskURL, ippureWebBodyLimit, "application/json")
	ippureAccumulate(&result, riskResult)
	if ippureTerminalTransportOutcome(riskResult.Outcome) {
		ippureCarryFailure(&result, riskResult)
		return finish(result, started)
	}
	_, protocolError, rechallenged = state.status()
	if rechallenged {
		ippureChallenge(&result)
		return finish(result, started)
	}
	if protocolError != "" {
		ippureUnknown(&result, "nonce_protocol", protocolError)
		return finish(result, started)
	}
	if !riskOK {
		ippureCarryFailure(&result, riskResult)
		return finish(result, started)
	}
	riskData, err := ippureEnvelopeData(riskBody)
	if err != nil {
		ippureUnknown(&result, "risk_response", "IPPure 风险响应格式无法识别")
		return finish(result, started)
	}
	riskScore, riskIP, hasRiskIP, err := ippureRiskFields(riskData)
	if err != nil || riskScore < 0 || riskScore > 100 {
		ippureUnknown(&result, "risk_score", "IPPure 未返回有效的 0–100 整数风险分")
		return finish(result, started)
	}
	if hasRiskIP && !ippureSameIPv4(riskIP, freshIP) {
		ippureUnknown(&result, "risk_ip", "IPPure 风险资料与本次观察到的 IPv4 不一致")
		return finish(result, started)
	}
	result.Outcome = "profiled"
	result.Summary = fmt.Sprintf("IPPure风险 %d/100，越低越好", riskScore)
	result.Details["fraud_score"] = strconv.Itoa(riskScore)
	result.Details["risk_scale_max"] = "100"
	result.Details["risk_direction"] = "lower_is_better"
	return finish(result, started)
}

func ippurePublicIPv4(value string) (string, bool) {
	address, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil || !address.Is4() || !address.IsGlobalUnicast() || address.IsPrivate() ||
		address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsUnspecified() || address.IsMulticast() {
		return "", false
	}
	for _, prefix := range ippureNonPublicIPv4Ranges {
		if prefix.Contains(address) {
			return "", false
		}
	}
	return address.String(), true
}

func ippureSameIPv4(value, expected string) bool {
	got, ok := ippurePublicIPv4(value)
	if !ok {
		return false
	}
	want, ok := ippurePublicIPv4(expected)
	return ok && got == want
}

func ippureEnvelopeData(body []byte) (json.RawMessage, error) {
	var envelope struct {
		OK   *bool           `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.OK == nil || !*envelope.OK || len(envelope.Data) == 0 || strings.TrimSpace(string(envelope.Data)) == "null" {
		return nil, fmt.Errorf("IPPure response envelope is not successful")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(envelope.Data, &object); err != nil || object == nil {
		return nil, fmt.Errorf("IPPure response data is not an object")
	}
	return envelope.Data, nil
}

func ippureBasicDetails(data json.RawMessage, freshIP string) (map[string]string, error) {
	object, err := ippureJSONObject(data)
	if err != nil {
		return nil, err
	}
	if raw, found := ippureLookup(object, "ip"); found {
		value, ok := ippureScalar(raw)
		if !ok || !ippureSameIPv4(value, freshIP) {
			return nil, fmt.Errorf("IPPure basic response IP did not match")
		}
	}

	details := map[string]string{}
	geo, _ := ippureJSONObject(object["geo"])
	for detailKey, candidates := range map[string][]string{
		"country":      {"country"},
		"country_code": {"country_code", "countryCode"},
		"city":         {"city"},
		"region":       {"region"},
	} {
		if raw, found := ippureLookup(object, candidates...); found {
			if value, ok := ippureScalar(raw); ok && value != "" {
				details[detailKey] = value
				continue
			}
		}
		if raw, found := ippureLookup(geo, candidates...); found {
			if value, ok := ippureScalar(raw); ok && value != "" {
				details[detailKey] = value
			}
		}
	}

	if raw, found := ippureLookup(object, "asn"); found {
		if asnObject, objectErr := ippureJSONObject(raw); objectErr == nil && asnObject != nil {
			if number, hasNumber := ippureLookup(asnObject, "number"); hasNumber {
				if value := formatRawASN(number); value != "" {
					details["asn"] = value
				}
			}
			if organization, hasOrganization := ippureLookup(asnObject, "org", "organization"); hasOrganization {
				if value, ok := ippureScalar(organization); ok && value != "" {
					details["organization"] = value
				}
			}
		} else if _, scalar := ippureScalar(raw); scalar {
			if value := formatRawASN(raw); value != "" {
				details["asn"] = value
			}
		}
	}
	if raw, found := ippureLookup(object, "organization", "as_organization", "asOrganization", "org"); found {
		if value, ok := ippureScalar(raw); ok && value != "" {
			details["organization"] = value
		}
	}
	ippureAddExplicitFlag(object, details, "is_residential", "ip_type", "住宅", "非住宅", "is_residential", "isResidential", "residential")
	ippureAddExplicitFlag(object, details, "is_broadcast", "origin_type", "广播 IP", "", "is_broadcast", "isBroadcast", "broadcast")
	if raw, found := ippureLookup(object, "is_native", "isNative", "native"); found {
		if value, ok := ippureScalar(raw); ok {
			details["is_native"] = value
			if native, err := strconv.ParseBool(value); err == nil && details["origin_type"] == "" {
				if native {
					details["origin_type"] = "原生 IP"
				} else {
					details["origin_type"] = "非原生 IP"
				}
			}
		}
	}
	return details, nil
}

func ippureRiskFields(data json.RawMessage) (int, string, bool, error) {
	object, err := ippureJSONObject(data)
	if err != nil {
		return 0, "", false, err
	}
	rawScore, found := ippureLookup(object, "risk_score")
	if !found || len(rawScore) == 0 || strings.TrimSpace(string(rawScore)) == "null" {
		return 0, "", false, fmt.Errorf("IPPure risk score is missing")
	}
	var score int
	if err := json.Unmarshal(rawScore, &score); err != nil {
		return 0, "", false, err
	}
	rawIP, hasIP := ippureLookup(object, "ip")
	if !hasIP {
		return score, "", false, nil
	}
	ip, ok := ippureScalar(rawIP)
	if !ok {
		return 0, "", true, fmt.Errorf("IPPure risk IP is invalid")
	}
	return score, ip, true, nil
}

func ippureJSONObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil, fmt.Errorf("expected JSON object")
	}
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, fmt.Errorf("expected JSON object")
	}
	return object, nil
}

func ippureLookup(object map[string]json.RawMessage, keys ...string) (json.RawMessage, bool) {
	for _, key := range keys {
		if value, ok := object[key]; ok {
			return value, true
		}
	}
	return nil, false
}

func ippureScalar(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return strings.TrimSpace(value), true
	}
	var boolean bool
	if err := json.Unmarshal(raw, &boolean); err == nil {
		return strconv.FormatBool(boolean), true
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		return number.String(), true
	}
	return "", false
}

func ippureAddExplicitFlag(object map[string]json.RawMessage, details map[string]string, detailKey, labelKey, yesLabel, noLabel string, keys ...string) {
	raw, found := ippureLookup(object, keys...)
	if !found {
		return
	}
	value, ok := ippureScalar(raw)
	if !ok {
		return
	}
	details[detailKey] = value
	if labelKey == "" {
		return
	}
	if parsed, err := strconv.ParseBool(value); err == nil {
		if parsed {
			details[labelKey] = yesLabel
		} else if noLabel != "" {
			details[labelKey] = noLabel
		}
	}
}

func ippureAccumulate(result *Result, part Result) {
	result.RequestCount += part.RequestCount
	result.BytesRead += part.BytesRead
	if part.HTTPStatus != nil {
		status := *part.HTTPStatus
		result.HTTPStatus = &status
	}
	if part.Details != nil {
		if result.Details == nil {
			result.Details = map[string]string{}
		}
		for key, value := range part.Details {
			result.Details[key] = value
		}
	}
}

func ippureCarryFailure(result *Result, part Result) {
	result.Outcome = part.Outcome
	result.Summary = part.Summary
	result.FailurePhase = part.FailurePhase
	result.ErrorMessage = part.ErrorMessage
	if part.Details != nil {
		if result.Details == nil {
			result.Details = map[string]string{}
		}
		for key, value := range part.Details {
			result.Details[key] = value
		}
	}
}

func ippureTerminalTransportOutcome(outcome string) bool {
	return outcome == "cancelled" || outcome == "timed_out" || outcome == "rate_limited"
}

func ippureUnknown(result *Result, phase, message string) {
	result.Outcome = "unknown"
	result.Summary = "IPPure 风险资料未确认"
	result.FailurePhase = phase
	result.ErrorMessage = message
}

func ippureChallenge(result *Result) {
	result.Outcome = "challenge"
	result.Summary = "IPPure API 要求再次验证；已停止后续查询"
	result.FailurePhase = "nonce_challenge"
	result.ErrorMessage = "IPPure 网页接口再次返回签名挑战，未继续重试"
}

func ippureHTTPRejected(result *Result) {
	if result.HTTPStatus == nil {
		return
	}
	status := *result.HTTPStatus
	result.Outcome = "http_rejected"
	result.FailurePhase = "http_status"
	result.ErrorMessage = fmt.Sprintf("服务返回 HTTP %d；判据要求 HTTP 200", status)
}

func ippureStatusCode(result Result) int {
	if result.HTTPStatus == nil {
		return 0
	}
	return *result.HTTPStatus
}

// The documented self-IP API is a separate provider endpoint, not a replay of
// the browser challenge. It gets one request within the same attempt deadline.
type ippureOfficialTransport struct{ base http.RoundTripper }

func (t *ippureOfficialTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	clone.Header.Set("User-Agent", "Clash-SpeedTest-Pro/1.0 (IPPure public API client)")
	return t.base.RoundTrip(clone)
}
func (t *ippureOfficialTransport) CloseIdleConnections() {
	if c, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}
func ippureOfficialFallback(ctx context.Context, client *http.Client, rule Rule, result Result, failed Result, freshIP string, started time.Time) Result {
	result.Details["web_basic_outcome"] = failed.Outcome
	result.Details["web_basic_http_status"] = strconv.Itoa(ippureStatusCode(failed))
	result.Details["api_method"] = "ippure_official_self_api_fallback_v1"
	result.Details["api_version"] = "public_v1"
	result.Details["official_endpoint"] = "https://my.ippure.com/v1/info"
	result.Details["query_at"] = started.UTC().Format(time.RFC3339Nano)
	officialClient := ippureWebClient(client, nil, false)
	base := officialClient.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	officialClient.Transport = &ippureOfficialTransport{base: base}
	rule.RuleVersion = 2
	rule.TargetURL = "https://my.ippure.com/v1/info"
	rule.MaximumBodyBytes = 8 * 1024
	part := checkIPPure(ctx, officialClient, rule, started)
	details := part.Details
	part.Details = nil
	ippureAccumulate(&result, part)
	if part.Outcome != "profiled" {
		ippureCarryFailure(&result, part)
		return finish(result, started)
	}
	if !ippureSameIPv4(details["ip"], freshIP) {
		ippureUnknown(&result, "official_ip_mismatch", "IPPure 官方接口未返回本次相同的 IPv4 出口，未借用其它地址的评分")
		return finish(result, started)
	}
	score, err := strconv.Atoi(details["fraud_score"])
	if err != nil || score < 0 || score > 100 {
		ippureUnknown(&result, "official_risk_score", "IPPure 官方接口未返回有效的 0–100 风险分")
		return finish(result, started)
	}
	for key, value := range details {
		result.Details[key] = value
	}
	result.Details["ip_family"] = "IPv4"
	result.Details["risk_scale_max"] = "100"
	result.Details["risk_direction"] = "lower_is_better"
	result.Outcome = "profiled"
	result.Summary = fmt.Sprintf("IPPure风险 %d/100，越低越好", score)
	result.ErrorMessage = ""
	result.FailurePhase = ""
	return finish(result, started)
}
