package profiles

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// UsageFallback records optional header acquisition without replacing the
// primary node body or its fetch status/digest. It contains no source address.
type UsageFallback struct {
	UserAgent       string    `json:"user_agent"`
	HTTPStatus      int       `json:"http_status,omitempty"`
	ErrorCode       string    `json:"error_code,omitempty"`
	RetryAfterUntil time.Time `json:"retry_after_until,omitempty"`
}

func fetchUsageFallback(ctx context.Context, rawURL, userAgent string, usage *SubscriptionUsage) (*SubscriptionUsage, error) {
	ua := "clash.meta"
	if strings.Contains(strings.ToLower(userAgent), "clash.meta") {
		ua = "mihomo"
	}
	observation := &UsageFallback{UserAgent: ua}
	evidence, _ := ctx.Value(fetchObserverKey{}).(*FetchEvidence)
	if evidence != nil {
		evidence.UsageFallback = observation
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(rawURL), nil)
	if err != nil {
		return usage, err
	}
	req.Header.Set("User-Agent", ua)
	resp, err := subscriptionClient(req, evidence).Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return usage, ctx.Err()
		}
		observation.ErrorCode = "request_failed"
	} else {
		// GET works with providers that do not implement HEAD. Never consume its
		// body: even a streaming or invalid alternate node list cannot replace cache.
		defer resp.Body.Close()
		observation.HTTPStatus = resp.StatusCode
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			candidate := parseSubscriptionUsageHeaders(resp.Header)
			if usageHeaderScore(candidate) > usageHeaderScore(usage) {
				usage = candidate
			}
			if usageHeaderScore(usage) <= 0 {
				observation.ErrorCode = "missing_usage_header"
			}
		} else {
			observation.ErrorCode = "http_error"
			if resp.StatusCode == http.StatusTooManyRequests {
				observation.RetryAfterUntil = retryAfterUntil(resp.Header.Get("Retry-After"), time.Now())
			}
		}
	}
	if usage == nil && observation.ErrorCode != "missing_usage_header" {
		usage = &SubscriptionUsage{UpdatedAt: time.Now().UTC()}
	}
	if usage != nil {
		usage.Fallback = observation
	}
	if ctx.Err() != nil {
		return usage, ctx.Err()
	}
	return usage, nil
}
