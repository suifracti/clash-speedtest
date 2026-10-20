package application

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/profiles"
	"github.com/faceair/clash-speedtest/core/subscriptionusage"
)

func TestAccountUsageCancellationAndStaleResponses(t *testing.T) {
	for _, change := range []string{"cancel", "user_agent", "newer_usage"} {
		t.Run(change, func(t *testing.T) {
			var block atomic.Bool
			started := make(chan struct{}, 1)
			release := make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if block.Load() {
					started <- struct{}{}
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
				}
				w.Header().Set("Subscription-Userinfo", "upload=0;download=100;total=1000")
				_, _ = w.Write([]byte("proxies: []"))
			}))
			defer func() { unblock(); provider.Close() }()
			s, paths := newAirportURLTestService(t)
			airport, err := s.CreateAirport("Fixture", provider.URL, "fixture")
			if err != nil {
				t.Fatal(err)
			}
			subID := airport.Subscriptions[0].ID
			original, _ := os.ReadFile(paths.CacheFile(subID))
			block.Store(true)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := s.RefreshAccountUsage(ctx, airport.ID, subID, "fixture"); done <- err }()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("request did not start")
			}
			if !s.profileWriteMu.TryLock() {
				t.Error("network holds profile write lock")
				cancel()
				unblock()
			} else {
				func() {
					defer s.profileWriteMu.Unlock()
					store, err := profiles.LoadStore(paths.StoreFile())
					if err != nil {
						t.Fatal(err)
					}
					sub := store.Get(airport.ID).GetSubscription(subID)
					if change == "user_agent" {
						sub.UserAgent = "newer-UA"
					} else if change == "newer_usage" {
						value := int64(333)
						sub.Usage.Download = &value
						sub.Usage.UpdatedAt = time.Now().Add(time.Second)
					}
					if change != "cancel" {
						if err = profiles.SaveStore(paths.StoreFile(), store); err != nil {
							t.Fatal(err)
						}
					}
				}()
			}
			if change == "cancel" {
				cancel()
			}
			unblock()
			select {
			case err = <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("request did not finish")
			}
			if change == "cancel" {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation lost: %v", err)
				}
			} else {
				var own *refreshError
				if !errors.As(err, &own) || own.Code != "configuration_changed" {
					t.Fatalf("stale response accepted: %v", err)
				}
			}
			body, _ := os.ReadFile(paths.CacheFile(subID))
			if string(body) != string(original) {
				t.Fatal("stale/cancelled usage changed cache")
			}
			saved, _ := profiles.LoadStore(paths.StoreFile())
			if change == "newer_usage" && *saved.Get(airport.ID).GetSubscription(subID).Usage.Download != 333 {
				t.Fatal("older fetch overwrote newer counters")
			}
			if !s.usageRefreshMu.TryLock() {
				t.Fatal("usage update remained locked after completion")
			}
			s.usageRefreshMu.Unlock()
		})
	}
}

func TestAccountUsageHistoryFailureRollsBackProfile(t *testing.T) {
	var down atomic.Int64
	down.Store(100)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Subscription-Userinfo", fmtUsage(down.Load()))
		_, _ = w.Write([]byte("proxies: []"))
	}))
	defer provider.Close()
	s, paths := newAirportURLTestService(t)
	airport, err := s.CreateAirport("Fixture", provider.URL, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	subID := airport.Subscriptions[0].ID
	original, _ := os.ReadFile(paths.StoreFile())
	cache, _ := os.ReadFile(paths.CacheFile(subID))
	down.Store(500)
	s.subscriptionRefreshUsageSaveHook = func(context.Context, []subscriptionusage.Snapshot) error {
		return errors.New("fixture history failure")
	}
	if _, err = s.RefreshAccountUsage(context.Background(), airport.ID, subID, "fixture"); err == nil {
		t.Fatal("history failure was hidden")
	}
	after, _ := os.ReadFile(paths.StoreFile())
	afterCache, _ := os.ReadFile(paths.CacheFile(subID))
	if string(after) != string(original) || string(afterCache) != string(cache) {
		t.Fatal("history failure left new profile state or touched nodes")
	}
}

func TestAccountUsageRateLimitAndNoticeBalance(t *testing.T) {
	for _, test := range []string{"rate_limit", "notice"} {
		t.Run(test, func(t *testing.T) {
			var mode atomic.Bool
			var calls atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if !mode.Load() {
					w.Header().Set("Subscription-Userinfo", "upload=0;download=10;total=100")
					_, _ = w.Write([]byte("proxies: []"))
					return
				}
				if test == "rate_limit" {
					w.Header().Set("Retry-After", "120")
					w.WriteHeader(http.StatusTooManyRequests)
					return
				}
				_, _ = w.Write([]byte("proxies:\n - {name: '剩余流量：11 GiB', type: ss, server: 192.0.2.1, port: 443, cipher: aes-128-gcm, password: fixture}"))
			}))
			defer provider.Close()
			s, paths := newAirportURLTestService(t)
			airport, err := s.CreateAirport("Fixture", provider.URL, "fixture")
			if err != nil {
				t.Fatal(err)
			}
			subID := airport.Subscriptions[0].ID
			cache, _ := os.ReadFile(paths.CacheFile(subID))
			mode.Store(true)
			calls.Store(0)
			result, err := s.RefreshAccountUsage(context.Background(), airport.ID, subID, "fixture")
			if test == "rate_limit" {
				if err == nil {
					t.Fatal("rate limit hidden")
				}
				if _, err = s.RefreshAccountUsage(context.Background(), airport.ID, subID, "fixture"); err == nil {
					t.Fatal("cooldown ignored")
				}
				saved, _ := profiles.LoadStore(paths.StoreFile())
				sub := saved.Get(airport.ID).GetSubscription(subID)
				if calls.Load() != 1 || *sub.Usage.Download != 10 || !sub.LastFailureRetryAt.After(time.Now()) {
					t.Fatalf("rate limit lost old counters/cooldown: calls=%d usage=%+v", calls.Load(), sub.Usage)
				}
			} else {
				if err != nil || result.Observation.CounterKind != "notice_remaining" || result.Observation.Remaining == nil || *result.Observation.Remaining != 11*(1<<30) || result.Observation.Upload != nil {
					t.Fatalf("notice balance: %+v %v", result, err)
				}
			}
			after, _ := os.ReadFile(paths.CacheFile(subID))
			if string(after) != string(cache) || strings.Contains(string(after), "11 GiB") {
				t.Fatal("usage-only operation replaced cache")
			}
		})
	}
}
