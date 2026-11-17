package application

import (
	"context"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/profiles"
)

func TestSourceDeletionKeepsCacheWhenConfigurationCannotBeSaved(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("read-only destination replacement semantics are Windows-specific")
	}
	for _, wholeAirport := range []bool{false, true} {
		name := "account"
		if wholeAirport {
			name = "airport"
		}
		t.Run(name, func(t *testing.T) {
			s, paths := newAirportURLTestService(t)
			ap := &profiles.Airport{ID: "airport-delete", Name: "Fixture", Subscriptions: []*profiles.Subscription{{ID: "account-delete", Name: "Fixture account", URL: "https://example.invalid/source"}}}
			if err := profiles.SaveStore(paths.StoreFile(), &profiles.Store{Airports: []*profiles.Airport{ap}}); err != nil {
				t.Fatal(err)
			}
			cache := []byte("proxies: []\n")
			if err := paths.WriteCache("account-delete", cache); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(paths.StoreFile())
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(paths.StoreFile(), 0444); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(paths.StoreFile(), 0600) })
			if wholeAirport {
				err = s.DeleteAirport(ap.ID)
			} else {
				err = s.DeleteSubscription(ap.ID, "account-delete")
			}
			if err == nil {
				t.Fatal("expected read-only configuration replacement to fail")
			}
			after, err := os.ReadFile(paths.StoreFile())
			if err != nil || string(after) != string(before) {
				t.Fatalf("failed deletion changed configuration: %v", err)
			}
			got, err := os.ReadFile(paths.CacheFile("account-delete"))
			if err != nil || string(got) != string(cache) {
				t.Fatalf("failed deletion removed account cache: %v", err)
			}
		})
	}
}

func TestSourceDeletionRetainsSavedHistoryAndOtherAccounts(t *testing.T) {
	s, paths := newAirportURLTestService(t)
	ap := &profiles.Airport{ID: "airport-delete", Name: "Fixture", Subscriptions: []*profiles.Subscription{{ID: "account-delete", Name: "One", URL: "https://example.invalid/one"}, {ID: "account-keep", Name: "Two", URL: "https://example.invalid/two"}}}
	if err := profiles.SaveStore(paths.StoreFile(), &profiles.Store{Airports: []*profiles.Airport{ap}}); err != nil {
		t.Fatal(err)
	}
	for _, sub := range ap.Subscriptions {
		if err := paths.WriteCache(sub.ID, []byte("proxies: []\n")); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	record := &history.LatencyTest{AttemptID: "delete-history", ProfileID: "account-delete", NodeKey: "node", NodeIdentityKey: "identity", ConfigRevisionKey: "revision", DisplayName: "Fixture", NodeType: "ss", TestProject: "latency_stability", RequestedAt: now, StartedAt: now, FinishedAt: now, Status: "completed"}
	if err := s.historyStore.SaveLatencyTest(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSubscription(ap.ID, "account-delete"); err != nil {
		t.Fatal(err)
	}
	store, err := profiles.LoadStore(paths.StoreFile())
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Get(ap.ID).Subscriptions) != 1 || store.Get(ap.ID).Subscriptions[0].ID != "account-keep" {
		t.Fatal("deletion changed other accounts")
	}
	if paths.HasCache("account-delete") || !paths.HasCache("account-keep") {
		t.Fatal("deletion removed the wrong cache")
	}
	if err := s.DeleteAirport(ap.ID); err != nil {
		t.Fatal(err)
	}
	if paths.HasCache("account-keep") {
		t.Fatal("airport deletion left its account cache")
	}
	got, err := s.historyStore.GetLatencyTest(context.Background(), record.AttemptID)
	if err != nil || got == nil || got.ProfileID != record.ProfileID {
		t.Fatalf("source deletion changed saved history: %v", err)
	}
}
