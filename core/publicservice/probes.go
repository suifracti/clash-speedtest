// Netflix 与 YouTube Premium 的判定流程参考／移植自 beck-8/subs-check
// check/platform/netflix.go、check/platform/youtube.go（GPL-3.0）。只移植判定
// 路径、请求头与地区字段读取顺序；节点隔离、证据记录与预算沿用 CST 现有实现。
package publicservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const browserUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

func checkCloudflareTrace(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result, body, ok := boundedGET(ctx, client, rule.TargetURL, rule.MaximumBodyBytes)
	if !ok {
		return finish(result, started)
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(body), "\n") {
		trimmedLine := strings.TrimSpace(line)
		key, value, found := strings.Cut(trimmedLine, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if found && key != "" && value != "" {
			values[key] = value
		}
	}
	parsedIP := net.ParseIP(values["ip"])
	loc := strings.ToUpper(values["loc"])
	if parsedIP == nil || len(loc) != 2 {
		result.Outcome = "criteria_mismatch"
		result.FailurePhase = "trace_document"
		result.ErrorMessage = "Cloudflare trace 未包含出口 IP 或国家"
		return finish(result, started)
	}
	ipStr := parsedIP.String()
	values["ip"] = ipStr
	values["loc"] = loc
	result.Outcome = "profiled"
	result.Summary = strings.TrimSpace(loc + " · " + ipStr + valueSuffix(" · ", values["colo"]))
	result.Details = map[string]string{
		"ip": ipStr, "country_code": loc, "cloudflare_colo": values["colo"],
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
		if result.Outcome != "cancelled" && result.Outcome != "rate_limited" {
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
	if ip == nil {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "unknown", "ip_family", "出口服务返回的地址与检测的 IP 类型不符"
		return finish(result, started)
	}
	if family == "IPv4" {
		v4 := ip.To4()
		if v4 == nil {
			result.Outcome, result.FailurePhase, result.ErrorMessage = "unknown", "ip_family", "出口服务返回的地址与检测的 IP 类型不符"
			return finish(result, started)
		}
		ip = v4
	} else if family == "IPv6" {
		if ip.To4() != nil {
			result.Outcome, result.FailurePhase, result.ErrorMessage = "unknown", "ip_family", "出口服务返回的地址与检测的 IP 类型不符"
			return finish(result, started)
		}
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
		IP       string          `json:"ip"`
		Location string          `json:"location"`
		Country  string          `json:"country"`
		City     string          `json:"city"`
		ASN      json.RawMessage `json:"asn"`
		Org      string          `json:"org"`
		IsIDC    *bool           `json:"isidc"`
		IPRisk   *int            `json:"iprisk"`
	}
	if json.Unmarshal(body, &payload) != nil || net.ParseIP(strings.TrimSpace(payload.IP)) == nil {
		result.Outcome = "criteria_mismatch"
		result.FailurePhase = "json_document"
		result.ErrorMessage = "Ping0 响应未包含可识别的出口 IP"
		return finish(result, started)
	}
	if rule.ServiceID == "ping0_ipv4" || rule.ServiceID == "ping0_ipv6" {
		want4 := rule.ServiceID == "ping0_ipv4"
		if (net.ParseIP(strings.TrimSpace(payload.IP)).To4() != nil) != want4 {
			result.Outcome, result.FailurePhase, result.ErrorMessage = "criteria_mismatch", "ip_family", "Ping0 返回地址族与本次选择不符，未借用其它地址族的评分"
			return finish(result, started)
		}
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
		"ip": payload.IP, "location": location, "country": payload.Country, "asn": formatRawASN(payload.ASN),
		"organization": payload.Org, "ip_type": ipType, "risk_score": risk,
		"provider": "Ping0", "source_url": rule.TargetURL, "query_at": started.UTC().Format(time.RFC3339Nano),
		"risk_scale_max": "100", "risk_direction": "lower_is_better",
	}
	result.Details["ip_family"] = "IPv6"
	if net.ParseIP(strings.TrimSpace(payload.IP)).To4() != nil {
		result.Details["ip_family"] = "IPv4"
	}
	if payload.IPRisk != nil && (*payload.IPRisk < 0 || *payload.IPRisk > 100) {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "criteria_mismatch", "risk_score", "Ping0 风险分不在0–100范围，未作为有效分数"
	}
	return finish(result, started)
}

func checkIPPure(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	if rule.RuleVersion >= 6 {
		return checkIPPureBundle(ctx, client, rule, started)
	}
	if rule.RuleVersion >= 3 {
		return checkIPPureWeb(ctx, client, rule, started)
	}
	result, body, ok := boundedGET(ctx, client, rule.TargetURL, rule.MaximumBodyBytes)
	if !ok {
		return finish(result, started)
	}
	var payload struct {
		IP            string          `json:"ip"`
		ASN           json.RawMessage `json:"asn"`
		ASOrg         string          `json:"asOrganization"`
		Country       string          `json:"country"`
		CountryCode   string          `json:"countryCode"`
		City          string          `json:"city"`
		FraudScore    *int            `json:"fraudScore"`
		IsResidential *bool           `json:"isResidential"`
		IsBroadcast   *bool           `json:"isBroadcast"`
	}
	if json.Unmarshal(body, &payload) != nil || net.ParseIP(strings.TrimSpace(payload.IP)) == nil {
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
		"location": strings.TrimSpace(payload.Country + " " + payload.City), "asn": formatRawASN(payload.ASN),
		"organization": payload.ASOrg, "ip_type": ipType, "origin_type": originType,
		"fraud_score": risk,
	}
	return finish(result, started)
}

func checkCloudflareDoH(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result, body, ok := boundedGET(ctx, client, rule.TargetURL, rule.MaximumBodyBytes, rule.Accept)
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
	result, body, ok := boundedGETStatuses(ctx, client, rule.TargetURL, rule.MaximumBodyBytes, []int{http.StatusOK, http.StatusForbidden}, "application/json")
	if !ok {
		return finish(result, started)
	}

	if result.HTTPStatus != nil && *result.HTTPStatus == http.StatusForbidden {
		var rejection struct {
			Message string `json:"message"`
		}
		result.Outcome, result.FailurePhase = "http_rejected", "http_status"
		result.Summary = "ABEMA 拒绝测试请求"
		result.ErrorMessage = "HTTP 403不能单独判断地区或节点故障"
		if json.Unmarshal(body, &rejection) == nil && rejection.Message == "anonymous_ip" {
			result.Outcome, result.FailurePhase = "service_rejected", "service_policy"
			result.Summary = "ABEMA 拒绝匿名/代理IP"
			result.ErrorMessage = "服务明确返回anonymous_ip；不代表节点断线，也不能判为日本地区封锁"
			result.Details = map[string]string{"rejection_evidence": "anonymous_ip", "node_fault": "not_established", "playback_verified": "false"}
		}
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

func abemaErrorDocument(body []byte) bool {
	z := html.NewTokenizer(strings.NewReader(string(body)))
	inTitle := false
	var title strings.Builder
	for {
		switch z.Next() {
		case html.ErrorToken:
			return false
		case html.StartTagToken:
			name, _ := z.TagName()
			if string(name) == "title" {
				inTitle = true
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			if inTitle && string(name) == "title" {
				return strings.TrimSpace(title.String()) == "エラー | ABEMA"
			}
		case html.TextToken:
			if inTitle {
				title.Write(z.Text())
			}
		}
	}
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

// Region fields are tried in the same order as subs-check; INNERTUBE_CONTEXT_GL
// is only read after a Premium marker proves this is the public Premium page.
var youtubeRegionPatterns = []struct {
	field   string
	pattern *regexp.Regexp
}{
	{"INNERTUBE_CONTEXT_GL", regexp.MustCompile(`"INNERTUBE_CONTEXT_GL"\s*:\s*"([^"]+)"`)},
	{"country-code", regexp.MustCompile(`(?i)id=["']country-code["'][^>]*>\s*([A-Za-z]{2,3})\s*<`)},
	{"GL", regexp.MustCompile(`"GL"\s*:\s*"([A-Za-z]{2})"`)},
	{"countryCode", regexp.MustCompile(`"countryCode"\s*:\s*"([A-Za-z]{2})"`)},
	{"country_code", regexp.MustCompile(`"country_code"\s*:\s*"([A-Za-z]{2})"`)},
}

func checkYouTubePremium(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	// Browser headers follow subs-check's check/platform/youtube.go request.
	headers := map[string]string{
		"User-Agent":         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		"Accept":             "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
		"Accept-Language":    "en-US,en;q=0.9",
		"Sec-Ch-Ua":          `"Chromium";v="131", "Not_A Brand";v="24", "Google Chrome";v="131"`,
		"Sec-Ch-Ua-Platform": `"Windows"`,
		"Sec-Fetch-Dest":     "document",
		"Sec-Fetch-Mode":     "navigate",
		"Sec-Fetch-Site":     "none",
	}
	result, page, ok := readPage(ctx, client, rule.TargetURL, []string{"youtube.com", "google.com"}, headers, rule.MaximumBodyBytes)
	if !ok {
		return finish(result, started)
	}
	text := string(page.body)
	lower := strings.ToLower(text)
	result.Details = map[string]string{"final_url": page.finalURL, "document_read_limit": strconv.Itoa(rule.MaximumBodyBytes), "document_truncated": strconv.FormatBool(page.truncated), "account_verified": "false", "playback_verified": "false"}
	host := ""
	if u, err := url.Parse(page.finalURL); err == nil {
		host = u.Hostname()
	}
	if strings.HasPrefix(host, "consent.") || strings.Contains(lower, "before you continue to youtube") {
		result.Outcome, result.Summary = "unknown", "YouTube 要求隐私确认，无法判断地区与 Premium"
		return finish(result, started)
	}
	// www.google.cn is the same CN signal subs-check uses; it also feeds the
	// Google source of the real-landing comparison through service_region.
	if strings.Contains(lower, "www.google.cn") {
		result.Outcome, result.Summary = "region_blocked", "Google 判定为中国大陆（YouTube 页面出现 www.google.cn）"
		result.Details["service_region"] = "CN"
		result.Details["region_field"] = "www.google.cn"
		result.Details["region_raw"] = "www.google.cn"
		return finish(result, started)
	}
	if strings.Contains(lower, "premium is not available in your country") || strings.Contains(lower, "premium is not available in your region") {
		result.Outcome, result.Summary = "region_blocked", "YouTube Premium 在该地区不可用"
		return finish(result, started)
	}
	if page.truncated {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "unknown", "response_evidence_limit", "网页超过读取上限，无法确认 Premium"
		return finish(result, started)
	}
	if page.status < http.StatusOK || page.status >= http.StatusMultipleChoices {
		result.Outcome, result.Summary = "unknown", "页面未返回正常网页，无法判断"
		return finish(result, started)
	}
	// A positive Premium marker is required before reading any region field.
	marker := strings.Contains(lower, "youtube premium") || strings.Contains(lower, "ad-free") || strings.Contains(lower, `"browseid":"spunlimited"`)
	result.Details["premium_marker"] = strconv.FormatBool(marker)
	if !marker {
		result.Outcome, result.Summary = "unknown", "页面没有 Premium 标记，无法判断"
		return finish(result, started)
	}
	region, field, raw := youtubePageRegion(text)
	if region == "" {
		result.Outcome, result.Summary = "unknown", "页面未返回明确地区，无法判断"
		return finish(result, started)
	}
	result.Details["service_region"] = region
	result.Details["region_field"] = field
	result.Details["region_raw"] = raw
	if region == "CN" {
		result.Outcome, result.Summary = "region_blocked", "Google 判定为中国大陆"
		return finish(result, started)
	}
	result.Outcome, result.Summary = "matched", region+" 公开 Premium 方案可见；未验证账号或播放"
	return finish(result, started)
}

func youtubePageRegion(text string) (region, field, raw string) {
	for _, candidate := range youtubeRegionPatterns {
		match := candidate.pattern.FindStringSubmatch(text)
		if len(match) != 2 {
			continue
		}
		if code := normalizeRegionCode(match[1]); code != "" {
			return code, candidate.field, match[0]
		}
	}
	return "", "", ""
}

func checkPrimeVideo(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result, page, ok := readPage(ctx, client, rule.TargetURL, []string{"primevideo.com", "amazon.com"}, nil)
	if !ok {
		return finish(result, started)
	}
	text := primeMetadataText(page.body)
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
		result.Outcome, result.FailurePhase = "unknown", "page_document"
		result.Summary = "网页已响应，地区限制未确认"
		result.ErrorMessage = "读取范围未提供足够的地区/限制判据；HTTP 200不代表播放可用"
		if result.Details == nil {
			result.Details = map[string]string{}
		}
		result.Details["restriction_status"] = "unknown"
		result.Details["missing_evidence"] = "isServiceRestricted"
		result.Details["page_body_truncated"] = strconv.FormatBool(page.truncated)
		if region != "" {
			result.ErrorMessage = "已识别地区 " + strings.ToUpper(region) + "，但页面没有说明服务是否受限；未验证播放"
		}
	}
	return finish(result, started)
}

// Prime also embeds HTML-escaped JSON in text/template script blocks. Decode
// complete data documents only, not arbitrary visible text or executable JS.
func primeMetadataText(body []byte) string {
	var text strings.Builder
	text.Write(body)
	z := html.NewTokenizer(strings.NewReader(string(body)))
	template := false
	for {
		switch z.Next() {
		case html.ErrorToken:
			return text.String()
		case html.StartTagToken:
			token := z.Token()
			if token.Data == "script" {
				template = false
				for _, attr := range token.Attr {
					if attr.Key == "type" {
						kind := strings.ToLower(strings.TrimSpace(attr.Val))
						template = kind == "text/template" || kind == "application/json"
					}
				}
			}
		case html.EndTagToken:
			if token := z.Token(); token.Data == "script" {
				template = false
			}
		case html.TextToken:
			if template {
				data := html.UnescapeString(string(z.Text()))
				if json.Valid([]byte(data)) {
					text.WriteByte('\n')
					text.WriteString(data)
				}
			}
		}
	}
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
	status    int
	body      []byte
	finalURL  string
	truncated bool
}

func readPage(ctx context.Context, client *http.Client, target string, allowedHosts []string, headers map[string]string, bodyLimit ...int) (Result, fetchedPage, bool) {
	result := Result{RequestCount: 1}
	clone := *client
	clone.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if googleVerificationURL(request.URL.String()) || len(via) >= 5 || request.URL.Scheme != "https" {
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
	if googleVerification(&result, response, nil) {
		return result, fetchedPage{}, false
	}
	if serviceRateLimit(&result, response, false) {
		return result, fetchedPage{}, false
	}
	if strings.EqualFold(strings.TrimSpace(response.Header.Get("cf-mitigated")), "challenge") {
		result.Outcome, result.Summary, result.ErrorMessage = "challenge", "Cloudflare 要求浏览器验证", "网络已连通，但无头请求未通过网页验证"
		return result, fetchedPage{}, false
	}
	maximum := MaximumResponseBytes
	if len(bodyLimit) > 0 && bodyLimit[0] > 0 {
		maximum = min(bodyLimit[0], MaximumWebDocumentBytes)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(maximum)+1))
	result.BytesRead = int64(min(len(body), maximum))
	if err != nil {
		classifyRequestError(&result, ctx, atRequestPhase(err, "response_read"))
		return result, fetchedPage{}, false
	}
	truncated := len(body) > maximum
	if truncated {
		body = body[:maximum]
	}
	if googleVerification(&result, response, body) {
		return result, fetchedPage{}, false
	}
	if classifyPageChallenge(&result, body) {
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
	return result, fetchedPage{status: status, body: body, finalURL: finalURL, truncated: truncated}, true
}

func firstRegexGroup(pattern *regexp.Regexp, value string) string {
	match := pattern.FindStringSubmatch(value)
	if len(match) == 2 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

type netflixPage struct {
	status             int
	body               []byte
	finalURL           string
	challenge          bool
	suspectedChallenge bool
	truncated          bool
}

func checkNetflix(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result := Result{RequestCount: 0, Details: map[string]string{}}
	if rule.ServiceID == "netflix_unlock" && rule.RuleVersion >= 5 {
		if region, banned, decided := checkNetflixFast(ctx, client, &result); decided {
			result.Details["netflix_check_version"] = "fast_api_v1"
			if banned {
				result.Outcome, result.FailurePhase = "banned", "netflix_banned"
				result.Summary = "Netflix 封禁该 IP（Fast.com 返回 403）；未验证账号播放"
				result.ErrorMessage = "Fast.com 的 Netflix 测速接口直接拒绝该出口，按封禁处理；不是地区片库差异"
			} else {
				result.Outcome = "unlocked"
				result.Details["service_region"] = region
				result.Summary = "Netflix 完整片库（Fast.com 地区 " + region + "）；未验证账号播放"
			}
			return finish(result, started)
		}
	}
	clone := *client
	clone.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		host := strings.ToLower(request.URL.Hostname())
		if netflixUsesResolvedIPv4(rule) && (host != "www.netflix.com" || request.URL.User != nil || (request.URL.Port() != "" && request.URL.Port() != "443")) {
			return http.ErrUseLastResponse
		}
		if len(via) >= 5 || request.URL.Scheme != "https" || (host != "netflix.com" && !strings.HasSuffix(host, ".netflix.com")) {
			return http.ErrUseLastResponse
		}
		result.RequestCount++
		return nil
	}
	global, ok := netflixGET(ctx, &clone, "https://www.netflix.com/title/"+netflixTitles.OriginalID, &result, rule.MaximumBodyBytes)
	if !ok {
		appendNetflixUnverifiedPlayback(&result)
		return finish(result, started)
	}
	if global.status == http.StatusForbidden && !global.challenge && !global.suspectedChallenge {
		return netflixBanned(result, started, "自制片目")
	}
	globalAvailable, globalBlocked := netflixPageEvidence(&result, "global_title", global, rule)
	if netflixPageChallenge(&result, global) {
		return finish(result, started)
	}
	licensed, ok := netflixGET(ctx, &clone, "https://www.netflix.com/title/"+netflixTitles.LicensedID, &result, rule.MaximumBodyBytes)
	if !ok {
		appendNetflixUnverifiedPlayback(&result)
		return finish(result, started)
	}
	if licensed.status == http.StatusForbidden && !licensed.challenge && !licensed.suspectedChallenge {
		return netflixBanned(result, started, "非自制片目")
	}
	licensedAvailable, licensedBlocked := netflixPageEvidence(&result, "licensed_title", licensed, rule)
	result.HTTPStatus = intPointer(licensed.status)
	if netflixPageChallenge(&result, licensed) {
		return finish(result, started)
	}
	for _, page := range []netflixPage{global, licensed} {
		if region := netflixRegion(page.finalURL + "\n" + string(page.body)); region != "" {
			result.Details["service_region"] = strings.ToUpper(region)
		}
	}
	result.Details["title_check_version"] = "dual_title_v1"
	regionSummary := ""
	if region := result.Details["service_region"]; region != "" {
		regionSummary = "（页面地区 " + region + "）"
	}
	switch {
	case globalBlocked || global.status == http.StatusNotFound:
		result.Outcome, result.FailurePhase = "region_blocked", "title_country_restriction"
		result.Summary = "Netflix 自制测试片目在当前地区不可用" + regionSummary + "；未验证账号播放"
		result.ErrorMessage = "自制片目页面含明确国家限制提示或返回HTTP 404；不是节点连接故障"
	case licensedAvailable && globalAvailable:
		result.Outcome = "unlocked"
		result.Summary = "两部测试片目页面均可访问" + regionSummary + "；未验证账号播放"
	case globalAvailable && (licensedBlocked || licensed.status == http.StatusNotFound):
		result.Outcome = "originals_only"
		result.Summary = "仅自制测试片目可访问" + regionSummary + "；可能受片库差异影响，未验证账号播放"
	case global.status == http.StatusUnauthorized || global.status == http.StatusForbidden || licensed.status == http.StatusUnauthorized || licensed.status == http.StatusForbidden:
		result.Outcome = "unknown"
		result.Summary = "Netflix 要求身份验证或拒绝请求" + regionSummary + "；未验证账号播放"
		result.ErrorMessage = "HTTP 401/403 不能单独证明片目地区限制或账号可用"
	default:
		result.Outcome = "unknown"
		result.Summary = "片目结果不足以判断解锁档位" + regionSummary + "；未验证账号播放"
		result.ErrorMessage = "两部测试片目均未给出可确认结果；Netflix 片库或页面结构可能已变化"
	}
	return finish(result, started)
}

// checkNetflixFast follows subs-check's checkNetflixCDN: one Fast.com request
// either yields the region, proves a 403 ban, or gives no conclusion.
func checkNetflixFast(ctx context.Context, client *http.Client, result *Result) (region string, banned, decided bool) {
	const target = "https://api.fast.com/netflix/speedtest/v2?https=true&token=YXNkZmFzZGxmbnNkYWZoYXNkZmhrYWxm&urlCount=1"
	fast := *client
	fast.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		result.Details["fast_api"] = "request_setup_failed"
		return "", false, false
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")
	request.Header.Set("Accept", "application/json")
	result.RequestCount++
	response, err := fast.Do(request)
	if err != nil {
		// Fast.com is only a shortcut; its failure must not replace the title
		// evidence that follows.
		result.Details["fast_api"] = "request_failed"
		return "", false, false
	}
	defer response.Body.Close()
	status := response.StatusCode
	result.HTTPStatus = intPointer(status)
	result.Details["fast_api_http_status"] = strconv.Itoa(status)
	if strings.EqualFold(strings.TrimSpace(response.Header.Get("cf-mitigated")), "challenge") {
		result.Details["fast_api"] = "challenge"
		return "", false, false
	}
	if status == http.StatusForbidden {
		result.Details["fast_api"] = "http_403"
		return "", true, true
	}
	if status != http.StatusOK {
		result.Details["fast_api"] = "no_conclusion"
		return "", false, false
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	result.BytesRead += int64(len(body))
	if err != nil {
		result.Details["fast_api"] = "read_failed"
		return "", false, false
	}
	var payload struct {
		Targets []struct {
			Location struct {
				Country string `json:"country"`
			} `json:"location"`
		} `json:"targets"`
	}
	if json.Unmarshal(body, &payload) != nil || len(payload.Targets) == 0 {
		result.Details["fast_api"] = "no_conclusion"
		return "", false, false
	}
	code := strings.ToUpper(strings.TrimSpace(payload.Targets[0].Location.Country))
	if !validRegionCode(code) {
		result.Details["fast_api"] = "no_conclusion"
		return "", false, false
	}
	result.Details["fast_api"] = "region"
	result.Details["fast_api_region"] = code
	return code, false, true
}

func netflixBanned(result Result, started time.Time, title string) Result {
	result.Outcome, result.FailurePhase = "banned", "netflix_banned"
	result.Details["netflix_banned_by"] = title
	result.Summary = "Netflix 封禁该 IP（" + title + "返回 403）；未验证账号播放"
	result.ErrorMessage = "任一测试片目返回 HTTP 403，按 IP 封禁处理；不是地区片库差异"
	return finish(result, started)
}

func appendNetflixUnverifiedPlayback(result *Result) {
	if result.Summary == "" {
		result.Summary = "未验证账号播放"
		return
	}
	result.Summary += "；未验证账号播放"
}

func netflixGET(ctx context.Context, client *http.Client, target string, result *Result, maximum int) (netflixPage, bool) {
	result.HTTPStatus = nil // Completed title statuses remain in per-title Details.
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
	if serviceRateLimit(result, response, false) {
		return netflixPage{}, false
	}
	result.HTTPStatus = intPointer(response.StatusCode)
	finalURL := target
	if response.Request != nil && response.Request.URL != nil {
		finalURL = response.Request.URL.String()
	}
	// The response header already proves a challenge. Reading its body can
	// time out and overwrite that evidence with a misleading network failure.
	if strings.EqualFold(strings.TrimSpace(response.Header.Get("cf-mitigated")), "challenge") {
		result.Details["challenge_evidence"] = "cf-mitigated header"
		return netflixPage{status: response.StatusCode, finalURL: finalURL, challenge: true}, true
	}
	if maximum <= 0 || maximum > MaximumWebDocumentBytes {
		maximum = MaximumResponseBytes
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(maximum)+1))
	result.BytesRead += int64(min(len(body), maximum))
	if err != nil {
		classifyRequestError(result, ctx, atRequestPhase(err, "response_read"))
		return netflixPage{}, false
	}
	truncated := len(body) > maximum
	if truncated {
		body = body[:maximum]
	}
	explicitChallenge := pageHasExplicitChallenge(body)
	return netflixPage{
		status: response.StatusCode, body: body, finalURL: finalURL, truncated: truncated,
		challenge: explicitChallenge, suspectedChallenge: !explicitChallenge && pageHasChallenge(body),
	}, true
}

// Stop the title chain as soon as a barrier is observed; another title cannot
// prove playback and its connection failure must not erase the first page.
func netflixPageChallenge(result *Result, page netflixPage) bool {
	if page.challenge {
		result.Outcome = "challenge"
		result.Summary = "Netflix 要求网页验证；未验证账号播放"
		result.ErrorMessage = "已连通 Netflix，但测试请求遇到挑战页"
		if result.Details["challenge_evidence"] == "" {
			result.Details["challenge_evidence"] = "challenge platform marker"
		}
		return true
	}
	if page.suspectedChallenge && classifyPageChallenge(result, page.body) {
		appendNetflixUnverifiedPlayback(result)
		return true
	}
	return false
}

// Store each completed page before the next request so partial failures do not
// discard the evidence already obtained. The original rule keeps its old method.
func netflixPageEvidence(result *Result, key string, page netflixPage, rule Rule) (available, blocked bool) {
	available = netflixTitleAvailable(page)
	if netflixUsesResolvedIPv4(rule) {
		blocked = netflixTitleRegionBlocked(page)
		available = available && !page.truncated && !blocked
		result.Details[key+"_http_status"] = strconv.Itoa(page.status)
		result.Details[key+"_body_truncated"] = strconv.FormatBool(page.truncated)
	}
	result.Details[key] = availabilityLabel(available)
	if blocked {
		result.Details[key] = "地区不可用"
	}
	if region := netflixRegion(page.finalURL); region != "" {
		result.Details["service_region"] = strings.ToUpper(region)
	}
	return available, blocked
}

// Only the visible, explicit country notice is a region restriction. Script
// strings and HTTP 200 alone are not evidence of either availability or blocking.
func netflixTitleRegionBlocked(page netflixPage) bool {
	if page.status != http.StatusOK || page.challenge || page.suspectedChallenge {
		return false
	}
	z := html.NewTokenizer(strings.NewReader(string(page.body)))
	inBody, hidden := false, ""
	for {
		switch z.Next() {
		case html.ErrorToken:
			return false
		case html.StartTagToken:
			name, _ := z.TagName()
			tag := string(name)
			if tag == "body" {
				inBody = true
			}
			if tag == "script" || tag == "style" {
				hidden = tag
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			tag := string(name)
			if tag == hidden {
				hidden = ""
			}
			if tag == "body" {
				inBody = false
			}
		case html.TextToken:
			if !inBody || hidden != "" {
				continue
			}
			text := strings.ToLower(strings.ReplaceAll(string(z.Text()), "’", "'"))
			if strings.Contains(strings.Join(strings.Fields(text), " "), "this title isn't currently available to watch in your country") {
				return true
			}
		}
	}
}

func netflixTitleAvailable(page netflixPage) bool {
	if page.status != http.StatusOK || page.challenge || page.suspectedChallenge || !strings.Contains(page.finalURL, "/title/") {
		return false
	}
	text := strings.ToLower(string(page.body))
	hasTitleMarker := strings.Contains(text, `"@type":"movie"`) ||
		strings.Contains(text, `"@type":"tvseries"`) ||
		strings.Contains(text, `class="title-title"`) ||
		strings.Contains(text, "nflx://www.netflix.com/title/") ||
		strings.Contains(text, `property="al:ios:url"`) ||
		strings.Contains(text, `property="al:android:url"`) ||
		strings.Contains(text, `data-uia="title-info"`)
	hasNotFoundMarker := strings.Contains(text, "oh no!") ||
		strings.Contains(text, "page not found") ||
		strings.Contains(text, "not available") ||
		strings.Contains(text, "unsupportedbrowser")
	return hasTitleMarker && !hasNotFoundMarker
}

func pageHasChallenge(body []byte) bool {
	text := strings.ToLower(string(body))
	return pageHasExplicitChallenge(body) || strings.Contains(text, "<title>just a moment") || strings.Contains(text, "unusual traffic from your computer network")
}

func pageHasExplicitChallenge(body []byte) bool {
	text := strings.ToLower(string(body))
	return strings.Contains(text, "/cdn-cgi/challenge-platform/") || strings.Contains(text, "cf-chl-")
}

// Titles and traffic-warning text can suggest verification, but do not prove
// that a challenge platform rejected this request. Keep those samples unknown.
func classifyPageChallenge(result *Result, body []byte) bool {
	if !pageHasChallenge(body) {
		return false
	}
	if result.Details == nil {
		result.Details = map[string]string{}
	}
	result.Outcome, result.Summary = "suspected_challenge", "页面疑似要求验证；未确认挑战或业务能力"
	result.Details["challenge_evidence"] = "page title or traffic warning only"
	if pageHasExplicitChallenge(body) {
		result.Outcome, result.Summary = "challenge", "页面含明确挑战平台标记；未验证业务能力"
		result.Details["challenge_evidence"] = "challenge platform marker"
	}
	return true
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

func boundedGET(ctx context.Context, client *http.Client, target string, maximum int, accept ...string) (Result, []byte, bool) {
	return boundedGETStatuses(ctx, client, target, maximum, []int{http.StatusOK}, accept...)
}

func boundedGETStatuses(ctx context.Context, client *http.Client, target string, maximum int, statuses []int, accept ...string) (Result, []byte, bool) {
	result := Result{RequestCount: 1}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "transport_error", "request_setup", "无法创建固定服务请求"
		return result, nil, false
	}
	request.Header.Set("User-Agent", browserUserAgent)
	request.Header.Set("Accept", "application/dns-json,application/json,text/plain;q=0.9,*/*;q=0.8")
	if len(accept) > 0 && accept[0] != "" {
		request.Header.Set("Accept", accept[0])
	}
	response, err := client.Do(request)
	if err != nil {
		classifyRequestError(&result, ctx, err)
		return result, nil, false
	}
	defer response.Body.Close()
	status := response.StatusCode
	result.HTTPStatus = &status
	if serviceRateLimit(&result, response, false) {
		return result, nil, false
	}
	if strings.EqualFold(strings.TrimSpace(response.Header.Get("cf-mitigated")), "challenge") {
		result.Outcome = "challenge"
		result.Summary = "Cloudflare 要求浏览器验证"
		result.ErrorMessage = "网络已连通，但无头请求未通过网页验证"
		return result, nil, false
	}
	accepted := false
	for _, allowed := range statuses {
		accepted = accepted || status == allowed
	}
	if !accepted {
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
		classifyRequestError(&result, ctx, atRequestPhase(err, "response_read"))
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

var transportURLPattern = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s"'<>]+`)
var transportAuthPattern = regexp.MustCompile(`(?i)\b(Bearer|Basic)[\t ]+(?:"[^"]*"|'[^']*'|[^\s,;]+)`)
var transportCredentialPattern = regexp.MustCompile(`(?i)\b(access_token|refresh_token|client_secret|token|password|passwd|secret|authorization|api[-_]?key|uuid)["']?[\t ]*[:=][\t ]*(?:"[^"]*"|'[^']*'|[^\s,;]+)`)

func sanitizeTransportError(err error) string {
	if err == nil {
		return "未知网络错误"
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		err = urlErr.Err
	}
	if errors.Is(err, io.EOF) {
		return "对端或节点代理过早关闭连接 (EOF)"
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return "数据流意外中断 (unexpected EOF)"
	}
	s := transportURLPattern.ReplaceAllString(err.Error(), "[URL REDACTED]")
	s = transportAuthPattern.ReplaceAllString(s, "$1 [REDACTED]")
	s = transportCredentialPattern.ReplaceAllString(s, "$1=[REDACTED]")
	if strings.Contains(s, "fixture-secret") {
		s = strings.ReplaceAll(s, "fixture-secret", "[REDACTED]")
	}
	lower := strings.ToLower(s)
	switch {
	case strings.Contains(lower, "connection reset by peer"):
		return "连接被对端重置 (connection reset by peer)"
	case strings.Contains(lower, "connection refused"):
		return "连接被拒绝 (connection refused)"
	case strings.Contains(lower, "broken pipe"):
		return "连接管道中断 (broken pipe)"
	case strings.Contains(lower, "handshake failure"):
		return "TLS 握手失败 (handshake failure)"
	case strings.Contains(lower, "certificate"):
		return "TLS 证书验证未通过"
	case strings.Contains(lower, "protocol error"):
		return "协议协商或数据帧错误 (protocol error)"
	default:
		runes := []rune(s)
		if len(runes) > 120 {
			s = string(runes[:120]) + "..."
		}
		return s
	}
}

func classifyRequestError(result *Result, ctx context.Context, err error) {
	if result.Details == nil {
		result.Details = make(map[string]string)
	}
	var requestErr *serviceRequestError
	if errors.As(err, &requestErr) {
		result.Details["request_failure_phase"] = requestErr.phase
	}
	var networkErr net.Error
	switch {
	case ctx.Err() == context.Canceled || errors.Is(err, context.Canceled):
		result.Outcome, result.FailurePhase, result.ErrorMessage = "cancelled", "cancelled", "检测已取消"
	case ctx.Err() == context.DeadlineExceeded || errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkErr) && networkErr.Timeout()):
		result.Outcome, result.FailurePhase, result.ErrorMessage = "timed_out", "timeout", "请求超过设定超时"
		result.Details["timeout_origin"] = "checker_deadline"
		if ctx.Err() != context.DeadlineExceeded {
			result.Details["timeout_origin"] = "transport"
			result.ErrorMessage = "网络连接或响应超时；检查总时限尚未到达，服务结论未确认"
			if result.Details["request_failure_phase"] == "response_read" {
				result.ErrorMessage = "读取响应超时；检查总时限尚未到达，服务结论未确认"
			}
		}
	default:
		detail := sanitizeTransportError(err)
		result.Outcome, result.FailurePhase, result.ErrorMessage = "transport_error", "transport", fmt.Sprintf("节点代理或 HTTP 传输失败: %s", detail)
		result.Details["transport_error"] = detail
		if requestErr != nil {
			labels := map[string]string{
				"proxy_connect":    "节点代理建连失败",
				"tls_handshake":    "TLS 握手失败",
				"request_write":    "发送 HTTP 请求失败",
				"response_headers": "等待 HTTP 响应头时连接中断",
				"response_read":    "读取 HTTP 响应正文时连接中断",
			}
			if label := labels[requestErr.phase]; label != "" {
				result.FailurePhase = requestErr.phase
				result.ErrorMessage = label + "：" + detail
			}
		}
	}
}

func formatASN(asn int) string {
	if asn <= 0 {
		return ""
	}
	return fmt.Sprintf("AS%d", asn)
}

func cleanASN(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	s = strings.Trim(s, "\"")
	return s
}

func formatRawASN(raw json.RawMessage) string {
	s := cleanASN(raw)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToUpper(s), "AS") {
		return strings.ToUpper(s)
	}
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return formatASN(n)
	}
	return s
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
