package application

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/profiles"
)

func vergeSourceFixture(t *testing.T) (string, func(string, string)) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("APPDATA", root)
	file, err := profiles.DefaultVergeProfilesFile()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(filepath.Dir(file), "profiles"), 0700); err != nil {
		t.Fatal(err)
	}
	write := func(url, body string) {
		t.Helper()
		index := "items:\n  - uid: remote-fixture\n    type: remote\n    name: fixture\n    file: fixture.yaml\n    url: " + url + "\n    updated: 1793966400\n    extra: {upload: 1, download: 2, total: 100}\n"
		if err := os.WriteFile(file, []byte(index), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(filepath.Dir(file), "profiles", "fixture.yaml"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return file, write
}

func TestVergeSyncKeepsAccountWhenSourceURLChanges(t *testing.T) {
	_, write := vergeSourceFixture(t)
	s, paths := newAirportURLTestService(t)
	sub := &profiles.Subscription{ID: "sub-a", Name: "original alias", URL: "https://sub.example.invalid/old", LastFailureCode: "network"}
	store := &profiles.Store{Airports: []*profiles.Airport{{ID: "airport-a", Name: "机场A", Subscriptions: []*profiles.Subscription{sub}}}}
	if err := profiles.SaveStore(paths.StoreFile(), store); err != nil {
		t.Fatal(err)
	}
	if err := paths.WriteCache(sub.ID, []byte(rollbackFixtureBody)); err != nil {
		t.Fatal(err)
	}
	write("https://sub.example.invalid/new?token=FIXTURE_SECRET", strings.ReplaceAll(rollbackFixtureBody, "replacement-node", "new-node"))

	preview, err := s.ListClashVergeSubscriptions()
	if err != nil || len(preview) != 1 || preview[0].SubscriptionID != "" {
		t.Fatalf("unrelated URLs must require an explicit account choice: %+v %v", preview, err)
	}
	if _, err = s.ImportClashVergeSubscription(context.Background(), ClashVergeImportRequest{UID: "remote-fixture", AirportID: "airport-a", SubscriptionID: "sub-a"}); err != nil {
		t.Fatal(err)
	}
	store, err = profiles.LoadStore(paths.StoreFile())
	if err != nil {
		t.Fatal(err)
	}
	updated := store.Get("airport-a").GetSubscription("sub-a")
	if updated.VergeUID != "remote-fixture" || updated.Name != "original alias" || updated.LastFailureCode != "" || updated.URL != "https://sub.example.invalid/new?token=FIXTURE_SECRET" {
		t.Fatalf("existing account not updated in place: %+v", updated)
	}
	if updated.NodeVersions == nil || len(updated.NodeVersions.Previous) != 1 || updated.NodeVersions.Current[0].Name != "new-node" {
		t.Fatalf("missing node change: %+v", updated.NodeVersions)
	}
	snapshots, err := s.historyStore.QuerySubscriptionUsage(context.Background(), time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, snapshot := range snapshots {
		if snapshot.Source == "subscription_update" && snapshot.FetchPath == "verge" && snapshot.Status == "ok" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing local source usage snapshot")
	}

	write("https://sub.example.invalid/rotated?token=FIXTURE_SECRET", strings.ReplaceAll(rollbackFixtureBody, "replacement-node", "rotated-node"))
	preview, err = s.ListClashVergeSubscriptions()
	if err != nil || !preview[0].Linked || !preview[0].URLChanged || preview[0].SubscriptionID != "sub-a" {
		t.Fatalf("UID association was lost: %+v %v", preview, err)
	}
	encoded, _ := json.Marshal(preview)
	if strings.Contains(string(encoded), "FIXTURE_SECRET") || strings.Contains(string(encoded), "sub.example.invalid") {
		t.Fatal("preview leaked a source URL")
	}
	if _, err = s.ImportClashVergeSubscription(context.Background(), ClashVergeImportRequest{UID: "remote-fixture"}); err != nil {
		t.Fatal(err)
	}
	store, _ = profiles.LoadStore(paths.StoreFile())
	if len(store.Airports) != 1 || len(store.Get("airport-a").Subscriptions) != 1 || store.Get("airport-a").GetSubscription("sub-a").NodeVersions.Current[0].Name != "rotated-node" {
		t.Fatal("rotated URL must update the same account and cache")
	}
}

func TestVergeSyncRejectsInvalidCacheWithoutChangingAccount(t *testing.T) {
	_, write := vergeSourceFixture(t)
	s, paths := newAirportURLTestService(t)
	store := &profiles.Store{Airports: []*profiles.Airport{{ID: "airport-a", Name: "机场A", Subscriptions: []*profiles.Subscription{{ID: "sub-a", Name: "fixture", URL: "https://sub.example.invalid/old"}}}}}
	if err := profiles.SaveStore(paths.StoreFile(), store); err != nil {
		t.Fatal(err)
	}
	if err := paths.WriteCache("sub-a", []byte(rollbackFixtureBody)); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(paths.StoreFile())
	write("https://sub.example.invalid/new", "proxies: []\n")
	if _, err := s.ImportClashVergeSubscription(context.Background(), ClashVergeImportRequest{UID: "remote-fixture", AirportID: "airport-a", SubscriptionID: "sub-a"}); err == nil {
		t.Fatal("invalid cache must be rejected")
	}
	after, _ := os.ReadFile(paths.StoreFile())
	cache, _ := os.ReadFile(paths.CacheFile("sub-a"))
	if !bytes.Equal(before, after) || string(cache) != rollbackFixtureBody {
		t.Fatal("failed import changed the existing source or cache")
	}
}

func TestVergeImportCreatesOneAirportAndReusesIt(t *testing.T) {
	_, write := vergeSourceFixture(t)
	s, paths := newAirportURLTestService(t)
	write("https://sub.example.invalid/new", rollbackFixtureBody)
	first, err := s.ImportClashVergeSubscription(context.Background(), ClashVergeImportRequest{UID: "remote-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.ImportClashVergeSubscription(context.Background(), ClashVergeImportRequest{UID: "remote-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	store, err := profiles.LoadStore(paths.StoreFile())
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Airports) != 1 || first.AirportID != second.AirportID || first.ID != second.ID || second.NodeCount != 1 {
		t.Fatal("reimport must update the created account rather than duplicate it")
	}
}
