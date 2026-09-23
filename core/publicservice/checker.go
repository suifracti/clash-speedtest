package publicservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
)

const (
	DefaultTimeout       = 10 * time.Second
	MaximumTimeout       = 30 * time.Second
	MaximumResponseBytes = 64 * 1024
	RuleVersion          = 1
)

// Rule is an immutable entry in the public-service catalog. The catalog is
// deliberately code-owned; callers cannot supply an arbitrary target URL.
type Rule struct {
	ServiceID        string `json:"service_id"`
	Name             string `json:"name"`
	RuleVersion      int    `json:"rule_version"`
	TargetURL        string `json:"target_url"`
	Method           string `json:"method"`
	SuccessCriterion string `json:"success_criterion"`
	RedirectPolicy   string `json:"redirect_policy"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
	MaximumBodyBytes int    `json:"maximum_body_bytes"`
	Accept           string `json:"accept,omitempty"`
	APIVersionHeader string `json:"api_version_header,omitempty"`
}

var catalog = []Rule{
	{
		ServiceID: "cloudflare_204", Name: "Cloudflare 204 连通性", RuleVersion: RuleVersion,
		TargetURL: "https://cp.cloudflare.com/generate_204", Method: http.MethodGet,
		SuccessCriterion: "HTTP status exactly 204; indicates only this target matched its connectivity rule",
		RedirectPolicy:   "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	{
		ServiceID: "google_204", Name: "Google 204 连通性", RuleVersion: RuleVersion,
		TargetURL: "https://www.google.com/generate_204", Method: http.MethodGet,
		SuccessCriterion: "HTTP status exactly 204; indicates only this target matched its connectivity rule",
		RedirectPolicy:   "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
	},
	{
		ServiceID: "github_api_root", Name: "GitHub 公共 API 根端点", RuleVersion: RuleVersion,
		TargetURL: "https://api.github.com/", Method: http.MethodGet,
		SuccessCriterion: "HTTP status exactly 200, JSON response, and root-index fields current_user_url and repository_url are non-empty",
		RedirectPolicy:   "do_not_follow", TimeoutSeconds: int(DefaultTimeout.Seconds()), MaximumBodyBytes: MaximumResponseBytes,
		Accept: "application/vnd.github+json", APIVersionHeader: "2026-03-10",
	},
}

func Catalog() []Rule {
	result := make([]Rule, len(catalog))
	copy(result, catalog)
	return result
}

func RuleFor(serviceID string) (Rule, bool) {
	for _, rule := range catalog {
		if rule.ServiceID == serviceID {
			return rule, true
		}
	}
	return Rule{}, false
}

type Result struct {
	Outcome      string    `json:"outcome"`
	HTTPStatus   *int      `json:"http_status,omitempty"`
	BytesRead    int64     `json:"bytes_read"`
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
	DurationMs   int64     `json:"duration_ms"`
	FailurePhase string    `json:"failure_phase,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
}

type ClientFactory func(node monitor.MonitoredNode, timeout time.Duration) (*http.Client, error)

// Checker issues exactly one request through the selected node's isolated
// proxy path. It has no cookie jar and adds no authentication or browser state.
type Checker struct {
	ClientFactory ClientFactory
}

func (c Checker) Check(ctx context.Context, node monitor.MonitoredNode, rule Rule, timeout time.Duration) Result {
	startedAt := time.Now().UTC()
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if timeout > MaximumTimeout {
		timeout = MaximumTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	factory := c.ClientFactory
	if factory == nil {
		factory = defaultClientFactory
	}
	client, err := factory(node, timeout)
	if err != nil {
		return finish(Result{Outcome: "transport_error", FailurePhase: "proxy_setup", ErrorMessage: "无法建立节点隔离代理连接"}, startedAt)
	}
	if client == nil {
		return finish(Result{Outcome: "transport_error", FailurePhase: "proxy_setup", ErrorMessage: "无法建立节点隔离代理连接"}, startedAt)
	}
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

	request, err := http.NewRequestWithContext(ctx, rule.Method, rule.TargetURL, nil)
	if err != nil {
		return finish(Result{Outcome: "transport_error", FailurePhase: "request_setup", ErrorMessage: "无法创建固定服务请求"}, startedAt)
	}
	request.Header.Set("User-Agent", "clash-speedtest")
	if rule.Accept != "" {
		request.Header.Set("Accept", rule.Accept)
	}
	if rule.APIVersionHeader != "" {
		request.Header.Set("X-GitHub-Api-Version", rule.APIVersionHeader)
	}

	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() == context.Canceled || err == context.Canceled {
			return finish(Result{Outcome: "cancelled", FailurePhase: "cancelled", ErrorMessage: "检测已取消"}, startedAt)
		}
		if ctx.Err() == context.DeadlineExceeded || err == context.DeadlineExceeded {
			return finish(Result{Outcome: "timed_out", FailurePhase: "timeout", ErrorMessage: "请求超过设定超时"}, startedAt)
		}
		if networkErr, ok := err.(net.Error); ok && networkErr.Timeout() {
			return finish(Result{Outcome: "timed_out", FailurePhase: "timeout", ErrorMessage: "请求超过设定超时"}, startedAt)
		}
		return finish(Result{Outcome: "transport_error", FailurePhase: "transport", ErrorMessage: "节点代理或 HTTP 传输失败；具体阶段未知"}, startedAt)
	}
	defer response.Body.Close()

	status := response.StatusCode
	result := Result{HTTPStatus: &status}
	if status >= 300 && status < 400 {
		result.Outcome = "redirect"
		result.FailurePhase = "http_status"
		result.ErrorMessage = "服务返回重定向；本规则不跟随重定向"
		return finish(result, startedAt)
	}
	if status == http.StatusTooManyRequests || (rule.ServiceID == "github_api_root" && status == http.StatusForbidden && strings.TrimSpace(response.Header.Get("X-RateLimit-Remaining")) == "0") {
		result.Outcome = "rate_limited"
		result.FailurePhase = "http_status"
		result.ErrorMessage = "响应状态或限流头表明目标限制了请求"
		return finish(result, startedAt)
	}

	if rule.ServiceID == "cloudflare_204" || rule.ServiceID == "google_204" {
		if status == http.StatusNoContent {
			result.Outcome = "matched"
			return finish(result, startedAt)
		}
		result.Outcome = "http_rejected"
		result.FailurePhase = "http_status"
		result.ErrorMessage = fmt.Sprintf("服务返回 HTTP %d；判据要求 HTTP 204", status)
		return finish(result, startedAt)
	}

	if status != http.StatusOK {
		result.Outcome = "http_rejected"
		result.FailurePhase = "http_status"
		result.ErrorMessage = fmt.Sprintf("服务返回 HTTP %d；判据要求 HTTP 200", status)
		return finish(result, startedAt)
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, MaximumResponseBytes))
	result.BytesRead = int64(len(body))
	if readErr != nil {
		var networkErr net.Error
		switch {
		case ctx.Err() == context.Canceled || errors.Is(readErr, context.Canceled):
			result.Outcome = "cancelled"
			result.FailurePhase = "cancelled"
			result.ErrorMessage = "检测已取消"
		case ctx.Err() == context.DeadlineExceeded || errors.Is(readErr, context.DeadlineExceeded) || (errors.As(readErr, &networkErr) && networkErr.Timeout()):
			result.Outcome = "timed_out"
			result.FailurePhase = "timeout"
			result.ErrorMessage = "请求超过设定超时"
		default:
			result.Outcome = "transport_error"
			result.FailurePhase = "response_read"
			result.ErrorMessage = "读取响应时发生传输错误"
		}
		return finish(result, startedAt)
	}
	if len(body) >= MaximumResponseBytes {
		result.Outcome = "criteria_mismatch"
		result.FailurePhase = "response_limit"
		result.ErrorMessage = "响应达到 64 KiB 读取上限，无法按规则确认"
		return finish(result, startedAt)
	}
	mediaType, _, mediaErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if mediaErr != nil || (mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json")) {
		result.Outcome = "criteria_mismatch"
		result.FailurePhase = "content_type"
		result.ErrorMessage = "响应内容类型不符合 JSON 判据"
		return finish(result, startedAt)
	}
	var root struct {
		CurrentUserURL string `json:"current_user_url"`
		RepositoryURL  string `json:"repository_url"`
	}
	if json.Unmarshal(body, &root) != nil || root.CurrentUserURL == "" || root.RepositoryURL == "" {
		result.Outcome = "criteria_mismatch"
		result.FailurePhase = "root_document"
		result.ErrorMessage = "响应未包含符合规则的 API 根索引"
		return finish(result, startedAt)
	}
	result.Outcome = "matched"
	return finish(result, startedAt)
}

func defaultClientFactory(node monitor.MonitoredNode, timeout time.Duration) (*http.Client, error) {
	return monitor.NewDefaultNodeDialer().CreateClient(node, timeout)
}

func finish(result Result, startedAt time.Time) Result {
	result.StartedAt = startedAt.UTC()
	result.FinishedAt = time.Now().UTC()
	result.DurationMs = result.FinishedAt.Sub(result.StartedAt).Milliseconds()
	return result
}
