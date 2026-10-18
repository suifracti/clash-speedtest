package publicservice

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
)

type netflixDNSFixture struct {
	client               *http.Client
	mu                   sync.Mutex
	addresses            []string
	dnsCalls, titleCalls int
	dnsStatus            int
	titleStatus          int
	badAddress           bool
}

func newNetflixDNSFixture(t *testing.T) *netflixDNSFixture {
	t.Helper()
	f := &netflixDNSFixture{dnsStatus: 200}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: []string{"www.netflix.com", "cloudflare-dns.com"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || r.TLS.ServerName != r.Host {
			t.Error("original TLS identity/HTTP Host was changed")
		}
		f.mu.Lock()
		if r.Host == "cloudflare-dns.com" {
			f.dnsCalls++
			status := f.dnsStatus
			bad := f.badAddress
			f.mu.Unlock()
			if r.URL.Query().Get("name") != "www.netflix.com" || r.URL.Query().Get("type") != "A" || r.Header.Get("Accept") != "application/dns-json" {
				t.Error("wrong fixed DNS query")
			}
			if status != 200 {
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(status)
				return
			}
			ip := "8.8.8.8"
			if bad {
				ip = "127.0.0.1"
			}
			w.Header().Set("Content-Type", "application/dns-json")
			json.NewEncoder(w).Encode(map[string]any{"Status": 0, "Answer": []map[string]any{{"type": 1, "TTL": 300, "data": ip}}})
			return
		}
		f.titleCalls++
		titleStatus := f.titleStatus
		f.mu.Unlock()
		if r.Host != "www.netflix.com" {
			t.Errorf("unexpected target Host %s", r.Host)
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("credentials leaked")
		}
		if titleStatus != 0 {
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(titleStatus)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<html>nflx://www.netflix.com/title/fixture</html>")
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	t.Cleanup(server.Close)
	transport := &http.Transport{DisableKeepAlives: true, TLSClientConfig: &tls.Config{RootCAs: roots}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		f.mu.Lock()
		f.addresses = append(f.addresses, address)
		f.mu.Unlock()
		if address != "cloudflare-dns.com:443" && address != "8.8.8.8:443" && address != "www.netflix.com:443" {
			return nil, errors.New("unexpected fixture destination")
		}
		// All traffic ends at the in-process test server; public addresses are metadata only.
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}}
	f.client = &http.Client{Transport: transport, Timeout: 2 * time.Second}
	t.Cleanup(transport.CloseIdleConnections)
	return f
}

func TestNetflixIPv4OptionPreservesIdentityAndOriginalRule(t *testing.T) {
	rule, ok := RuleFor("netflix_unlock_ipv4")
	if !ok {
		t.Fatal("missing separate opt-in Netflix IPv4 rule")
	}
	original, _ := RuleFor("netflix_unlock")
	if rule.ServiceID == original.ServiceID || original.RuleVersion != 3 || rule.BatchDefault {
		t.Fatal("new mode replaced the original or became default")
	}
	f := newNetflixDNSFixture(t)
	c := Checker{DriftSamples: 4, ClientFactory: func(monitor.MonitoredNode, time.Duration) (*http.Client, error) { return f.client, nil }}
	r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, 2*time.Second)
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Outcome != "unlocked" || r.RequestCount != 9 || f.dnsCalls != 1 || f.titleCalls != 8 {
		t.Fatalf("outcome=%s requests=%d DNS=%d title=%d details=%v", r.Outcome, r.RequestCount, f.dnsCalls, f.titleCalls, r.Details)
	}
	if r.Details["target_dns_mode"] != "selected_node_doh_ipv4_v1" || r.Details["target_ipv4"] != "8.8.8.8" || r.Details["playback_verified"] != "false" {
		t.Fatalf("missing method evidence: %v", r.Details)
	}
	for i, address := range f.addresses {
		want := "8.8.8.8:443"
		if i == 0 {
			want = "cloudflare-dns.com:443"
		}
		if address != want {
			t.Fatalf("destination %d=%s want=%s", i, address, want)
		}
	}
}

func TestNetflixIPv4DNSFailureStopsBeforeTitleRequests(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		bad    bool
		want   string
	}{
		{"DNS rate limit", 429, false, "rate_limited"},
		{"DNS rejected", 403, false, "unknown"},
		{"unusable answer", 200, true, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule, ok := RuleFor("netflix_unlock_ipv4")
			if !ok {
				t.Fatal("missing opt-in rule")
			}
			f := newNetflixDNSFixture(t)
			f.dnsStatus = tc.status
			f.badAddress = tc.bad
			c := Checker{DriftSamples: 4, ClientFactory: func(monitor.MonitoredNode, time.Duration) (*http.Client, error) { return f.client, nil }}
			r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, 2*time.Second)
			f.mu.Lock()
			defer f.mu.Unlock()
			if r.Outcome != tc.want || f.dnsCalls != 1 || f.titleCalls != 0 || r.RequestCount != 1 || r.Details["target_http_attempts"] != "0" {
				t.Fatalf("outcome=%s DNS=%d title=%d requests=%d details=%v", r.Outcome, f.dnsCalls, f.titleCalls, r.RequestCount, r.Details)
			}
		})
	}
}

func TestNetflixIPv4PinIsFixedForThisAttemptAndReportsFreshness(t *testing.T) {
	e := &netflixIPv4Evidence{ip: "8.8.8.8", ttl: 1, expires: time.Now().Add(-time.Second), phases: &requestPhases{}}
	if address, err := e.address("www.netflix.com:443"); err != nil || address != "8.8.8.8:443" {
		t.Fatalf("address=%s err=%v", address, err)
	}
	for _, address := range []string{"other.netflix.com:443", "www.netflix.com:444", "127.0.0.1:443"} {
		if _, err := e.address(address); !errors.Is(err, errNetflixDNSHost) {
			t.Fatalf("changed target %s was accepted: %v", address, err)
		}
	}
	result := Result{}
	e.attach(&result)
	if result.Details["target_dns_answer_fresh_at_finish"] != "false" || result.Details["target_address_policy"] != "one_lookup_frozen_for_this_attempt" || result.Details["target_dns_cache_scope"] != "this_attempt_only" {
		t.Fatalf("old lookup falsely claimed current DNS freshness: %v", result.Details)
	}
}

func TestNetflixIPv4UnsupportedClientStopsBeforeDNS(t *testing.T) {
	rule, _ := RuleFor("netflix_unlock_ipv4")
	calls := 0
	c := Checker{ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
		calls++
		return fixtureResponse(req, 200, "", ""), nil
	})}
	r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, time.Second)
	if r.Outcome != "not_executed" || calls != 0 || r.RequestCount != 0 {
		t.Fatalf("unsupported transport used an uncontrolled path: calls=%d outcome=%s", calls, r.Outcome)
	}
}

// Shape from the saved public SG title page: large head, then the real title
// metadata and a visible country restriction. No captured page/tracking data.
func TestNetflixIPv4LargeTitlePageDoesNotHideCountryRestriction(t *testing.T) {
	rule, _ := RuleFor(netflixIPv4Service)
	body := `<html><head>` + strings.Repeat(" ", 360000) + `<meta property="al:ios:url" content="nflx://www.netflix.com/title/fixture"/></head><body><div>Oh no! This title isn’t currently available to watch in your country.</div></body></html>`
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := fixtureResponse(req, 200, "text/html", body)
		resp.Request.URL, _ = req.URL.Parse("https://www.netflix.com/sg" + req.URL.Path)
		return resp, nil
	})}
	r := checkNetflix(context.Background(), client, rule, time.Now())
	if r.Outcome != "region_blocked" || r.BytesRead != int64(2*len(body)) || r.Details["global_title"] != "地区不可用" || r.Details["service_region"] != "SG" {
		t.Fatalf("outcome=%s bytes=%d details=%v", r.Outcome, r.BytesRead, r.Details)
	}
}

func TestNetflixIPv4BoundedPageCannotClaimAvailabilityFromPrefix(t *testing.T) {
	rule, _ := RuleFor(netflixIPv4Service)
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `<html><meta property="al:ios:url" content="nflx://www.netflix.com/title/fixture"/>` + strings.Repeat(" ", 1048576)
		return fixtureResponse(req, 200, "text/html", body), nil
	})}
	r := checkNetflix(context.Background(), client, rule, time.Now())
	if r.Outcome != "unknown" || r.Details["global_title_body_truncated"] != "true" || r.BytesRead != 2*1048576 {
		t.Fatalf("outcome=%s bytes=%d details=%v", r.Outcome, r.BytesRead, r.Details)
	}
}

func TestNetflixIPv4RetainsFirstPageWhenSecondRequestFails(t *testing.T) {
	rule, _ := RuleFor(netflixIPv4Service)
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "70143836") {
			return nil, context.DeadlineExceeded
		}
		return fixtureResponse(req, 200, "text/html", `<html><body>Oh no! This title isn’t currently available to watch in your country.</body></html>`), nil
	})}
	r := checkNetflix(context.Background(), client, rule, time.Now())
	if r.Details["global_title"] != "地区不可用" || r.Details["global_title_http_status"] != "200" || r.Outcome == "unlocked" || r.Outcome == "region_blocked" {
		t.Fatalf("first page evidence was lost or partial evidence became a full conclusion: %+v", r)
	}
}

func TestNetflixCountryRestrictionRequiresVisibleNotice(t *testing.T) {
	page := netflixPage{status: 200, finalURL: "https://www.netflix.com/sg/title/fixture", body: []byte(`<html><head><script>var text="Oh no! This title isn’t currently available to watch in your country."</script></head><body><h1>Fixture</h1></body></html>`)}
	// A hidden script string must not become a visible region block.
	rule, _ := RuleFor(netflixIPv4Service)
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return fixtureResponse(req, 200, "text/html", string(page.body)), nil
	})}
	if checkNetflix(context.Background(), client, rule, time.Now()).Outcome == "region_blocked" {
		t.Fatal("script-only text became a country restriction")
	}
}

func TestDefaultNetflixAutomaticallyUsesSelectedNodeIPv4(t *testing.T) {
	rule, _ := RuleFor("netflix_unlock")
	f := newNetflixDNSFixture(t)
	c := Checker{DriftSamples: 1, ClientFactory: func(monitor.MonitoredNode, time.Duration) (*http.Client, error) { return f.client, nil }}
	r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, 2*time.Second)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dnsCalls != 1 || f.titleCalls != 2 || r.RequestCount != 3 || rule.RuleVersion != 3 || r.Details["target_dns_mode"] != "selected_node_doh_ipv4_v1" {
		t.Fatalf("default rule did not resolve through selected node: dns=%d title=%d version=%d details=%v", f.dnsCalls, f.titleCalls, rule.RuleVersion, r.Details)
	}
}

func TestDefaultNetflixTitleRateLimitStopsWithoutRetry(t *testing.T) {
	rule, _ := RuleFor("netflix_unlock")
	f := newNetflixDNSFixture(t)
	f.titleStatus = 429
	c := Checker{DriftSamples: 4, ClientFactory: func(monitor.MonitoredNode, time.Duration) (*http.Client, error) { return f.client, nil }}
	r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, 2*time.Second)
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Outcome != "rate_limited" || f.dnsCalls != 1 || f.titleCalls != 1 || r.RequestCount != 2 {
		t.Fatalf("rate limit was retried: dns=%d title=%d result=%+v", f.dnsCalls, f.titleCalls, r)
	}
}
