package publicservice

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type cfChallengeTarget struct {
	Label string
	URL   string
}

var cloudflareChallengeTargets = []cfChallengeTarget{
	{Label: "ChatGPT", URL: "https://chatgpt.com/"},
	{Label: "Grok", URL: "https://grok.com/"},
	{Label: "Claude", URL: "https://claude.ai/"},
}

func cloudflareChallengeRule() Rule {
	return Rule{
		ServiceID:        "cloudflare_challenge",
		Name:             "Cloudflare 人机验证检测",
		Category:         "地区与人机验证",
		Region:           "全球",
		ResultKind:       "ip_quality",
		RuleVersion:      1,
		Description:      "探测代表性 Cloudflare 保护站点（ChatGPT、Grok、Claude）是否向当前出口弹出人机验证/盾（Managed Challenge / Turnstile），仅观察不绕过。",
		TargetURL:        "https://chatgpt.com/",
		Method:           http.MethodGet,
		SuccessCriterion: "依次以普通浏览器请求头访问 3 个代表性 Cloudflare 站点：全部直通判未触发 CF 盾，部分站点弹盾判需人机验证，多数或全部弹盾判易触发 CF 验证",
		RedirectPolicy:   "do_not_follow",
		TimeoutSeconds:   int(DefaultTimeout.Seconds()),
		MaximumBodyBytes: 16 * 1024,
	}
}

func isCloudflareChallengeSignal(resp *http.Response, body []byte) (triggered bool, evidence string) {
	if resp != nil && strings.EqualFold(strings.TrimSpace(resp.Header.Get("cf-mitigated")), "challenge") {
		return true, "cf-mitigated"
	}
	lower := strings.ToLower(string(body))
	switch {
	case strings.Contains(lower, "challenges.cloudflare.com/turnstile") || strings.Contains(lower, "cf-turnstile"):
		return true, "turnstile"
	case strings.Contains(lower, "/cdn-cgi/challenge-platform/") || strings.Contains(lower, "_cf_chl_opt") || strings.Contains(lower, "cf-chl-"):
		return true, "challenge-platform"
	case strings.Contains(lower, "<title>just a moment"):
		return true, "just-a-moment"
	default:
		return false, ""
	}
}

func checkCloudflareChallenge(ctx context.Context, client *http.Client, rule Rule, started time.Time) Result {
	result := Result{Details: map[string]string{
		"probe_note": "基于普通浏览器请求头探测代表性 Cloudflare 站点是否返回 Managed Challenge / Turnstile，仅记录不绕过",
	}}
	if client == nil {
		result.Outcome, result.FailurePhase, result.ErrorMessage = "transport_error", "proxy_setup", "无法建立节点隔离代理连接"
		return finish(result, started)
	}
	localClient := *client
	localClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	targets := cloudflareChallengeTargets
	var (
		responded   int
		passed      int
		challenged  int
		siteDetails []string
		evidences   []string
		firstErr    *Result
	)
	bodyCap := rule.MaximumBodyBytes
	if bodyCap <= 0 || bodyCap > 32*1024 {
		bodyCap = 16 * 1024
	}

	for _, target := range targets {
		if ctx.Err() != nil {
			if firstErr == nil {
				var errRes Result
				classifyRequestError(&errRes, ctx, ctx.Err())
				firstErr = &errRes
			}
			break
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.URL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", browserUserAgent)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")

		result.RequestCount++
		resp, err := localClient.Do(req)
		if err != nil {
			if firstErr == nil {
				var errRes Result
				classifyRequestError(&errRes, ctx, err)
				firstErr = &errRes
			}
			siteDetails = append(siteDetails, target.Label+": 连接失败")
			continue
		}
		status := resp.StatusCode
		result.HTTPStatus = &status
		if ray := resp.Header.Get("CF-Ray"); ray != "" {
			result.Details["cf_ray"] = ray
			if parts := strings.Split(ray, "-"); len(parts) > 1 && result.Details["cf_colo"] == "" {
				result.Details["cf_colo"] = strings.TrimSpace(parts[1])
			}
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, int64(bodyCap)))
		_ = resp.Body.Close()
		result.BytesRead += int64(len(body))
		if readErr != nil && len(body) == 0 {
			if firstErr == nil {
				var errRes Result
				classifyRequestError(&errRes, ctx, atRequestPhase(readErr, "response_read"))
				firstErr = &errRes
			}
			siteDetails = append(siteDetails, fmt.Sprintf("%s: 读取失败(%d)", target.Label, status))
			continue
		}

		responded++
		if triggered, ev := isCloudflareChallengeSignal(resp, body); triggered {
			challenged++
			siteDetails = append(siteDetails, fmt.Sprintf("%s: 弹盾(%d %s)", target.Label, status, ev))
			evidences = append(evidences, target.Label+":"+ev)
			continue
		}
		if status >= 200 && status < 400 {
			passed++
			siteDetails = append(siteDetails, fmt.Sprintf("%s: 直通(%d)", target.Label, status))
			continue
		}
		siteDetails = append(siteDetails, fmt.Sprintf("%s: HTTP %d", target.Label, status))
	}

	result.Details["cf_challenge_sites"] = strconv.Itoa(len(targets))
	result.Details["cf_responded_count"] = strconv.Itoa(responded)
	result.Details["cf_challenged_count"] = strconv.Itoa(challenged)
	result.Details["cf_passed_count"] = strconv.Itoa(passed)
	if len(siteDetails) > 0 {
		result.Details["cf_challenge_detail"] = strings.Join(siteDetails, " · ")
	}
	if len(evidences) > 0 {
		result.Details["challenge_evidence"] = strings.Join(evidences, ", ")
	}

	switch {
	case responded == 0 && firstErr != nil:
		result.Outcome = firstErr.Outcome
		result.FailurePhase = firstErr.FailurePhase
		result.ErrorMessage = firstErr.ErrorMessage
		for k, v := range firstErr.Details {
			result.Details[k] = v
		}
	case challenged == 0 && passed > 0:
		result.Outcome = "profiled"
		result.Summary = fmt.Sprintf("未触发 CF 人机验证（%d/%d 站点直通：%s）", passed, responded, result.Details["cf_challenge_detail"])
	case challenged > 0 && passed > 0 && challenged < passed:
		result.Outcome = "verification_required"
		result.FailurePhase = "cloudflare_challenge"
		result.Summary = fmt.Sprintf("部分站点触发 CF 人机验证（%d/%d 弹盾：%s）", challenged, responded, result.Details["cf_challenge_detail"])
		result.ErrorMessage = result.Summary
	case challenged > 0:
		result.Outcome = "challenge"
		result.FailurePhase = "cloudflare_challenge"
		result.Summary = fmt.Sprintf("易触发 CF 人机验证（%d/%d 站点弹盾：%s）", challenged, responded, result.Details["cf_challenge_detail"])
		result.ErrorMessage = result.Summary
	default:
		result.Outcome = "unknown"
		result.FailurePhase = "http_status"
		result.Summary = "未取得可确认的直通或 CF 验证信号"
		if detail := result.Details["cf_challenge_detail"]; detail != "" {
			result.ErrorMessage = detail
		}
	}
	return finish(result, started)
}
