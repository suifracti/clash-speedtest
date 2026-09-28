package profiles

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/metacubex/mihomo/constant"
)

func newAirportID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func IsHTTPURL(raw string) bool {
	lower := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func DefaultUserAgent() string {
	return constant.MihomoName + "/" + constant.Version
}

func FetchSubscription(rawURL, userAgent string) ([]byte, error) {
	body, _, err := fetchSubscriptionResponse(rawURL, userAgent)
	return body, err
}

func FetchSubscriptionWithUsage(rawURL, userAgent string) ([]byte, *SubscriptionUsage, error) {
	return FetchSubscriptionWithUsageContext(context.Background(), rawURL, userAgent)
}

func FetchSubscriptionWithUsageContext(ctx context.Context, rawURL, userAgent string) ([]byte, *SubscriptionUsage, error) {
	body, usage, err := fetchSubscriptionResponseContext(ctx, rawURL, userAgent)
	if err != nil || usage != nil {
		return body, usage, err
	}
	// Some providers expose usage only to subscription managers. Keep the
	// original response as the node-config authority; read only the headers
	// from this second request and discard its body.
	return body, fetchUsageHeaderContext(ctx, rawURL), nil
}

func fetchSubscriptionResponse(rawURL, userAgent string) ([]byte, *SubscriptionUsage, error) {
	return fetchSubscriptionResponseContext(context.Background(), rawURL, userAgent)
}

func fetchSubscriptionResponseContext(ctx context.Context, rawURL, userAgent string) ([]byte, *SubscriptionUsage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(rawURL), nil)
	if err != nil {
		return nil, nil, err
	}
	if userAgent == "" {
		userAgent = DefaultUserAgent()
	}
	req.Header.Set("User-Agent", userAgent)
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("http %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	if len(body) == 0 {
		return nil, nil, fmt.Errorf("empty subscription body")
	}
	return body, parseSubscriptionUsage(resp.Header.Get("Subscription-Userinfo")), nil
}

func fetchUsageHeader(rawURL string) *SubscriptionUsage {
	return fetchUsageHeaderContext(context.Background(), rawURL)
}

func fetchUsageHeaderContext(ctx context.Context, rawURL string) *SubscriptionUsage {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(rawURL), nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "clash.meta")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	return parseSubscriptionUsage(resp.Header.Get("Subscription-Userinfo"))
}

func RedactURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" {
		return compactDisplay(raw, 72)
	}
	query := parsed.Query()
	changed := false
	for key := range query {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "key") || strings.Contains(lower, "password") || lower == "auth" {
			query.Set(key, "REDACTED")
			changed = true
		}
	}
	if changed {
		parsed.RawQuery = query.Encode()
	}
	return compactDisplay(parsed.String(), 72)
}

func compactDisplay(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func ExpandLocalPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || IsHTTPURL(raw) {
		return raw
	}
	if strings.HasPrefix(raw, "~"+string(os.PathSeparator)) || raw == "~" || strings.HasPrefix(raw, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			raw = home + raw[1:]
		}
	}
	return raw
}
