package publicservice

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type aiUnlockEndpoint struct {
	allowedHosts []string
}

func aiUnlockRules() []Rule {
	return []Rule{
		{
			ServiceID: "chatgpt_unlock", Name: "ChatGPT（OpenAI）网页解锁", Category: "AI 服务", Region: "全球", ResultKind: "unlock", RuleVersion: 1,
			Description: "检查 ChatGPT 官方网页是否返回对话输入界面；不使用账号或发送消息。",
			TargetURL:   "https://chatgpt.com/", Method: http.MethodGet,
			SuccessCriterion: "单次访问官方网页；只有出现 ChatGPT 对话输入界面才记为解锁。地区拒绝、登录、挑战、限流或证据不足均不判为解锁；不代表账号或消息发送可用。",
			RedirectPolicy:   "probe_specific", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumWebDocumentBytes,
			Accept: "text/html,application/xhtml+xml",
		},
		{
			ServiceID: "claude_unlock", Name: "Claude（Anthropic）网页解锁", Category: "AI 服务", Region: "全球", ResultKind: "unlock", RuleVersion: 1,
			Description: "检查 Claude 官方网页是否返回对话输入界面；不使用账号或发送消息。",
			TargetURL:   "https://claude.ai/new", Method: http.MethodGet,
			SuccessCriterion: "单次访问官方网页；只有出现 Claude 对话输入界面才记为解锁。地区拒绝、登录、挑战、限流或证据不足均不判为解锁；不代表账号或消息发送可用。",
			RedirectPolicy:   "probe_specific", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumWebDocumentBytes,
			Accept: "text/html,application/xhtml+xml",
		},
		{
			ServiceID: "gemini_unlock", Name: "Gemini（Google）网页解锁", Category: "AI 服务", Region: "全球", ResultKind: "unlock", RuleVersion: 1,
			Description: "检查 Gemini 官方网页是否返回对话输入界面；不使用账号或发送消息。",
			TargetURL:   "https://gemini.google.com/app", Method: http.MethodGet,
			SuccessCriterion: "单次访问官方网页；只有出现 Gemini 对话输入界面才记为解锁。地区拒绝、登录、挑战、限流或证据不足均不判为解锁；不代表账号或消息发送可用。",
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

func checkAIUnlock(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result := Result{Details: map[string]string{}}
	endpoint, ok := aiUnlockEndpointFor(rule.ServiceID)
	if !ok {
		result.Outcome, result.Summary, result.FailurePhase = "unknown", "无法识别 AI 服务规则", "rule_config"
		return finish(result, started)
	}
	page, fetched := fetchAIUnlockPage(ctx, client, rule, endpoint, &result)
	if !fetched {
		return finish(result, started)
	}

	body := strings.ToLower(string(page.body))
	if result.Outcome == "challenge" || result.Outcome == "suspected_challenge" {
		return finish(result, started)
	}
	if aiUnlockRegionRefusal(rule.ServiceID, page.finalURL, body) {
		result.Outcome, result.Summary = "region_blocked", "厂商明确拒绝当前地区访问"
		result.ErrorMessage = "服务页面明确提示当前地区不可用"
		return finish(result, started)
	}
	if page.status == http.StatusUnauthorized || page.status == http.StatusProxyAuthRequired {
		result.Outcome, result.Summary = "auth_failed", "网页要求登录；未验证服务可用"
		result.ErrorMessage = "未使用账号凭据，无法确认该节点的 AI 服务可用性"
		return finish(result, started)
	}
	if page.status < 200 || page.status >= 300 {
		result.Outcome, result.Summary = "unknown", "网页未提供足够的服务可用性证据"
		result.FailurePhase = "http_status"
		result.ErrorMessage = fmt.Sprintf("HTTP %d 未能确认地区或对话入口可用", page.status)
		return finish(result, started)
	}
	if !aiUnlockComposerVisible(rule.ServiceID, body) {
		if aiUnlockAuthRequired(body) {
			result.Outcome, result.Summary = "auth_failed", "网页要求登录；未验证服务可用"
			result.ErrorMessage = "未使用账号凭据，无法确认该节点的 AI 服务可用性"
			return finish(result, started)
		}
		result.Outcome, result.Summary = "unknown", "网页未出现可确认的 AI 对话输入界面"
		result.FailurePhase = "service_evidence"
		result.ErrorMessage = "仅有网页响应不足以确认 AI 服务入口可用"
		return finish(result, started)
	}

	result.Outcome, result.Summary = "unlocked", "官方 AI 对话输入界面可用；未验证账号或消息发送"
	return finish(result, started)
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

func aiUnlockComposerVisible(serviceID, body string) bool {
	hasComposer := strings.Contains(body, "<textarea") || strings.Contains(body, "contenteditable") || strings.Contains(body, `role="textbox"`)
	if !hasComposer {
		return false
	}
	switch serviceID {
	case "chatgpt_unlock":
		return strings.Contains(body, "chat with chatgpt")
	case "claude_unlock":
		return strings.Contains(body, "claude") && strings.Contains(body, "how can i help you today?")
	case "gemini_unlock":
		return strings.Contains(body, "gemini") && strings.Contains(body, "ask gemini")
	default:
		return false
	}
}

func aiUnlockAuthRequired(body string) bool {
	return strings.Contains(body, "sign in") || strings.Contains(body, "log in") || strings.Contains(body, "login") || strings.Contains(body, "authentication required")
}
