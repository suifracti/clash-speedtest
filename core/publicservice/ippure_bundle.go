package publicservice

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"golang.org/x/text/encoding/unicode"
	"io"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const ippureCFURL = "https://cf.999831.xyz/api/cf-summary"
const ippureCFGateway = ippureWebAPIBase + "/api/gateway/cf"

// This API binds its ephemeral nonce to the connection. Keep that connection
// only inside this attempt, without changing latency/download/client defaults.
func ippureSessionTransport(base http.RoundTripper) http.RoundTripper {
	switch t := base.(type) {
	case phaseTransport:
		return phaseTransport{base: ippureSessionTransport(t.base), phases: t.phases}
	case *http.Transport:
		clone := t.Clone()
		clone.DisableKeepAlives = false
		return clone
	default:
		return base
	}
}

type ippureOriginTransport struct{ base http.RoundTripper }

func (t ippureOriginTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	if req.URL.Host == "api.123169.xyz" {
		clone.Header.Set("Origin", "https://ippure.com")
		clone.Header.Set("Referer", "https://ippure.com/")
	}
	return t.base.RoundTrip(clone)
}
func (t ippureOriginTransport) CloseIdleConnections() {
	if c, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}

// Match the site's normal signed API request: SHA256(x-t) AES-CBC key,
// random IV followed by PKCS7-padded ciphertext. No browser cookies are copied.
// Request.text() uses the web UTF-8 decoder: malformed sequences collapse by
// maximal subpart, and a leading BOM is removed. Go rune conversion differs.
func ippureBodyText(raw []byte) string {
	text, _ := unicode.UTF8BOM.NewDecoder().String(string(raw))
	return text
}

func ippureEncryptedBody(request *http.Request, raw []byte, xt string) error {
	key := sha256.Sum256([]byte(xt))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return err
	}
	pad := aes.BlockSize - len(raw)%aes.BlockSize
	plain := append(append([]byte(nil), raw...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	body := make([]byte, aes.BlockSize+len(plain))
	if _, err = rand.Read(body[:aes.BlockSize]); err != nil {
		return err
	}
	cipher.NewCBCEncrypter(block, body[:aes.BlockSize]).CryptBlocks(body[aes.BlockSize:], plain)
	request.Body = io.NopCloser(bytes.NewReader(body))
	request.ContentLength = int64(len(body))
	request.GetBody = nil
	return nil
}

// Normal API nonce refreshes are bounded by the website's replay limit; HTTP/CAPTCHA failures stop.
func ippureExtraExchange(ctx context.Context, client *http.Client, method, target string, payload []byte, session ...*ippureNonceState) (Result, []byte, bool) {
	state := newIPPureNonceState()
	if len(session) > 0 && session[0] != nil {
		state = session[0]
	}
	// A refusal in the CF gateway must not poison the independent ASN GET.
	if _, protocolError, rechallenged := state.status(); protocolError != "" || rechallenged {
		limit := state.replayLimit()
		state = newIPPureNonceState()
		state.normalRefreshLimit = limit
	}
	local := ippureWebClient(client, state, true)
	total := Result{}
	for pass := 0; pass < 2+state.replayLimit(); pass++ {
		before := state.revision()
		part := Result{RequestCount: 1}
		req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(payload))
		if err != nil {
			return part, nil, false
		}
		req.Header.Set("User-Agent", browserUserAgent)
		req.Header.Set("Accept", "application/json")
		if method == http.MethodPost {
			req.Header.Set("Content-Type", "application/octet-stream")
		}
		response, err := local.Do(req)
		if err != nil {
			classifyRequestError(&part, ctx, err)
			ippureAccumulate(&total, part)
			ippureCarryFailure(&total, part)
			return total, nil, false
		}
		code := response.StatusCode
		part.HTTPStatus = &code
		body, readErr := io.ReadAll(io.LimitReader(response.Body, ippureWebBodyLimit+1))
		response.Body.Close()
		part.BytesRead = int64(min(len(body), ippureWebBodyLimit))
		if serviceRateLimit(&part, response, false) {
			ippureAccumulate(&total, part)
			ippureCarryFailure(&total, part)
			return total, nil, false
		}
		if ippureHasCFChallengeMarker(response.Header) {
			part.Outcome = "challenge"
			part.ErrorMessage = "验证页拦截"
		} else if code != 200 {
			part.Outcome = "http_rejected"
			part.ErrorMessage = fmt.Sprintf("HTTP %d", code)
		} else if readErr != nil {
			classifyRequestError(&part, ctx, readErr)
		} else if len(body) > ippureWebBodyLimit {
			ippureUnknown(&part, "response_limit", "响应超过读取上限")
		}
		ippureAccumulate(&total, part)
		if part.Outcome != "" {
			ippureCarryFailure(&total, part)
			return total, nil, false
		}
		_, protocolError, rechallenged := state.status()
		if protocolError != "" || rechallenged {
			ippureChallenge(&total)
			return total, nil, false
		}
		if state.revision() > before {
			if !state.firstNonceResponseAllowsReplay(body, true) {
				ippureUnknown(&total, "nonce_protocol", "签名响应格式无法识别")
				return total, nil, false
			}
			continue
		}
		return total, body, true
	}
	ippureChallenge(&total)
	return total, nil, false
}
func ippureExtraStatus(r Result) string {
	if r.FailurePhase == "nonce_challenge" {
		return "接口签名未通过"
	}
	if r.HTTPStatus != nil && *r.HTTPStatus != 200 {
		return fmt.Sprintf("HTTP %d", *r.HTTPStatus)
	}
	switch r.Outcome {
	case "cancelled":
		return "已取消"
	case "timed_out":
		return "查询超时"
	case "challenge":
		return "验证页拦截"
	case "rate_limited":
		return "目标限流"
	}
	return "未取得结果"
}

func checkIPPureBundle(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	local := *client
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	sessionTransport := ippureSessionTransport(base)
	if rule.RuleVersion >= 7 {
		sessionTransport = ippureBrowserSession(base)
	}
	local.Transport = ippureOriginTransport{sessionTransport}
	defer local.CloseIdleConnections()
	state := newIPPureNonceState()
	var result Result
	var cfPending chan Result
	if rule.RuleVersion >= 7 {
		state.normalRefreshLimit = 3
		cfPending = make(chan Result, 1)
		go func() {
			cfCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
			defer cancel()
			cfPending <- checkIPPureCF(cfCtx, &local, state)
		}()
	}
	if rule.RuleVersion >= 7 {
		official := rule
		official.RuleVersion = 2
		official.TargetURL = "https://my.ippure.com/v1/info"
		official.MaximumBodyBytes = 8 * 1024
		result = checkIPPure(ctx, &local, official, started)
		if result.Details == nil {
			result.Details = map[string]string{}
		}
		result.Details["provider"] = "IPPure"
		result.Details["api_method"] = "ippure_official_self_api_v2"
		result.Details["official_endpoint"] = official.TargetURL
		result.Details["query_at"] = started.UTC().Format(time.RFC3339Nano)
		if result.Outcome == "profiled" {
			ip, valid := ippurePublicIPv4(result.Details["ip"])
			score, err := strconv.Atoi(result.Details["fraud_score"])
			if !valid || err != nil || score < 0 || score > 100 {
				ippureUnknown(&result, "official_profile", "官方接口未返回有效IPv4风险资料")
			} else {
				result.Details["ip"] = ip
				result.Details["ip_family"] = "IPv4"
				result.Details["risk_scale_max"] = "100"
				result.Details["risk_direction"] = "lower_is_better"
				result.Details["api_method"] = "ippure_official_self_api_v2"
				result.Details["official_endpoint"] = official.TargetURL
				result.Details["query_at"] = started.UTC().Format(time.RFC3339Nano)
			}
		}
	} else {
		result = checkIPPureWeb(ctx, &local, rule, started, state)
	}
	// Preserve the working public self-IP path if the web nonce API refuses
	// this client. A failed extra query must not regress the primary score.
	if result.Outcome == "challenge" && ctx.Err() == nil && result.Details["ip"] != "" {
		result = ippureOfficialFallback(ctx, &local, rule, result, result, result.Details["ip"], started)
	}
	if rule.RuleVersion < 7 {
		if _, protocolError, rechallenged := state.status(); protocolError != "" || rechallenged {
			state = newIPPureNonceState()
		}
	}
	if result.Details == nil {
		result.Details = map[string]string{}
	}
	d := result.Details
	d["profile_bundle_version"] = "1"
	d["cf_status"] = "未查询"
	d["asn_traffic_status"] = "未查询"
	if rule.RuleVersion >= 7 {
		d["profile_bundle_version"] = "2"
		d["api_client_profile"] = "standard_transport"
		if ippureUsesBrowserSession(sessionTransport) {
			d["api_client_profile"] = "chrome133_h2_v1"
		}
	}
	// Each sub-result keeps its own scope/time. Its failure cannot overwrite the
	// IPPure score, HTTP status, or outcome of the primary lookup.
	add := func(part Result) { result.RequestCount += part.RequestCount; result.BytesRead += part.BytesRead }
	if ctx.Err() != nil && cfPending == nil {
		d["cf_status"] = "本次时限已到"
		d["asn_traffic_status"] = "本次时限已到"
		return finish(result, started)
	}
	var cfResult Result
	if cfPending != nil {
		cfResult = <-cfPending
	} else {
		cfResult = checkIPPureCF(ctx, &local, state)
	}
	add(cfResult)
	for k, v := range cfResult.Details {
		d[k] = v
	}
	asn := strings.TrimPrefix(strings.ToUpper(d["asn"]), "AS")
	if asn == "" {
		asn = d["cf_asn"]
	}

	asnNumber, err := strconv.ParseUint(asn, 10, 32)
	if err != nil || asnNumber == 0 {
		d["asn_traffic_status"] = "未取得ASN"
		return finish(result, started)
	}
	d["asn_traffic_asn"] = "AS" + asn
	d["asn_traffic_scope"] = "整个ASN的统计，并非当前节点的流量比例"
	d["asn_traffic_query_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	d["asn_traffic_source_url"] = "https://ippure.com/guide/ASN-Human-Bot-Ratio"
	if ctx.Err() != nil {
		d["asn_traffic_status"] = "本次时限已到"
		return finish(result, started)
	}
	asnCtx := ctx
	cancelASN := func() {}
	if rule.RuleVersion >= 7 {
		asnCtx, cancelASN = context.WithTimeout(ctx, 3*time.Second)
	}
	defer cancelASN()
	part, body, ok := ippureExtraExchange(asnCtx, &local, http.MethodGet, ippureWebAPIBase+"/api/info/asn/botclass/"+asn, nil, state)
	add(part)
	if !ok {
		d["asn_traffic_status"] = ippureExtraStatus(part)
		return finish(result, started)
	}
	var doc struct {
		OK   bool `json:"ok"`
		Data struct {
			Human *float64 `json:"human"`
			Bot   *float64 `json:"bot"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &doc) != nil || !doc.OK || doc.Data.Human == nil || doc.Data.Bot == nil || *doc.Data.Human < 0 || *doc.Data.Human > 100 || *doc.Data.Bot < 0 || *doc.Data.Bot > 100 || math.Abs(*doc.Data.Human+*doc.Data.Bot-100) > 0.1 {
		d["asn_traffic_status"] = "统计数据不足"
		return finish(result, started)
	}
	d["asn_human_percent"] = strconv.FormatFloat(*doc.Data.Human, 'f', 2, 64)
	d["asn_bot_percent"] = strconv.FormatFloat(*doc.Data.Bot, 'f', 2, 64)
	d["asn_traffic_status"] = "已取得"
	return finish(result, started)
}

func checkIPPureCF(ctx context.Context, local *http.Client, state *ippureNonceState) Result {
	result := Result{Details: map[string]string{"cf_status": "未取得结果"}}
	d := result.Details
	add := func(part Result) { result.RequestCount += part.RequestCount; result.BytesRead += part.BytesRead }
	cfAt := time.Now().UTC()
	d["cf_query_at"] = cfAt.Format(time.RFC3339Nano)
	d["cf_source_url"] = "https://ippure.com/cloudflare"
	d["cf_scope"] = "本次App请求环境，受出口与客户端特征影响，不等于浏览器结果"
	cfCtx := ctx
	part, payload, ok := boundedGET(cfCtx, local, ippureCFURL, ippureWebBodyLimit, "application/octet-stream")
	add(part)
	asn := ""
	if ok {
		part, body, valid := ippureExtraExchange(cfCtx, local, http.MethodPost, ippureCFGateway, payload, state)
		add(part)
		if valid {
			var doc struct {
				IP   string          `json:"ip"`
				Risk *int            `json:"risk_score"`
				ASN  json.RawMessage `json:"asn"`
			}
			if json.Unmarshal(body, &doc) == nil && net.ParseIP(doc.IP) != nil && doc.Risk != nil && *doc.Risk >= 0 && *doc.Risk <= 100 {
				d["cf_risk_score"] = strconv.Itoa(*doc.Risk)
				d["cf_status"] = "已取得"
				d["cf_ip"] = doc.IP
				d["cf_ip_family"] = "IPv6"
				if net.ParseIP(doc.IP).To4() != nil {
					d["cf_ip_family"] = "IPv4"
				}
				if asn == "" {
					asn = strings.TrimPrefix(cleanASN(doc.ASN), "AS")
				}
			} else {
				d["cf_status"] = "未返回有效系数"
			}
		} else {
			d["cf_status"] = ippureExtraStatus(part)
		}
	} else {
		d["cf_status"] = ippureExtraStatus(part)
	}
	d["cf_asn"] = asn
	return result
}
