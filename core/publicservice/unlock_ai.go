package publicservice

import (
	"context"
	_ "embed"
	"encoding/json"
	"golang.org/x/text/language"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
)

type aiUnlockEndpoint struct {
	allowedHosts []string
}

func aiUnlockRules() []Rule {
	return []Rule{
		{
			ServiceID: "chatgpt_unlock", Name: "ChatGPT（OpenAI）网页解锁", Category: "AI 服务", Region: "全球", ResultKind: "unlock", RuleVersion: 2,
			Description: "读取 ChatGPT trace 地区，配合官方地区可用接口判断；不登录或发送消息。",
			TargetURL:   "https://chatgpt.com/cdn-cgi/trace", Method: http.MethodGet,
			SuccessCriterion: "trace 提供 loc，官方 compliance/cookie_requirements 接口返回有效 JSON 且无地区拒绝；不验证账号或消息发送。",
			RedirectPolicy:   "probe_specific", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumWebDocumentBytes,
			Accept: "text/html,application/xhtml+xml",
		},
		{
			ServiceID: "claude_unlock", Name: "Claude（Anthropic）网页解锁", Category: "AI 服务", Region: "全球", ResultKind: "unlock", RuleVersion: 2,
			Description: "读取 Claude trace 的 loc，对照官方支持地区；不登录或发送消息。",
			TargetURL:   "https://claude.ai/cdn-cgi/trace", Method: http.MethodGet,
			SuccessCriterion: "trace 返回 loc 且在官方支持地区列表中；不验证账号或消息发送。",
			RedirectPolicy:   "probe_specific", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumWebDocumentBytes,
			Accept: "text/html,application/xhtml+xml",
		},
		{
			ServiceID: "gemini_unlock", Name: "Gemini（Google）网页解锁", Category: "AI 服务", Region: "全球", ResultKind: "unlock", RuleVersion: 2,
			Description: "读取 Gemini 页面明确的地区可用标记；不登录或发送消息。",
			TargetURL:   "https://gemini.google.com/", Method: http.MethodGet,
			SuccessCriterion: "Gemini 页面出现地区可用标记 45631641,null,true；缺失时未知，不验证账号或消息发送。",
			RedirectPolicy:   "probe_specific", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumWebDocumentBytes,
			Accept: "text/html,application/xhtml+xml",
		},
	}
}

func aiUnlockEndpointFor(serviceID string) (aiUnlockEndpoint, bool) {
	switch serviceID {
	case "chatgpt_unlock":
		return aiUnlockEndpoint{allowedHosts: []string{"chatgpt.com", "openai.com"}}, true
	case "claude_unlock":
		return aiUnlockEndpoint{allowedHosts: []string{"claude.ai", "anthropic.com"}}, true
	case "gemini_unlock":
		return aiUnlockEndpoint{allowedHosts: []string{"gemini.google.com", "google.com"}}, true
	default:
		return aiUnlockEndpoint{}, false
	}
}

//go:embed claude_regions.json
var claudeRegionsJSON []byte
var claudeRegions = func() struct {
	CountryCodes []string `json:"country_codes"`
} {
	var v struct {
		CountryCodes []string `json:"country_codes"`
	}
	if err := json.Unmarshal(claudeRegionsJSON, &v); err != nil {
		panic(err)
	}
	return v
}()

func checkAIUnlock(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result := Result{Details: map[string]string{}}
	endpoint, _ := aiUnlockEndpointFor(rule.ServiceID)
	page, ok := fetchAIUnlockPage(ctx, client, rule, endpoint, &result)
	if !ok {
		return finish(result, started)
	}
	result.Details["final_url"] = page.finalURL
	if result.Outcome == "challenge" || result.Outcome == "suspected_challenge" {
		return finish(result, started)
	}
	body := string(page.body)
	if aiUnlockRegionRefusal(rule.ServiceID, page.finalURL, strings.ToLower(body)) {
		result.Outcome, result.Summary = "region_blocked", "服务明确提示当前地区不可用"
		return finish(result, started)
	}
	if rule.ServiceID == "gemini_unlock" {
		region := firstRegexGroup(regexp.MustCompile(`,2,1,200,"([A-Z]{3})"`), body)
		if region != "" {
			if code, err := language.ParseRegion(region); err == nil {
				result.Details["service_region"] = code.String()
			}
		}
		switch {
		case page.status == 200 && !page.truncated && regexp.MustCompile(`45631641,\s*null,\s*true`).MatchString(body):
			result.Outcome, result.Summary = "unlocked", "Gemini 地区可用标记通过；未验证账号或对话"
		default:
			result.Outcome, result.Summary = "unknown", "未取得 Gemini 明确可用标记，无法判断"
		}
		return finish(result, started)
	}
	loc := firstRegexGroup(regexp.MustCompile(`(?m)^loc=([A-Z]{2})\s*$`), body)
	if page.status != 200 || loc == "" {
		result.Outcome, result.Summary = "unknown", "未取得服务 trace 的 loc，无法判断"
		return finish(result, started)
	}
	result.Details["service_region"] = loc
	result.Details["region_raw"] = "loc=" + loc
	if rule.ServiceID == "claude_unlock" {
		if slices.Contains(claudeRegions.CountryCodes, loc) {
			result.Outcome, result.Summary = "unlocked", "Claude 官方支持该地区；未验证账号或对话"
		} else {
			result.Outcome, result.Summary = "region_blocked", "Claude 官方尚不支持该地区"
		}
		return finish(result, started)
	}
	apiRule := rule
	apiRule.TargetURL = "https://api.openai.com/compliance/cookie_requirements"
	apiRule.Accept = "application/json"
	apiResult := Result{Details: map[string]string{}}
	apiPage, fetched := fetchAIUnlockPage(ctx, client, apiRule, endpoint, &apiResult)
	apiResult.RequestCount += result.RequestCount
	apiResult.BytesRead += result.BytesRead
	apiResult.Details = result.Details
	if !fetched {
		return finish(apiResult, started)
	}
	apiResult.Details["availability_url"] = apiPage.finalURL
	var payload map[string]json.RawMessage
	valid := json.Unmarshal(apiPage.body, &payload) == nil && len(payload) > 0
	switch {
	case valid && strings.Contains(strings.ToLower(string(apiPage.body)), "unsupported_country"):
		apiResult.Outcome, apiResult.Summary = "region_blocked", "OpenAI 官方地区接口拒绝该地区"
	case valid && apiPage.status == 200 && !apiPage.truncated && payload["error"] == nil:
		apiResult.Outcome, apiResult.Summary = "unlocked", "OpenAI 官方地区接口允许访问；未验证账号或对话"
	default:
		apiResult.Outcome, apiResult.Summary = "unknown", "官方地区接口未返回明确结论，无法判断"
	}
	return finish(apiResult, started)
}

func fetchAIUnlockPage(ctx context.Context, client *http.Client, rule Rule, endpoint aiUnlockEndpoint, result *Result) (fetchedPage, bool) {
	result.RequestCount = 1
	clone := *client
	clone.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 || request.URL.Scheme != "https" {
			return http.ErrUseLastResponse
		}
		host := strings.ToLower(request.URL.Hostname())
		for _, allowed := range endpoint.allowedHosts {
			if host == allowed || strings.HasSuffix(host, "."+allowed) {
				result.RequestCount++
				return nil
			}
		}
		return http.ErrUseLastResponse
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rule.TargetURL, nil)
	if err != nil {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "transport_error", "request_setup", "无法创建固定 AI 网页请求"
		return fetchedPage{}, false
	}
	request.Header.Set("User-Agent", browserUserAgent)
	request.Header.Set("Accept", rule.Accept)
	request.Header.Set("Accept-Language", "en-US,en;q=0.9")
	response, err := clone.Do(request)
	if err != nil {
		classifyRequestError(result, ctx, err)
		return fetchedPage{}, false
	}
	defer response.Body.Close()

	status := response.StatusCode
	result.HTTPStatus = &status
	if serviceRateLimit(result, response, false) {
		return fetchedPage{}, false
	}
	if strings.EqualFold(strings.TrimSpace(response.Header.Get("cf-mitigated")), "challenge") {
		result.Outcome, result.Summary, result.ErrorMessage = "challenge", "Cloudflare 要求浏览器验证", "网页挑战未通过；未验证 AI 服务可用性"
		return fetchedPage{}, false
	}
	maximum := rule.MaximumBodyBytes
	if maximum <= 0 || maximum > MaximumWebDocumentBytes {
		maximum = MaximumWebDocumentBytes
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(maximum)+1))
	result.BytesRead = int64(min(len(body), maximum))
	if err != nil {
		classifyRequestError(result, ctx, atRequestPhase(err, "response_read"))
		return fetchedPage{}, false
	}
	truncated := len(body) > maximum
	if truncated {
		body = body[:maximum]
	}
	finalURL := rule.TargetURL
	if response.Request != nil && response.Request.URL != nil {
		finalURL = response.Request.URL.String()
	}
	page := fetchedPage{status: status, body: body, finalURL: finalURL, truncated: truncated}
	classifyPageChallenge(result, body)
	return page, true
}

func aiUnlockRegionRefusal(serviceID, finalURL, body string) bool {
	if serviceID == "claude_unlock" && strings.Contains(strings.ToLower(finalURL), "/app-unavailable-in-region") {
		return true
	}
	brand := map[string]string{
		"chatgpt_unlock": "chatgpt",
		"claude_unlock":  "claude",
		"gemini_unlock":  "gemini",
	}[serviceID]
	if brand == "" || !strings.Contains(body, brand) {
		return false
	}
	for _, phrase := range []string{
		"not available in your country",
		"not available in this country",
		"not available in your region",
		"not available in this region",
		"not supported in your country",
		"not supported in your region",
		"only available in certain regions",
		"unsupported region",
	} {
		if strings.Contains(body, phrase) {
			return true
		}
	}
	return false
}
