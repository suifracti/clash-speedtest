package profiles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/net/http/httpproxy"
)

func TestSubscriptionNormalProxySelection(t *testing.T) {
	for _, tc := range []struct {
		name, target, envHTTP, envHTTPS, noProxy, want, policy string
		systemCalls                                            int
	}{
		{"environment HTTPS first", "https://sub.example/secret?token=fixture", "", "http://env-proxy.invalid:81", "", "http://env-proxy.invalid:81", "environment_proxy", 0},
		{"environment HTTP first", "http://sub.example/", "http://env-proxy.invalid:82", "", "", "http://env-proxy.invalid:82", "environment_proxy", 0},
		{"system HTTPS fallback", "https://sub.example/", "", "", "", "http://system-proxy.invalid:83", "system_proxy", 1},
		{"NO_PROXY domain", "https://sub.example/", "", "", ".example", "", "direct_normal_network", 0},
		{"NO_PROXY CIDR", "https://192.0.2.10/", "", "", "192.0.2.0/24", "", "direct_normal_network", 0},
		{"loopback", "http://127.0.0.1/", "", "", "", "", "direct_normal_network", 0},
		{"localhost", "http://localhost/", "", "", "", "", "direct_normal_network", 0},
		{"IPv6 loopback", "http://[::1]/", "", "", "", "", "direct_normal_network", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, tc.target, nil)
			evidence := &FetchEvidence{}
			calls := 0
			cfg := &httpproxy.Config{HTTPProxy: tc.envHTTP, HTTPSProxy: tc.envHTTPS, NoProxy: tc.noProxy}
			proxy := normalSubscriptionProxy(cfg, func(context.Context, string) (*url.URL, error) {
				calls++
				return url.Parse("http://system-proxy.invalid:83")
			}, func(s string) { evidence.ProxyPolicy = s })
			got, err := proxy(req)
			if err != nil {
				t.Fatal(err)
			}
			value := ""
			if got != nil {
				value = got.String()
			}
			if value != tc.want || calls != tc.systemCalls || evidence.ProxyPolicy != tc.policy {
				t.Fatalf("proxy=%s calls=%d policy=%s", value, calls, evidence.ProxyPolicy)
			}
			b, _ := json.Marshal(evidence)
			if strings.Contains(string(b), "token") || strings.Contains(string(b), "proxy.invalid") {
				t.Fatal("sensitive URL leaked to fetch evidence")
			}
		})
	}
}

func TestSubscriptionSystemProxyFailureDoesNotFallBackSilently(t *testing.T) {
	expected := errors.New("fixture failure")
	req, _ := http.NewRequest(http.MethodGet, "https://sub.example", nil)
	resolver := normalSubscriptionProxy(&httpproxy.Config{}, func(context.Context, string) (*url.URL, error) { return nil, expected }, nil)
	proxy, err := resolver(req)
	if proxy != nil || !errors.Is(err, expected) {
		t.Fatal("system path failure swallowed")
	}
}

func TestSubscriptionSystemProxyKeepsBoundedGETAndRedirectPolicy(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Host != "sub.example" || r.URL.RawQuery != "token=fixture-secret" || r.Method != http.MethodGet {
			t.Error("source request changed")
		}
		fmt.Fprint(w, "fixture body")
	}))
	defer server.Close()
	req, _ := http.NewRequest(http.MethodGet, "http://sub.example/source?token=fixture-secret", nil)
	e := &FetchEvidence{}
	client := subscriptionClient(req, e)
	client.Transport.(*subscriptionFallbackTransport).primary.Proxy = normalSubscriptionProxy(&httpproxy.Config{}, func(context.Context, string) (*url.URL, error) { return url.Parse(server.URL) }, func(p string) { e.ProxyPolicy = p })
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	client.CloseIdleConnections()
	if requests != 1 || e.RequestCount != 1 || e.ProxyPolicy != "system_proxy" {
		t.Fatalf("requests=%d evidence=%+v", requests, e)
	}
	via, _ := http.NewRequest(http.MethodGet, "https://sub.example", nil)
	next, _ := http.NewRequest(http.MethodGet, "http://sub.example", nil)
	if client.CheckRedirect(next, []*http.Request{via}) == nil {
		t.Fatal("HTTPS downgrade allowed")
	}
	if client.CheckRedirect(via, []*http.Request{via, via, via, via}) == nil {
		t.Fatal("unbounded redirects")
	}
}

func TestSubscriptionProxyFailureRetriesPhysicalDirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Subscription-Userinfo", "upload=1; download=2; total=100")
		fmt.Fprint(w, "subscription body")
	}))
	defer server.Close()
	proxy := httptest.NewServer(http.NotFoundHandler())
	proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL)
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	evidence := &FetchEvidence{}
	client := subscriptionClient(req, evidence)
	fallback := client.Transport.(*subscriptionFallbackTransport)
	fallback.primary.Proxy = func(*http.Request) (*url.URL, error) {
		evidence.ProxyPolicy = "system_proxy"
		return proxyURL, nil
	}
	fallback.physical = func() (*http.Client, error) { return server.Client(), nil }
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "subscription body" || evidence.ProxyPolicy != "physical_direct" || parseSubscriptionUsageHeaders(resp.Header) == nil {
		t.Fatalf("body=%s path=%s", body, evidence.ProxyPolicy)
	}
}
