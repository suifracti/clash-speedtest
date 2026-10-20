package application

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/profiles"
)

func TestUsageHeaderFallbackKeepsNodeCacheAndOriginCooldown(t *testing.T) {
	var mode atomic.Bool
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !mode.Load() {
			w.Header().Set("Subscription-Userinfo", "upload=0;download=10;total=100")
			_, _ = w.Write([]byte("proxies: [] # original nodes"))
			return
		}
		if r.UserAgent() == "clash.meta" {
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte("must never become node cache"))
			return
		}
		_, _ = w.Write([]byte("proxies: [] # primary nodes"))
	}))
	defer server.Close()
	s, paths := newAirportURLTestService(t)
	airport, err := s.CreateAirport("Fallback fixture", server.URL+"/one", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddSubscription(airport.ID, "Second", server.URL+"/two", "", "fixture"); err != nil {
		t.Fatal(err)
	}
	mode.Store(true)
	calls.Store(0)
	state, err := s.collectSubscriptionUsage(context.Background(), time.Now(), "fixture", true)
	if err != nil {
		t.Fatal(err)
	}
	if state.State != "completed_with_issues" || calls.Load() != 2 {
		t.Fatalf("fallback cooldown did not skip next same-origin account: state=%+v calls=%d", state, calls.Load())
	}
	body, err := os.ReadFile(paths.CacheFile(airport.Subscriptions[0].ID))
	if err != nil || string(body) != "proxies: [] # primary nodes" {
		t.Fatalf("optional header failure replaced/discarded primary cache: %q %v", body, err)
	}
	store, err := profiles.LoadStore(paths.StoreFile())
	if err != nil {
		t.Fatal(err)
	}
	first := store.Get(airport.ID).Subscriptions[0]
	if first.Usage == nil || first.Usage.Fallback == nil || first.Usage.Fallback.HTTPStatus != 429 {
		t.Fatalf("fallback cooldown not persisted: %+v", first.Usage)
	}
	if _, err = s.collectSubscriptionUsage(context.Background(), time.Now(), "fixture", true); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("a second forced refresh ignored provider cooldown: %d", calls.Load())
	}
}
