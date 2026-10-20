package profiles

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
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
	return "clash-verge/v2.5.8"
}

func FetchSubscription(rawURL, userAgent string) ([]byte, error) {
	body, _, err := fetchSubscriptionPrimaryContext(context.Background(), rawURL, userAgent)
	return body, err
}

func FetchSubscriptionWithUsage(rawURL, userAgent string) ([]byte, *SubscriptionUsage, error) {
	return FetchSubscriptionWithUsageContext(context.Background(), rawURL, userAgent)
}

func FetchSubscriptionWithUsageContext(ctx context.Context, rawURL, userAgent string) ([]byte, *SubscriptionUsage, error) {
	return fetchSubscriptionResponseContext(ctx, rawURL, userAgent)
}

func fetchSubscriptionResponseContext(ctx context.Context, rawURL, userAgent string) ([]byte, *SubscriptionUsage, error) {
	// Share one deadline between the primary download and the optional headers.
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	body, usage, err := fetchSubscriptionPrimaryContext(ctx, rawURL, userAgent)
	if err != nil || usageHeaderScore(usage) > 0 {
		return body, usage, err
	}
	usage, err = fetchUsageFallback(ctx, rawURL, userAgent, usage)
	return body, usage, err
}

func fetchSubscriptionPrimaryContext(ctx context.Context, rawURL, userAgent string) ([]byte, *SubscriptionUsage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(rawURL), nil)
	if err != nil {
		return nil, nil, err
	}
	if userAgent == "" {
		userAgent = DefaultUserAgent()
	}
	req.Header.Set("User-Agent", userAgent)
	evidence, _ := ctx.Value(fetchObserverKey{}).(*FetchEvidence)
	if evidence != nil {
		evidence.Method = http.MethodGet
		evidence.UserAgent = req.UserAgent()
	}
	client := subscriptionClient(req, evidence)
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if evidence != nil {
		evidence.HTTPStatus = resp.StatusCode
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retryAt := time.Time{}
		if resp.StatusCode == http.StatusTooManyRequests {
			retryAt = retryAfterUntil(resp.Header.Get("Retry-After"), time.Now())
			if evidence != nil {
				evidence.RetryAfterUntil = retryAt
			}
		}
		return nil, nil, &FetchHTTPError{Status: resp.StatusCode, RetryAfterUntil: retryAt}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxSubscriptionBytes+1))
	if evidence != nil {
		evidence.BytesRead = len(body)
	}
	if err != nil {
		return nil, nil, err
	}
	if len(body) > MaxSubscriptionBytes {
		return nil, nil, &FetchBodyError{Reason: "subscription response exceeds 8 MiB"}
	}
	if len(body) == 0 {
		return nil, nil, &FetchBodyError{Reason: "empty subscription body"}
	}
	if evidence != nil {
		evidence.BodySHA256 = fmt.Sprintf("%x", sha256.Sum256(body))
	}
	return body, parseSubscriptionUsageHeaders(resp.Header), nil
}

func RedactURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" {
		return compactDisplay(raw, 72)
	}
	changed := false
	if parsed.User != nil {
		if _, hasPassword := parsed.User.Password(); hasPassword {
			parsed.User = url.UserPassword(parsed.User.Username(), "REDACTED")
			changed = true
		} else if parsed.User.Username() != "" {
			parsed.User = url.User("REDACTED")
			changed = true
		}
	}
	query := parsed.Query()
	queryChanged := false
	for key := range query {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "key") || strings.Contains(lower, "password") || strings.Contains(lower, "secret") || lower == "auth" {
			query.Set(key, "REDACTED")
			queryChanged = true
			changed = true
		}
	}
	if !changed {
		return compactDisplay(raw, 72)
	}
	if queryChanged {
		parsed.RawQuery = query.Encode()
	}
	return compactDisplay(parsed.String(), 72)
}

func compactDisplay(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
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
