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

	"github.com/faceair/clash-speedtest/core/speedtester"
	"golang.org/x/net/http/httpproxy"
)

const MaxSubscriptionBytes = 8 << 20

type FetchEvidence struct {
	UsageFallback   *UsageFallback `json:"usage_fallback,omitempty"`
	ProxyPolicy     string         `json:"proxy_policy,omitempty"`
	Method          string         `json:"method"`
	UserAgent       string         `json:"user_agent"`
	RequestCount    int            `json:"request_count"`
	RedirectCount   int            `json:"redirect_count"`
	BytesRead       int            `json:"bytes_read"`
	BodySHA256      string         `json:"body_sha256,omitempty"`
	HTTPStatus      int            `json:"http_status,omitempty"`
	RetryAfterUntil time.Time      `json:"retry_after_until,omitempty"`
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
	evidence, _ := ctx.Value(fetchObserverKey{}).(*FetchEvidence)
	if evidence == nil {
		evidence = &FetchEvidence{}
	}
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
	if evidence == nil {
		evidence = &FetchEvidence{}
	}
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		transport = &http.Transport{}
	}
	transport = transport.Clone()
	transport.DisableKeepAlives = true
	var mu sync.Mutex
	transport.Proxy = normalSubscriptionProxy(httpproxy.FromEnvironment(), func(ctx context.Context, scheme string) (*url.URL, error) {
		return normalSystemProxy(ctx, scheme)
	}, func(policy string) {
		if evidence != nil {
			mu.Lock()
			evidence.ProxyPolicy = policy
			mu.Unlock()
		}
	})
	trace := &httptrace.ClientTrace{WroteRequest: func(httptrace.WroteRequestInfo) {
		if evidence == nil {
			return
		}
		mu.Lock()
		evidence.RequestCount++
		mu.Unlock()
	}}
	*req = *req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	return &http.Client{Transport: &subscriptionFallbackTransport{primary: transport, evidence: evidence, physical: func() (*http.Client, error) { return speedtester.NewPhysicalHTTPClient(60 * time.Second) }}, Timeout: 60 * time.Second, CheckRedirect: func(next *http.Request, via []*http.Request) error {
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

// Subscription acquisition tries the normal network first. A nil proxy may mean
// an explicit bypass, so check NO_PROXY/loopback before the macOS fallback.
func normalSubscriptionProxy(config *httpproxy.Config, system func(context.Context, string) (*url.URL, error), observe func(string)) func(*http.Request) (*url.URL, error) {
	environment := config.ProxyFunc()
	bypassConfig := *config
	bypassConfig.HTTPProxy = "http://bypass-check.invalid:1"
	bypassConfig.HTTPSProxy = "http://bypass-check.invalid:1"
	bypassConfig.CGI = false
	bypass := bypassConfig.ProxyFunc()
	report := func(policy string) {
		if observe != nil {
			observe(policy)
		}
	}
	return func(req *http.Request) (*url.URL, error) {
		proxy, err := environment(req.URL)
		if err != nil {
			return nil, err
		}
		if proxy != nil {
			report("environment_proxy")
			return proxy, nil
		}
		check, err := bypass(req.URL)
		if err != nil {
			return nil, err
		}
		if check == nil {
			report("direct_normal_network")
			return nil, nil
		}
		proxy, err = system(req.Context(), req.URL.Scheme)
		if err != nil {
			report("system_proxy_unavailable")
			return nil, err
		}
		if proxy != nil {
			report("system_proxy")
		} else {
			report("direct_normal_network")
		}
		return proxy, nil
	}
}

type subscriptionFallbackTransport struct {
	primary  *http.Transport
	physical func() (*http.Client, error)
	evidence *FetchEvidence
}

func (t *subscriptionFallbackTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.evidence.ProxyPolicy != "physical_direct" {
		resp, err := t.primary.RoundTrip(req)
		if err == nil {
			return resp, nil
		}
		if req.Context().Err() != nil || (t.evidence.ProxyPolicy != "environment_proxy" && t.evidence.ProxyPolicy != "system_proxy" && t.evidence.ProxyPolicy != "direct_normal_network") {
			return nil, err
		}
	}
	t.evidence.ProxyPolicy = "physical_direct"
	client, err := t.physical()
	if err != nil {
		return nil, err
	}
	transport := client.Transport
	if transport == nil {
		transport = &http.Transport{Proxy: nil}
	}
	if httpTransport, ok := transport.(*http.Transport); ok {
		httpTransport.DisableKeepAlives = true
	}
	return transport.RoundTrip(req)
}
