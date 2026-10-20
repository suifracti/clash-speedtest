package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/profiles"
	"github.com/faceair/clash-speedtest/core/subscriptionusage"
)

func accountUsageFixture(t *testing.T, providerURL string, cached bool) (*Server, profiles.Paths, time.Time) {
	t.Helper()
	root := t.TempDir()
	paths := profiles.Paths{Dir: filepath.Join(root, "profiles")}
	at := time.Now().Add(-time.Hour).UTC()
	up, down, total := int64(0), int64(100), int64(6000)
	store := &profiles.Store{Airports: []*profiles.Airport{{ID: "a", Name: "Fixture", UpdatedAt: at, Subscriptions: []*profiles.Subscription{
		{ID: "one", Name: "One", URL: providerURL + "/one?token=FAKE_ACCOUNT_SECRET", UserAgent: "account-fixture", UpdatedAt: at, Usage: &profiles.SubscriptionUsage{Upload: &up, Download: &down, Total: &total, UpdatedAt: at}},
		{ID: "two", Name: "Two", URL: providerURL + "/two", UpdatedAt: at},
	}}}}
	if err := profiles.SaveStore(paths.StoreFile(), store); err != nil {
		t.Fatal(err)
	}
	if cached {
		if err := paths.WriteCache("one", []byte(validRefreshBody+"\n# original cache")); err != nil {
			t.Fatal(err)
		}
	}
	server, err := NewServer(ServerConfig{ProfilePaths: paths, HistoryDir: filepath.Join(root, "history"), UserAgent: "global-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	return server, paths, at
}

func TestRefreshOneAccountUsageKeepsNodesAndOtherAccounts(t *testing.T) {
	var requests atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.UserAgent() != "account-fixture" {
			t.Errorf("per-account UA ignored: %q", r.UserAgent())
		}
		w.Header().Set("Subscription-Userinfo", "upload=0;download=1100;total=6000")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer provider.Close()
	server, paths, at := accountUsageFixture(t, provider.URL, true)
	response := performRefreshRequest(server.Handler(), http.MethodPost, "/api/airports/a/subscriptions/one/usage/refresh", "")
	if response.Code != http.StatusOK {
		t.Fatalf("usage-only endpoint: %d %s", response.Code, response.Body.String())
	}
	var result struct {
		Subscription struct {
			ID        string                      `json:"id"`
			UpdatedAt time.Time                   `json:"updated_at"`
			Usage     *profiles.SubscriptionUsage `json:"usage"`
		} `json:"subscription"`
		Observation subscriptionusage.Snapshot `json:"observation"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Subscription.ID != "one" || !result.Subscription.UpdatedAt.Equal(at) || result.Subscription.Usage == nil || result.Subscription.Usage.Download == nil || *result.Subscription.Usage.Download != 1100 || result.Observation.Status != "ok" || result.Observation.Source != "account_usage_refresh" {
		t.Fatalf("usage-only result: %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "FAKE_ACCOUNT_SECRET") || strings.Contains(response.Body.String(), "/one?token=") {
		t.Fatal("usage-only response leaked source")
	}
	body, err := os.ReadFile(paths.CacheFile("one"))
	if err != nil || string(body) != validRefreshBody+"\n# original cache" {
		t.Fatalf("usage-only refresh changed nodes: %q %v", body, err)
	}
	store, err := profiles.LoadStore(paths.StoreFile())
	if err != nil {
		t.Fatal(err)
	}
	if !store.Get("a").UpdatedAt.Equal(at) || !store.Get("a").GetSubscription("one").UpdatedAt.Equal(at) || store.Get("a").GetSubscription("two").Usage != nil || requests.Load() != 1 {
		t.Fatalf("other state changed or fallback/body read: calls=%d", requests.Load())
	}
	day := time.Now().In(subscriptionusage.Location).Format("2006-01-02")
	report, err := server.app.GetSubscriptionUsage(context.Background(), day, day, "day", "")
	if err != nil {
		t.Fatal(err)
	}
	observed := false
	for _, interval := range report.Intervals {
		if interval.Bytes != nil && *interval.Bytes == 1000 {
			observed = true
		}
	}
	if len(report.Accounts) != 1 || !observed {
		t.Fatalf("usage change not recorded once: %+v", report)
	}
}

func TestAccountUsageOnlySupportsEmptyAndPartialResponsesWithoutCache(t *testing.T) {
	for _, test := range []struct{ name, header, want string }{{"empty body with counters", "upload=0;download=0;total=6000", "ok"}, {"partial counters", "download=1100;total=6000", "partial"}, {"no headers", "", "missing"}} {
		t.Run(test.name, func(t *testing.T) {
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if test.header != "" {
					w.Header().Set("Subscription-Userinfo", test.header)
				}
			}))
			defer provider.Close()
			server, paths, at := accountUsageFixture(t, provider.URL, false)
			response := performRefreshRequest(server.Handler(), http.MethodPost, "/api/airports/a/subscriptions/one/usage/refresh", "")
			if response.Code != http.StatusOK {
				t.Fatalf("usage endpoint: %d %s", response.Code, response.Body.String())
			}
			var result struct {
				Observation subscriptionusage.Snapshot `json:"observation"`
			}
			_ = json.Unmarshal(response.Body.Bytes(), &result)
			if result.Observation.Status != test.want {
				t.Fatalf("usage status=%q want %q", result.Observation.Status, test.want)
			}
			if test.want != "ok" && result.Observation.Upload != nil {
				t.Fatal("missing upload became zero")
			}
			if paths.HasCache("one") {
				t.Fatal("usage refresh created node cache")
			}
			saved, _ := profiles.LoadStore(paths.StoreFile())
			if !saved.Get("a").GetSubscription("one").UpdatedAt.Equal(at) {
				t.Fatal("usage refresh changed node timestamp")
			}
		})
	}
}
