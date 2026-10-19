package publicservice

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// Public resource targets were cross-checked against one-ip's site catalog.
// Requests use the existing selected-node client, never a remote Worker proxy.
func resourceRules() []Rule {
	specs := [][4]string{
		{"wechat", "微信", "社交与国内", "https://res.wx.qq.com/a/wx_fed/assets/res/NTI4MWU5.ico"},
		{"qq", "腾讯 QQ", "社交与国内", "https://www.qq.com/favicon.ico"},
		{"telegram", "Telegram", "社交与国内", "https://web.telegram.org/favicon.ico"},
		{"bilibili", "哔哩哔哩", "社交与国内", "https://www.bilibili.com/favicon.ico"},
		{"douyin", "抖音", "社交与国内", "https://lf3-static.bytednsdoc.com/obj/eden-cn/favicon.ico"},
		{"xiaohongshu", "小红书", "社交与国内", "https://fe-static.xhscdn.com/favicon.ico"},
		{"chatgpt", "ChatGPT", "AI 服务", "https://chatgpt.com/favicon.ico"},
		{"claude", "Claude", "AI 服务", "https://claude.ai/favicon.ico"},
		{"grok", "Grok", "AI 服务", "https://grok.com/favicon.ico"},
		{"gemini", "Gemini", "AI 服务", "https://gemini.google.com/robots.txt"},
		{"perplexity", "Perplexity", "AI 服务", "https://www.perplexity.ai/favicon.ico"},
		{"deepseek", "DeepSeek", "AI 服务", "https://chat.deepseek.com/favicon.ico"},
		{"qwen", "通义千问", "AI 服务", "https://chat.qwen.ai/favicon.ico"},
		{"kimi", "Kimi", "AI 服务", "https://www.kimi.com/favicon.ico"},
	}
	rules := make([]Rule, 0, len(specs))
	for _, s := range specs {
		rules = append(rules, Rule{ServiceID: "resource_" + s[0], Name: s[1] + " 资源连通性", Category: s[2], Region: "全球", ResultKind: "endpoint_reachability", RuleVersion: 1, TargetURL: s[3], Method: http.MethodGet, Description: "通过被测节点读取官方轻量资源；仅说明该资源可达，不代表登录、发消息或AI对话可用。", SuccessCriterion: "单次GET，HTTP 200，图标须为图像，robots须有User-agent字段；挑战、拒绝、跳转独立保留，不验证账号或业务", RedirectPolicy: "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes})
	}
	return rules
}

func checkResource(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result, body, ok := boundedGETStatuses(ctx, client, rule.TargetURL, rule.MaximumBodyBytes, []int{200, 403, 503})
	if !ok {
		if result.HTTPStatus != nil && *result.HTTPStatus >= 300 && *result.HTTPStatus < 400 {
			result.Outcome = "redirect"
			result.Summary = "资源返回跳转；本次未跟随，最终资源未确认"
		}
		return finish(result, started)
	}
	if classifyPageChallenge(&result, body) {
		return finish(result, started)
	}
	if result.HTTPStatus != nil && *result.HTTPStatus != 200 {
		result.Outcome = "http_rejected"
		result.FailurePhase = "http_status"
		result.ErrorMessage = "资源请求被HTTP拒绝；未发现明确挑战标记"
		return finish(result, started)
	}
	valid := strings.HasPrefix(http.DetectContentType(body), "image/")
	if strings.HasSuffix(rule.TargetURL, "/robots.txt") {
		valid = strings.Contains(strings.ToLower(string(body)), "user-agent:") && !strings.Contains(strings.ToLower(string(body)), "<html")
	}
	if !valid {
		result.Outcome = "criteria_mismatch"
		result.FailurePhase = "resource_document"
		result.ErrorMessage = "HTTP 200，但未取得预期资源；可能是登录页、错误页或空响应"
		return finish(result, started)
	}
	result.Outcome = "reachable"
	result.Summary = "资源可达；未验证账号、消息或AI对话"
	result.Details = map[string]string{"resource_url": rule.TargetURL, "scope": "仅当前资源端点，不代表完整业务可用", "source_catalog": "one-ip public target catalog"}
	return finish(result, started)
}
