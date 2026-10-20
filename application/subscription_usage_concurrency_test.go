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
)

func TestUsageRefreshCancellationFinalizesClaimAndReleasesProfileLock(t *testing.T) {
	var block atomic.Bool
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if block.Load() {
			started <- struct{}{}
			<-r.Context().Done()
			return
		}
		w.Header().Set("Subscription-Userinfo", fmtUsage(100))
		_, _ = w.Write([]byte("proxies: []"))
	}))
	defer server.Close()
	s, _ := newAirportURLTestService(t)
	if _, err := s.CreateAirport("fixture", server.URL, "fixture"); err != nil {
		t.Fatal(err)
	}
	block.Store(true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := s.collectSubscriptionUsage(ctx, time.Now(), "fixture", true); done <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("local provider was not called")
	}
	if s.profileWriteMu.TryLock() {
		s.profileWriteMu.Unlock()
	} else {
		t.Error("network fetch holds profile write lock")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("cancellation error lost: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled collection did not return")
	}
	state, err := s.historyStore.SubscriptionRefreshState(context.Background())
	if err != nil || state.State != "cancelled" {
		t.Fatalf("claimed refresh left unfinished: %+v %v", state, err)
	}
}

func TestUsageRefreshConcurrentProvidersDoNotOverwriteChangedSources(t *testing.T) {
	for _, change := range []string{"source_url", "newer_refresh"} {
		t.Run(change, func(t *testing.T) {
			s, paths := newAirportURLTestService(t)
			var block atomic.Bool
			started := make(chan int, 2)
			release := make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			defer unblock()
			var airports []*AirportDTO
			for i := 0; i < 2; i++ {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					down, body := int64(100), "proxies: []\n# baseline\n"
					if block.Load() {
						started <- i
						select {
						case <-release:
						case <-r.Context().Done():
							return
						}
						down, body = 200, "proxies: []\n# fresh fixture\n"
					}
					w.Header().Set("Subscription-Userinfo", fmtUsage(down))
					_, _ = w.Write([]byte(body))
				}))
				t.Cleanup(server.Close)
				ap, err := s.CreateAirport("fixture", server.URL, "fixture")
				if err != nil {
					t.Fatal(err)
				}
				airports = append(airports, ap)
			}
			block.Store(true)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := s.collectSubscriptionUsage(ctx, time.Now(), "fixture", true); done <- err }()
			for n := 0; n < 2; n++ {
				select {
				case <-started:
				case <-time.After(5 * time.Second):
					t.Error("independent providers fetched serially")
					n = 2
				}
			}
			wantDownload := int64(100)
			if !s.profileWriteMu.TryLock() {
				t.Error("network fetch blocks profile edits")
			} else {
				func() {
					defer s.profileWriteMu.Unlock()
					store, err := profiles.LoadStore(paths.StoreFile())
					if err != nil {
						t.Fatal(err)
					}
					sub := store.Get(airports[0].ID).Subscriptions[0]
					if change == "source_url" {
						sub.URL += "/changed"
					} else {
						sub.UpdatedAt = time.Now().Add(time.Second)
						sub.Usage.Download = 333
						wantDownload = 333
					}
					if err := profiles.SaveStore(paths.StoreFile(), store); err != nil {
						t.Fatal(err)
					}
				}()
			}
			unblock()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("collection did not finish")
			}
			store, err := profiles.LoadStore(paths.StoreFile())
			if err != nil {
				t.Fatal(err)
			}
			first, second := store.Get(airports[0].ID).Subscriptions[0], store.Get(airports[1].ID).Subscriptions[0]
			if first.Usage.Download != wantDownload || second.Usage.Download != 200 {
				t.Fatalf("stale/current counters: first=%d second=%d", first.Usage.Download, second.Usage.Download)
			}
			body, err := os.ReadFile(paths.CacheFile(first.ID))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(body), "fresh fixture") {
				t.Fatal("stale fetch overwrote changed source cache")
			}
			state, err := s.historyStore.SubscriptionRefreshState(context.Background())
			if err != nil || state.State != "completed_with_issues" {
				t.Fatalf("wrong collection state: %+v %v", state, err)
			}
		})
	}
}

func TestUsageRefreshSerializesOriginAndHonors429Cooldown(t *testing.T) {
	var limited atomic.Bool
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if limited.Load() {
			calls.Add(1)
			w.Header().Set("Retry-After", "600")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Subscription-Userinfo", fmtUsage(100))
		_, _ = w.Write([]byte("proxies: []"))
	}))
	defer server.Close()
	s, _ := newAirportURLTestService(t)
	ap, err := s.CreateAirport("fixture", server.URL, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddSubscription(ap.ID, "same origin", server.URL+"/other", "", "fixture"); err != nil {
		t.Fatal(err)
	}
	limited.Store(true)
	for i := 0; i < 2; i++ {
		state, err := s.collectSubscriptionUsage(context.Background(), time.Now(), "fixture", true)
		if err != nil || state.State != "completed_with_issues" {
			t.Fatalf("cooldown collection: %+v %v", state, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("same-origin cooldown issued %d requests, want only the first 429", calls.Load())
	}
}
