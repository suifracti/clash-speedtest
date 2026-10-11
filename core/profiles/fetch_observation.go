package profiles

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const MaxSubscriptionBytes = 8 << 20

type FetchEvidence struct {
	Method          string    `json:"method"`
	UserAgent       string    `json:"user_agent"`
	RequestCount    int       `json:"request_count"`
	RedirectCount   int       `json:"redirect_count"`
	BytesRead       int       `json:"bytes_read"`
	BodySHA256      string    `json:"body_sha256,omitempty"`
	HTTPStatus      int       `json:"http_status,omitempty"`
	RetryAfterUntil time.Time `json:"retry_after_until,omitempty"`
}

type FetchHTTPError struct {
	Status          int
	RetryAfterUntil time.Time
}

func (e *FetchHTTPError) Error() string { return fmt.Sprintf("http %d", e.Status) }

type FetchBodyError struct{ Reason string }

func (e *FetchBodyError) Error() string { return e.Reason }

type fetchObserverKey struct{}

func WithFetchEvidence(ctx context.Context, evidence *FetchEvidence) context.Context {
	return context.WithValue(ctx, fetchObserverKey{}, evidence)
}

func FetchSubscriptionObserved(ctx context.Context, rawURL, userAgent string) ([]byte, *SubscriptionUsage, *FetchEvidence, error) {
	evidence := &FetchEvidence{}
	body, usage, err := FetchSubscriptionWithUsageContext(WithFetchEvidence(ctx, evidence), rawURL, userAgent)
	return body, usage, evidence, err
}

func retryAfterUntil(value string, now time.Time) time.Time {
	if seconds, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && seconds >= 0 && seconds <= 31_536_000 {
		return now.Add(time.Duration(seconds) * time.Second)
	}
	if at, err := http.ParseTime(value); err == nil && at.After(now) {
		return at
	}
	return now.Add(30 * time.Minute)
}

func subscriptionClient(req *http.Request, evidence *FetchEvidence) *http.Client {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		transport = &http.Transport{}
	}
	transport = transport.Clone()
	transport.DisableKeepAlives = true
	var mu sync.Mutex
	trace := &httptrace.ClientTrace{WroteRequest: func(httptrace.WroteRequestInfo) {
		if evidence == nil {
			return
		}
		mu.Lock()
		evidence.RequestCount++
		mu.Unlock()
	}}
	*req = *req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	return &http.Client{Transport: transport, Timeout: 60 * time.Second, CheckRedirect: func(next *http.Request, via []*http.Request) error {
		if evidence != nil {
			mu.Lock()
			evidence.RedirectCount++
			mu.Unlock()
		}
		if len(via) > 3 {
			return fmt.Errorf("subscription redirect limit exceeded")
		}
		if via[len(via)-1].URL.Scheme == "https" && next.URL.Scheme != "https" {
			return fmt.Errorf("subscription HTTPS downgrade refused")
		}
		return nil
	}}
}

func SourceOrigin(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}
