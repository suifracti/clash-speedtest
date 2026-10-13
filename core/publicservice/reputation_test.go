package publicservice

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/faceair/clash-speedtest/core/monitor"
	"net/http"
	"strings"
	"testing"
	"time"
)

type memoryReputation struct {
	cache    string
	at       time.Time
	reserved int
	cool     time.Time
}

func (m *memoryReputation) LoadIPReputation(context.Context, string, time.Time) (string, error) {
	return m.cache, nil
}
func (m *memoryReputation) SaveIPReputation(_ context.Context, _ string, data string, at time.Time) error {
	m.cache = data
	m.at = at
	return nil
}
func (m *memoryReputation) ReserveIPReputation(_ context.Context, at time.Time) (bool, error) {
	if m.reserved >= 15 || at.Before(m.cool) {
		return false, nil
	}
	m.reserved++
	return true, nil
}
func (m *memoryReputation) CoolIPReputation(_ context.Context, at time.Time) error {
	m.cool = at
	return nil
}
func TestIPOKQueriesObservedFamilyAndReusesProviderOnly(t *testing.T) {
	store := &memoryReputation{}
	calls := 0
	checker := Checker{ReputationStore: store, ClientFactory: fixtureClient(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host == "api.ipify.org" {
			return fixtureResponse(r, 200, "application/json", `{"ip":"8.8.8.8"}`), nil
		}
		if r.URL.Query().Get("ip") != "8.8.8.8" || r.Header.Get("Accept-Version") != "1" {
			t.Fatalf("wrong lookup %s", r.URL)
		}
		return fixtureResponse(r, 200, "application/json", `{"apiVersion":"1","geo":{"ip":"8.8.8.8","asn":15169,"asName":"Google"},"risk":25,"ipType":"hosting","signals":["hosting"],"sources":[{"source":"provider","observedAt":"2026-10-09T10:00:00Z"}]}`), nil
	})}
	rule, ok := RuleFor("ipok_ipv4")
	if !ok {
		t.Fatal("missing on-demand rule")
	}
	a := checker.Check(context.Background(), monitor.MonitoredNode{NodeIdentityKey: "one"}, rule, DefaultTimeout)
	b := checker.Check(context.Background(), monitor.MonitoredNode{NodeIdentityKey: "two"}, rule, DefaultTimeout)
	if a.Outcome != "profiled" || b.Outcome != "profiled" || calls != 3 || store.reserved != 1 || b.Details["cache_status"] != "hit" || b.Details["evidence_node_identity"] != "two" {
		t.Fatalf("cache/path %#v %#v calls=%d", a, b, calls)
	}
}
func TestIPOKFamilyMismatchDoesNotQueryProvider(t *testing.T) {
	calls := 0
	c := Checker{ReputationStore: &memoryReputation{}, ClientFactory: fixtureClient(func(r *http.Request) (*http.Response, error) {
		calls++
		return fixtureResponse(r, 200, "application/json", `{"ip":"2001:4860:4860::8888"}`), nil
	})}
	rule, _ := RuleFor("ipok_ipv4")
	r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	if r.Outcome == "profiled" || calls != 1 {
		t.Fatalf("family fallback %#v %d", r, calls)
	}
}
func TestIPOKRejectsWrongQueriedAddressAndHonors429(t *testing.T) {
	for _, status := range []int{200, 429} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			store := &memoryReputation{}
			c := Checker{ReputationStore: store, ClientFactory: fixtureClient(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "api.ipify.org" {
					return fixtureResponse(r, 200, "application/json", `{"ip":"8.8.8.8"}`), nil
				}
				response := fixtureResponse(r, status, "application/json", `{"geo":{"ip":"1.1.1.1"},"risk":0}`)
				response.Header.Set("Retry-After", "3600")
				return response, nil
			})}
			rule, _ := RuleFor("ipok_ipv4")
			result := c.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
			if result.Outcome == "profiled" || store.cache != "" {
				t.Fatalf("invalid provider data accepted: %#v", result)
			}
			if status == 429 && store.cool.Before(time.Now().Add(59*time.Minute)) {
				t.Fatal("Retry-After not persisted")
			}
		})
	}
}
func TestIPOKIPv6UsesIPv6ObservationAndExplicitAddress(t *testing.T) {
	store := &memoryReputation{}
	c := Checker{ReputationStore: store, ClientFactory: fixtureClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "api6.ipify.org" {
			return fixtureResponse(r, 200, "application/json", `{"ip":"2001:4860:4860::8888"}`), nil
		}
		if r.URL.Query().Get("ip") != "2001:4860:4860::8888" {
			t.Fatalf("wrong IPv6 query %s", r.URL)
		}
		return fixtureResponse(r, 200, "application/json", `{"apiVersion":"1","geo":{"ip":"2001:4860:4860::8888"},"risk":null}`), nil
	})}
	rule, _ := RuleFor("ipok_ipv6")
	r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	if r.Outcome != "profiled" || r.Details["ip_family"] != "IPv6" || r.Details["risk_score"] != "未提供" {
		t.Fatalf("IPv6 or missing risk fabricated: %#v", r)
	}
}

func TestIPOKPersistsExplicitlyExhaustedProviderQuota(t *testing.T) {
	store := &memoryReputation{}
	c := Checker{ReputationStore: store, ClientFactory: fixtureClient(func(r *http.Request) (*http.Response, error) {
		body := `{"apiVersion":"1","geo":{"ip":"8.8.8.8"},"quota":{"arbitraryLookup":{"remaining":0,"resetsInSeconds":3600}}}`
		if r.URL.Host == "api.ipify.org" {
			body = `{"ip":"8.8.8.8"}`
		}
		return fixtureResponse(r, 200, "application/json", body), nil
	})}
	rule, _ := RuleFor("ipok_ipv4")
	r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	if r.Outcome != "profiled" || store.cool.Before(time.Now().Add(59*time.Minute)) {
		t.Fatalf("known zero quota ignored %#v", r)
	}
}

// A denied preflight must skip even transport preparation (and hence DNS).
func (m *memoryReputation) CheckIPReputationBudget(_ context.Context, at time.Time) (bool, error) {
	return m.reserved < 15 && !at.Before(m.cool), nil
}

func TestIPOKPreflightSkipsAllNetworkWhenBlocked(t *testing.T) {
	for _, store := range []*memoryReputation{{reserved: 15}, {cool: time.Now().Add(time.Hour)}} {
		factoryCalls, httpCalls := 0, 0
		c := Checker{ReputationStore: store, ClientFactory: func(monitor.MonitoredNode, time.Duration) (*http.Client, error) {
			factoryCalls++
			return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				httpCalls++
				return fixtureResponse(r, 200, "application/json", `{"ip":"8.8.8.8"}`), nil
			})}, nil
		}}
		rule, _ := RuleFor("ipok_ipv4")
		r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
		if factoryCalls != 0 || httpCalls != 0 || r.RequestCount != 0 || r.Outcome != "not_executed" || r.FailurePhase != "provider_quota" {
			t.Fatalf("blocked preflight entered network: factory=%d http=%d result=%+v", factoryCalls, httpCalls, r)
		}
	}
}

func TestIPOKBoundedFailureEvidenceAndPrivacy(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		phase     string
		truncated string
	}{
		{"forbidden", 403, `{"error":"forbidden","token":"fixture-secret","subscription_url":"https://private.invalid/sub?token=fixture-token","request":{"Authorization":"Bearer fixture-bearer"}}`, "provider_http", "false"},
		{"limited", 429, `{"error":"quota exceeded","password":"fixture-password"}`, "provider_http", "false"},
		{"wrong_ip", 200, `{"apiVersion":"1","geo":{"ip":"1.1.1.1"},"risk":25}`, "provider_document", "false"},
		{"wrong_version", 200, `{"apiVersion":"2","geo":{"ip":"8.8.8.8"},"risk":25}`, "provider_document", "false"},
		{"too_large", 200, strings.Repeat("x", 65537), "response_limit", "true"},
		{"html_echo", 403, `<html>https://private.invalid/sub?token=fixture-html Authorization: Bearer fixture-htmlsecret</html>`, "provider_http", "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &memoryReputation{}
			c := Checker{ReputationStore: store, ClientFactory: fixtureClient(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "api.ipify.org" {
					return fixtureResponse(r, 200, "application/json", `{"ip":"8.8.8.8"}`), nil
				}
				rsp := fixtureResponse(r, tc.status, "application/json", tc.body)
				rsp.Header.Set("Retry-After", "3600")
				rsp.Header.Set("X-IPOK-Upstream", "stale-cache")
				rsp.Header.Set("Set-Cookie", "session=fixture-cookie")
				rsp.Header.Set("Authorization", "Bearer fixture-response-secret")
				return rsp, nil
			})}
			rule, _ := RuleFor("ipok_ipv4")
			r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
			if r.Outcome == "profiled" || store.cache != "" || r.FailurePhase != tc.phase || r.HTTPStatus == nil || *r.HTTPStatus != tc.status {
				t.Fatalf("wrong failure/cache: %+v", r)
			}
			captured := tc.body
			if len(captured) > 65536 {
				captured = captured[:65536]
			}
			hash := sha256.Sum256([]byte(captured))
			if r.Details["provider_response_body_sha256"] != fmt.Sprintf("%x", hash) || r.Details["provider_response_body_truncated"] != tc.truncated || r.Details["provider_response_body"] == "" || len(r.Details["provider_response_body"]) > 65536 {
				t.Fatalf("missing bounded evidence: %+v", r.Details)
			}
			blob, _ := json.Marshal(r)
			for _, forbidden := range []string{"fixture-secret", "fixture-token", "private.invalid", "fixture-bearer", "fixture-password", "fixture-htmlsecret", "fixture-cookie", "fixture-response-secret"} {
				if strings.Contains(string(blob), forbidden) {
					t.Fatalf("sensitive fixture value leaked: %s", forbidden)
				}
			}
			if !strings.Contains(r.Details["provider_response_headers"], "3600") || r.Details["provider_upstream"] != "stale-cache" {
				t.Fatal("necessary source headers lost")
			}
		})
	}
}

func TestIPOKSuccessFreshnessAndUpstreamSurviveCache(t *testing.T) {
	store := &memoryReputation{}
	calls := 0
	c := Checker{ReputationStore: store, ClientFactory: fixtureClient(func(r *http.Request) (*http.Response, error) {
		calls++
		body := `{"apiVersion":"1","geo":{"ip":"8.8.8.8","asn":15169},"sources":[{"source":"vendor","observedAt":"2026-10-09T10:00:00Z","token":"fixture-source-secret"}],"quota":{"arbitraryLookup":{"remaining":5,"resetsInSeconds":3600}},"debug":{"RawConfig":"fixture-config-secret"}}`
		if r.URL.Host == "api.ipify.org" {
			body = `{"ip":"8.8.8.8"}`
		}
		rsp := fixtureResponse(r, 200, "application/json", body)
		rsp.Header.Set("X-IPOK-Upstream", "stale-cache")
		return rsp, nil
	})}
	rule, _ := RuleFor("ipok_ipv4")
	a := c.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	b := c.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	if a.Outcome != "profiled" || b.Outcome != "profiled" || calls != 3 || b.Details["cache_status"] != "hit" || b.Details["provider_upstream"] != "stale-cache" || a.Details["query_at"] != b.Details["query_at"] {
		t.Fatalf("source/cache mismatch a=%+v b=%+v", a, b)
	}
	if !strings.Contains(b.Details["source_observations"], "2026-10-09T10:00:00Z") || !strings.Contains(b.Details["provider_quota"], `"remaining":5`) {
		t.Fatal("source freshness/quota lost")
	}
	blob, _ := json.Marshal(b)
	if strings.Contains(string(blob), "fixture-source-secret") || strings.Contains(string(blob), "fixture-config-secret") || strings.Contains(store.cache, "fixture-source-secret") {
		t.Fatal("success evidence/cache leaked unknown fields")
	}
}

type brokenPreflightStore struct{ memoryReputation }

func (*brokenPreflightStore) CheckIPReputationBudget(context.Context, time.Time) (bool, error) {
	return false, errors.New("fixture storage unavailable")
}

func TestIPOKMissingOrFailedBudgetStoreSkipsTransport(t *testing.T) {
	for _, store := range []ReputationStore{nil, &brokenPreflightStore{}} {
		factories := 0
		c := Checker{ReputationStore: store, ClientFactory: func(monitor.MonitoredNode, time.Duration) (*http.Client, error) {
			factories++
			return nil, errors.New("must not enter transport")
		}}
		rule, _ := RuleFor("ipok_ipv4")
		r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
		if factories != 0 || r.RequestCount != 0 || r.Outcome != "not_executed" || r.FailurePhase != "local_storage" {
			t.Fatalf("storage failure entered transport: %+v", r)
		}
	}
}

type failedCacheStore struct{ memoryReputation }

func (*failedCacheStore) SaveIPReputation(context.Context, string, string, time.Time) error {
	return errors.New("fixture cache write failure")
}

func TestIPOKCacheSaveFailureKeepsMeasuredEvidenceWithoutRetry(t *testing.T) {
	store := &failedCacheStore{}
	calls := 0
	c := Checker{ReputationStore: store, ClientFactory: fixtureClient(func(r *http.Request) (*http.Response, error) {
		calls++
		body := `{"apiVersion":"1","geo":{"ip":"8.8.8.8"},"sources":[{"observedAt":"2026-10-09T10:00:00Z"}]}`
		if r.URL.Host == "api.ipify.org" {
			body = `{"ip":"8.8.8.8"}`
		}
		return fixtureResponse(r, 200, "application/json", body), nil
	})}
	rule, _ := RuleFor("ipok_ipv4")
	r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	if r.Outcome != "profiled" || r.Details["cache_save_error"] == "" || r.Details["provider_response_body_sha256"] == "" || calls != 2 || store.cache != "" {
		t.Fatalf("cache failure lost measurement/retried %+v", r)
	}
}

func TestIPOKPrivacyRedactsSensitiveValuesInKnownFields(t *testing.T) {
	store := &memoryReputation{}
	c := Checker{ReputationStore: store, ClientFactory: fixtureClient(func(r *http.Request) (*http.Response, error) {
		body := `{"apiVersion":"1","geo":{"ip":"8.8.8.8","asName":"Bearer fixture-known-field","isp":"https://user:fixture-userinfo@private.invalid/sub?token=fixture-query"},"sources":[{"source":"https://private.invalid/api?token=fixture-source","observedAt":"2026-10-09T10:00:00Z"}],"signals":["Authorization: fixture-auth"],"error":"password=fixture-pass"}`
		if r.URL.Host == "api.ipify.org" {
			body = `{"ip":"8.8.8.8"}`
		}
		rsp := fixtureResponse(r, 200, "application/json", body)
		rsp.Header.Set("X-IPOK-Upstream", "Bearer fixture-header")
		return rsp, nil
	})}
	rule, _ := RuleFor("ipok_ipv4")
	r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	blob, _ := json.Marshal(r)
	for _, secret := range []string{"fixture-known-field", "fixture-userinfo", "fixture-query", "fixture-source", "fixture-auth", "fixture-pass", "fixture-header", "private.invalid"} {
		if strings.Contains(string(blob), secret) || strings.Contains(store.cache, secret) {
			t.Fatalf("known field privacy leak %s", secret)
		}
	}
	if r.Outcome != "profiled" || !strings.Contains(r.Details["source_observations"], "2026-10-09T10:00:00Z") {
		t.Fatal("privacy projection damaged observation")
	}
}

func TestIPOKProjectionOverflowDoesNotRestoreUnsafeSourceFields(t *testing.T) {
	store := &memoryReputation{}
	body := `{"apiVersion":"1","geo":{"ip":"8.8.8.8","asName":"Bearer fixture-overflow-secret"},"signals":[` + strings.Repeat(`"<",`, 9000) + `"<"]}`
	c := Checker{ReputationStore: store, ClientFactory: fixtureClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "api.ipify.org" {
			return fixtureResponse(r, 200, "application/json", `{"ip":"8.8.8.8"}`), nil
		}
		return fixtureResponse(r, 200, "application/json", body), nil
	})}
	rule, _ := RuleFor("ipok_ipv4")
	r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	blob, _ := json.Marshal(r)
	if strings.Contains(string(blob), "fixture-overflow-secret") || strings.Contains(store.cache, "fixture-overflow-secret") {
		t.Fatal("bounded projection restored unsanitized original document")
	}
	if r.Outcome != "criteria_mismatch" || r.FailurePhase != "response_evidence_limit" || store.cache != "" {
		t.Fatal("projection overflow became an effective profile")
	}
}
