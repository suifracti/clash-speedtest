package publicservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const browserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/125 Safari/537.36"

func checkCloudflareTrace(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result, body, ok := boundedGET(ctx, client, rule.TargetURL, rule.MaximumBodyBytes)
	if !ok {
		return finish(result, started)
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(body), "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if found && key != "" && value != "" {
			values[key] = value
		}
	}
	if net.ParseIP(values["ip"]) == nil || len(values["loc"]) != 2 {
		result.Outcome = "criteria_mismatch"
		result.FailurePhase = "trace_document"
		result.ErrorMessage = "Cloudflare trace 未包含出口 IP 或国家"
		return finish(result, started)
	}
	result.Outcome = "profiled"
	result.Summary = strings.TrimSpace(values["loc"] + " · " + values["ip"] + valueSuffix(" · ", values["colo"]))
	result.Details = map[string]string{
		"ip": values["ip"], "country_code": values["loc"], "cloudflare_colo": values["colo"],
		"tls": values["tls"], "http_protocol": values["http"],
	}
	return finish(result, started)
}

func checkIPify(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result, body, ok := boundedGET(ctx, client, rule.TargetURL, rule.MaximumBodyBytes)
	family := "IPv4"
	if rule.ServiceID == "exit_ipv6" {
		family = "IPv6"
	}
	if !ok {
		if result.Outcome != "cancelled" {
			result.Outcome = "unknown"
			result.Summary = family + " 出口未确认；可能是节点不支持、服务暂时不可达或请求超时"
		}
		return finish(result, started)
	}
	var payload struct {
		IP string `json:"ip"`
	}
	if json.Unmarshal(body, &payload) != nil {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "unknown", "response_body", "出口服务未返回可识别的 JSON"
		return finish(result, started)
	}
	ip := net.ParseIP(strings.TrimSpace(payload.IP))
	if ip == nil || (family == "IPv4" && ip.To4() == nil) || (family == "IPv6" && ip.To4() != nil) {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "unknown", "ip_family", "出口服务返回的地址与检测的 IP 类型不符"
		return finish(result, started)
	}
	result.Outcome = "profiled"
	result.Summary = family + " 出口 " + ip.String()
	result.Details = map[string]string{
		"ip": ip.String(), "ip_family": family,
		"exit_scope": "此检测目标看到的节点出口；其它服务可能因 DNS 或分流策略看到不同地址",
	}
	return finish(result, started)
}

func checkPing0(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result, body, ok := boundedGET(ctx, client, rule.TargetURL, rule.MaximumBodyBytes)
	if !ok {
		return finish(result, started)
	}
	var payload struct {
		IP       string `json:"ip"`
		Location string `json:"location"`
		Country  string `json:"country"`
		City     string `json:"city"`
		ASN      string `json:"asn"`
		Org      string `json:"org"`
		IsIDC    *bool  `json:"isidc"`
		IPRisk   *int   `json:"iprisk"`
	}
	if json.Unmarshal(body, &payload) != nil || strings.TrimSpace(payload.IP) == "" {
		result.Outcome = "criteria_mismatch"
		result.FailurePhase = "json_document"
		result.ErrorMessage = "Ping0 响应未包含可识别的出口 IP"
		return finish(result, started)
	}
	location := strings.TrimSpace(payload.Location)
	if location == "" {
		location = strings.TrimSpace(payload.Country + " " + payload.City)
	}
	ipType := optionalFlag(payload.IsIDC, "机房 IDC", "非 IDC（不等于家宽）")
	risk := optionalScore(payload.IPRisk)
	result.Outcome = "profiled"
	result.Summary = fmt.Sprintf("第三方风险分 %s · %s", risk, ipType)
	result.Details = map[string]string{
		"ip": payload.IP, "location": location, "country": payload.Country, "asn": payload.ASN,
		"organization": payload.Org, "ip_type": ipType, "risk_score": risk,
	}
	return finish(result, started)
}

func checkIPPure(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result, body, ok := boundedGET(ctx, client, rule.TargetURL, rule.MaximumBodyBytes)
	if !ok {
		return finish(result, started)
	}
	var payload struct {
		IP            string `json:"ip"`
		ASN           int    `json:"asn"`
		ASOrg         string `json:"asOrganization"`
		Country       string `json:"country"`
		CountryCode   string `json:"countryCode"`
		City          string `json:"city"`
		FraudScore    *int   `json:"fraudScore"`
		IsResidential *bool  `json:"isResidential"`
		IsBroadcast   *bool  `json:"isBroadcast"`
	}
	if json.Unmarshal(body, &payload) != nil || strings.TrimSpace(payload.IP) == "" {
		result.Outcome = "criteria_mismatch"
		result.FailurePhase = "json_document"
		result.ErrorMessage = "IPPure 响应未包含可识别的出口 IP"
		return finish(result, started)
	}
	ipType := optionalFlag(payload.IsResidential, "住宅（第三方判断）", "非住宅")
	originType := optionalFlag(payload.IsBroadcast, "广播 IP", "原生 IP")
	risk := optionalScore(payload.FraudScore)
	result.Outcome = "profiled"
	result.Summary = fmt.Sprintf("第三方欺诈分 %s · %s · %s", risk, ipType, originType)
	result.Details = map[string]string{
		"ip": payload.IP, "country": payload.Country, "country_code": payload.CountryCode,
		"location": strings.TrimSpace(payload.Country + " " + payload.City), "asn": formatASN(payload.ASN),
		"organization": payload.ASOrg, "ip_type": ipType, "origin_type": originType,
		"fraud_score": risk,
	}
	return finish(result, started)
}

func checkCloudflareDoH(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result, body, ok := boundedGET(ctx, client, rule.TargetURL, rule.MaximumBodyBytes)
	if !ok {
		return finish(result, started)
	}
	var payload struct {
		Status *int `json:"Status"`
		Answer []struct {
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if json.Unmarshal(body, &payload) != nil {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "criteria_mismatch", "json_document", "DoH 响应不是可识别的 DNS JSON"
		return finish(result, started)
	}
	if payload.Status == nil || *payload.Status != 0 || len(payload.Answer) == 0 {
		result.Outcome, result.FailurePhase = "criteria_mismatch", "dns_status"
		result.ErrorMessage = "DoH 未返回成功状态及查询答案，不能确认解析成功"
		return finish(result, started)
	}
	result.Outcome, result.Summary = "matched", "DoH 查询成功"
	result.Details = map[string]string{"dns_status": "0", "answer_count": strconv.Itoa(len(payload.Answer))}
	return finish(result, started)
}

func checkAbema(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result, body, ok := boundedGET(ctx, client, rule.TargetURL, rule.MaximumBodyBytes)
	if !ok {
		return finish(result, started)
	}
	var payload struct {
		ISOCountryCode string `json:"isoCountryCode"`
	}
	if json.Unmarshal(body, &payload) != nil || strings.TrimSpace(payload.ISOCountryCode) == "" {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "unknown", "region_document", "ABEMA 未返回可识别的地区代码"
		return finish(result, started)
	}
	region := strings.ToUpper(strings.TrimSpace(payload.ISOCountryCode))
	result.Details = map[string]string{"service_region": region}
	if region == "JP" {
		result.Outcome, result.Summary = "unlocked", "日本地区入口可用"
	} else {
		result.Outcome, result.Summary = "region_limited", "仅海外档位"
	}
	return finish(result, started)
}

var fodFlagPattern = regexp.MustCompile(`(?i)<FLAG\s+TYPE=["'](true|false)["']`)

func checkFOD(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result, body, ok := boundedGET(ctx, client, rule.TargetURL, rule.MaximumBodyBytes)
	if !ok {
		return finish(result, started)
	}
	match := fodFlagPattern.FindStringSubmatch(string(body))
	if len(match) != 2 {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "unknown", "region_document", "FOD 地区端点未返回可识别的 FLAG"
		return finish(result, started)
	}
	if strings.EqualFold(match[1], "true") {
		result.Outcome, result.Summary = "unlocked", "日本地区允许"
	} else {
		result.Outcome, result.Summary = "region_blocked", "日本地区限制"
	}
	return finish(result, started)
}

func checkYouTubePremium(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result, page, ok := readPage(ctx, client, rule.TargetURL, []string{"youtube.com", "google.com"}, map[string]string{"Accept-Language": "en-US,en;q=0.9"})
	if !ok {
		return finish(result, started)
	}
	text := string(page.body)
	lower := strings.ToLower(text)
	region := firstRegexGroup(regexp.MustCompile(`(?i)"INNERTUBE_CONTEXT_GL"\s*:\s*"([^"]+)"`), text)
	if strings.Contains(lower, "www.google.cn") {
		region = "CN"
	}
	if region != "" {
		result.Details = map[string]string{"service_region": strings.ToUpper(region)}
	}
	switch {
	case strings.Contains(lower, "premium is not available in your country") || region == "CN":
		result.Outcome, result.Summary = "region_blocked", "YouTube Premium 在该地区不可用"
	case region != "" && strings.Contains(lower, "ad-free"):
		result.Outcome, result.Summary = "unlocked", "Premium 页面有地区与订阅信息；未验证购买或播放"
	default:
		result.Outcome, result.FailurePhase, result.ErrorMessage = "unknown", "page_document", "YouTube Premium 页面未包含稳定的可用性标记"
	}
	return finish(result, started)
}

func checkPrimeVideo(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result, page, ok := readPage(ctx, client, rule.TargetURL, []string{"primevideo.com", "amazon.com"}, nil)
	if !ok {
		return finish(result, started)
	}
	text := string(page.body)
	region := firstRegexGroup(regexp.MustCompile(`(?i)"currentTerritory"\s*:\s*"([^"]+)"`), text)
	if region != "" {
		result.Details = map[string]string{"service_region": strings.ToUpper(region)}
	}
	restricted := firstRegexGroup(regexp.MustCompile(`(?i)"isServiceRestricted"\s*:\s*(true|false)`), text)
	if strings.EqualFold(restricted, "true") {
		result.Outcome, result.Summary = "region_blocked", "Prime Video 服务在该地区受限"
	} else if region != "" && strings.EqualFold(restricted, "false") {
		result.Outcome, result.Summary = "unlocked", "Prime Video 地区检查通过；未验证播放"
	} else {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "unknown", "page_document", "Prime Video 页面未包含稳定的地区标记"
	}
	return finish(result, started)
}

func checkHuluJapan(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result, page, ok := readPage(ctx, client, rule.TargetURL, []string{"hulu.jp"}, nil)
	if !ok {
		return finish(result, started)
	}
	result.Details = map[string]string{"final_url": page.finalURL}
	if strings.Contains(strings.ToLower(page.finalURL), "/restrict") {
		result.Outcome, result.Summary = "region_blocked", "Hulu Japan 地区限制"
	} else if page.status >= 200 && page.status < 300 {
		result.Outcome, result.Summary = "reachable", "Hulu Japan 网页可达；未验证地区授权或播放"
	} else {
		result.Outcome, result.FailurePhase = "unknown", "http_status"
		result.ErrorMessage = fmt.Sprintf("Hulu Japan 返回 HTTP %d，无法确认地区状态", page.status)
	}
	return finish(result, started)
}

func checkGoogleSearch(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result, page, ok := readPage(ctx, client, rule.TargetURL, []string{"google.com"}, map[string]string{"Accept-Language": "en-US,en;q=0.9"})
	if !ok {
		return finish(result, started)
	}
	lower := strings.ToLower(string(page.body))
	if strings.Contains(lower, "unusual traffic") || strings.Contains(lower, "/sorry/") || strings.Contains(lower, "recaptcha") {
		result.Outcome, result.Summary = "challenge", "Google 要求验证码或流量验证"
		return finish(result, started)
	}
	result.Outcome, result.Summary = "reachable", "Google 搜索结果页可达"
	return finish(result, started)
}

type fetchedPage struct {
	status   int
	body     []byte
	finalURL string
}

func readPage(ctx context.Context, client *http.Client, target string, allowedHosts []string, headers map[string]string) (Result, fetchedPage, bool) {
	result := Result{RequestCount: 1}
	clone := *client
	clone.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 || request.URL.Scheme != "https" {
			return http.ErrUseLastResponse
		}
		host := strings.ToLower(request.URL.Hostname())
		for _, suffix := range allowedHosts {
			if host == suffix || strings.HasSuffix(host, "."+suffix) {
				result.RequestCount++
				return nil
			}
		}
		return http.ErrUseLastResponse
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "transport_error", "request_setup", "无法创建固定服务请求"
		return result, fetchedPage{}, false
	}
	request.Header.Set("User-Agent", browserUserAgent)
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := clone.Do(request)
	if err != nil {
		classifyRequestError(&result, ctx, err)
		return result, fetchedPage{}, false
	}
	defer response.Body.Close()
	status := response.StatusCode
	result.HTTPStatus = &status
	if strings.EqualFold(strings.TrimSpace(response.Header.Get("cf-mitigated")), "challenge") {
		result.Outcome, result.Summary, result.ErrorMessage = "challenge", "Cloudflare 要求浏览器验证", "网络已连通，但无头请求未通过网页验证"
		return result, fetchedPage{}, false
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, MaximumResponseBytes+1))
	result.BytesRead = int64(min(len(body), MaximumResponseBytes))
	if err != nil {
		classifyRequestError(&result, ctx, err)
		return result, fetchedPage{}, false
	}
	if len(body) > MaximumResponseBytes {
		body = body[:MaximumResponseBytes]
	}
	if pageHasChallenge(body) {
		result.Outcome, result.Summary = "challenge", "网页要求浏览器验证，不能确认功能可用"
		return result, fetchedPage{}, false
	}
	if status < 200 || status >= 300 {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "http_rejected", "http_status", fmt.Sprintf("HTTP %d 不能证明地区限制或功能可用", status)
		return result, fetchedPage{}, false
	}
	finalURL := target
	if response.Request != nil && response.Request.URL != nil {
		finalURL = response.Request.URL.String()
	}
	return result, fetchedPage{status: status, body: body, finalURL: finalURL}, true
}

func firstRegexGroup(pattern *regexp.Regexp, value string) string {
	match := pattern.FindStringSubmatch(value)
	if len(match) == 2 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

type netflixPage struct {
	status    int
	body      []byte
	finalURL  string
	challenge bool
}

func checkNetflix(ctx context.Context, client *http.Client, _ Rule, started time.Time) Result {
	result := Result{RequestCount: 0, Details: map[string]string{}}
	clone := *client
	clone.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		host := strings.ToLower(request.URL.Hostname())
		if len(via) >= 5 || request.URL.Scheme != "https" || (host != "netflix.com" && !strings.HasSuffix(host, ".netflix.com")) {
			return http.ErrUseLastResponse
		}
		result.RequestCount++
		return nil
	}
	global, ok := netflixGET(ctx, &clone, "https://www.netflix.com/title/81280792", &result)
	if !ok {
		return finish(result, started)
	}
	licensed, ok := netflixGET(ctx, &clone, "https://www.netflix.com/title/70143836", &result)
	if !ok {
		return finish(result, started)
	}
	result.HTTPStatus = intPointer(licensed.status)
	if global.challenge || licensed.challenge {
		result.Outcome = "challenge"
		result.Summary = "Netflix 要求网页验证"
		result.ErrorMessage = "已连通 Netflix，但测试请求遇到挑战页"
		return finish(result, started)
	}
	globalAvailable := netflixTitleAvailable(global)
	licensedAvailable := netflixTitleAvailable(licensed)
	result.Details["global_title"] = availabilityLabel(globalAvailable)
	result.Details["licensed_title"] = availabilityLabel(licensedAvailable)
	if region := netflixRegion(licensed.finalURL + "\n" + string(licensed.body)); region != "" {
		result.Details["service_region"] = strings.ToUpper(region)
	}
	switch {
	case licensedAvailable && globalAvailable:
		result.Outcome = "unlocked"
		result.Summary = "测试片目页面可访问；不代表账号播放已解锁"
	case globalAvailable && licensed.status == http.StatusNotFound:
		result.Outcome = "originals_only"
		result.Summary = "宽松片目可访问、版权片目不存在；可能是片库差异，未验证播放"
	case global.status == http.StatusForbidden || licensed.status == http.StatusForbidden:
		result.Outcome = "service_rejected"
		result.Summary = "Netflix 拒绝测试请求"
		result.ErrorMessage = "HTTP 403 只能说明请求被拒绝，不能单独断定是地区限制"
	default:
		result.Outcome = "unknown"
		result.Summary = "片目结果不足以判断解锁档位"
		result.ErrorMessage = "两部测试片目均未给出可确认结果；Netflix 片库或页面结构可能已变化"
	}
	return finish(result, started)
}

func netflixGET(ctx context.Context, client *http.Client, target string, result *Result) (netflixPage, bool) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "transport_error", "request_setup", "无法创建 Netflix 测试请求"
		return netflixPage{}, false
	}
	request.Header.Set("User-Agent", browserUserAgent)
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	result.RequestCount++
	response, err := client.Do(request)
	if err != nil {
		classifyRequestError(result, ctx, err)
		return netflixPage{}, false
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, MaximumResponseBytes+1))
	result.BytesRead += int64(min(len(body), MaximumResponseBytes))
	if err != nil {
		classifyRequestError(result, ctx, err)
		return netflixPage{}, false
	}
	if len(body) > MaximumResponseBytes {
		body = body[:MaximumResponseBytes]
	}
	finalURL := target
	if response.Request != nil && response.Request.URL != nil {
		finalURL = response.Request.URL.String()
	}
	return netflixPage{
		status: response.StatusCode, body: body, finalURL: finalURL,
		challenge: strings.EqualFold(strings.TrimSpace(response.Header.Get("cf-mitigated")), "challenge") || pageHasChallenge(body),
	}, true
}

func netflixTitleAvailable(page netflixPage) bool {
	if page.status != http.StatusOK || page.challenge || !strings.Contains(page.finalURL, "/title/") {
		return false
	}
	text := strings.ToLower(string(page.body))
	return (strings.Contains(text, `"@type":"movie"`) || strings.Contains(text, `"@type":"tvseries"`) || strings.Contains(text, `class="title-title"`)) && !strings.Contains(text, "oh no!") && !strings.Contains(text, "page not found") && !strings.Contains(text, "not available")
}

func pageHasChallenge(body []byte) bool {
	text := strings.ToLower(string(body))
	return strings.Contains(text, "/cdn-cgi/challenge-platform/") || strings.Contains(text, "cf-chl-") || strings.Contains(text, "<title>just a moment") || strings.Contains(text, "unusual traffic from your computer network")
}

func optionalFlag(value *bool, yes, no string) string {
	if value == nil {
		return "未提供"
	}
	if *value {
		return yes
	}
	return no
}
func optionalScore(value *int) string {
	if value == nil || *value < 0 || *value > 100 {
		return "未提供"
	}
	return strconv.Itoa(*value)
}

var netflixRegionPattern = regexp.MustCompile(`(?i)netflix\.com/([a-z]{2}(?:-[a-z]{2})?)/title`)

func netflixRegion(value string) string {
	match := netflixRegionPattern.FindStringSubmatch(value)
	if len(match) == 2 {
		return match[1]
	}
	return ""
}

func boundedGET(ctx context.Context, client *http.Client, target string, maximum int) (Result, []byte, bool) {
	result := Result{RequestCount: 1}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "transport_error", "request_setup", "无法创建固定服务请求"
		return result, nil, false
	}
	request.Header.Set("User-Agent", browserUserAgent)
	request.Header.Set("Accept", "application/dns-json,application/json,text/plain;q=0.9,*/*;q=0.8")
	response, err := client.Do(request)
	if err != nil {
		classifyRequestError(&result, ctx, err)
		return result, nil, false
	}
	defer response.Body.Close()
	status := response.StatusCode
	result.HTTPStatus = &status
	if strings.EqualFold(strings.TrimSpace(response.Header.Get("cf-mitigated")), "challenge") {
		result.Outcome = "challenge"
		result.Summary = "Cloudflare 要求浏览器验证"
		result.ErrorMessage = "网络已连通，但无头请求未通过网页验证"
		return result, nil, false
	}
	if status != http.StatusOK {
		result.Outcome = "http_rejected"
		result.FailurePhase = "http_status"
		result.ErrorMessage = fmt.Sprintf("服务返回 HTTP %d；判据要求 HTTP 200", status)
		return result, nil, false
	}
	if maximum <= 0 || maximum > MaximumResponseBytes {
		maximum = MaximumResponseBytes
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(maximum)+1))
	result.BytesRead = int64(min(len(body), maximum))
	if err != nil {
		classifyRequestError(&result, ctx, err)
		return result, nil, false
	}
	if len(body) > maximum {
		result.Outcome = "criteria_mismatch"
		result.FailurePhase = "response_limit"
		result.ErrorMessage = fmt.Sprintf("响应超过 %d KiB 读取上限", maximum/1024)
		return result, nil, false
	}
	return result, body, true
}

func classifyRequestError(result *Result, ctx context.Context, err error) {
	var networkErr net.Error
	switch {
	case ctx.Err() == context.Canceled || errors.Is(err, context.Canceled):
		result.Outcome, result.FailurePhase, result.ErrorMessage = "cancelled", "cancelled", "检测已取消"
	case ctx.Err() == context.DeadlineExceeded || errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkErr) && networkErr.Timeout()):
		result.Outcome, result.FailurePhase, result.ErrorMessage = "timed_out", "timeout", "请求超过设定超时"
	default:
		result.Outcome, result.FailurePhase, result.ErrorMessage = "transport_error", "transport", "节点代理或 HTTP 传输失败；具体阶段未知"
	}
}

func formatASN(asn int) string {
	if asn <= 0 {
		return ""
	}
	return fmt.Sprintf("AS%d", asn)
}

func availabilityLabel(value bool) string {
	if value {
		return "可访问"
	}
	return "不可确认"
}

func valueSuffix(prefix, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return prefix + value
}

func intPointer(value int) *int { return &value }
