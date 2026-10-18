package publicservice

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// A quota response is evidence to stop sampling, not a failed node. Only retain
// normalized scheduling hints; arbitrary response header text is not evidence.
func serviceRateLimit(result *Result, response *http.Response, github bool) bool {
	if response.StatusCode != http.StatusTooManyRequests && !(github && response.StatusCode == http.StatusForbidden && strings.TrimSpace(response.Header.Get("X-RateLimit-Remaining")) == "0") {
		return false
	}
	status := response.StatusCode
	result.HTTPStatus = &status
	result.Outcome, result.FailurePhase = "rate_limited", "http_status"
	result.Summary = "目标限流；未确认服务能力，不判为节点故障"
	result.ErrorMessage = "目标限制了请求；已停止本次后续采样，不自动重试"
	if result.Details == nil {
		result.Details = map[string]string{}
	}
	result.Details["rate_limit_source"] = "http_429"
	if status == http.StatusForbidden {
		result.Details["rate_limit_source"] = "github_quota_zero"
	}
	value := strings.TrimSpace(response.Header.Get("Retry-After"))
	if seconds, err := strconv.ParseUint(value, 10, 32); err == nil {
		result.Details["retry_after_seconds"] = strconv.FormatUint(seconds, 10)
	} else if at, err := http.ParseTime(value); err == nil {
		result.Details["retry_after_at"] = at.UTC().Format(time.RFC3339)
	}
	if github {
		if reset, err := strconv.ParseUint(strings.TrimSpace(response.Header.Get("X-RateLimit-Reset")), 10, 32); err == nil && reset > 0 {
			result.Details["rate_limit_reset_at"] = time.Unix(int64(reset), 0).UTC().Format(time.RFC3339)
		}
	}
	return true
}
