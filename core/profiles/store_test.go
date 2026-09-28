package profiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreSaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, StoreFileName)
	store := &Store{}
	store.Add(&Airport{ID: "abc", Name: "测试机场", URL: "https://example.com/sub?token=secret"})
	if err := SaveStore(path, store); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Airports) != 1 || loaded.Airports[0].Name != "测试机场" {
		t.Fatalf("unexpected store: %+v", loaded)
	}
	removed := loaded.Remove("abc")
	if removed == nil || len(loaded.Airports) != 0 {
		t.Fatalf("remove failed: %+v", loaded)
	}
}

func TestApplyAirportEdit(t *testing.T) {
	airport := &Airport{
		ID:        "abc",
		Name:      "旧名",
		URL:       "https://old.example/sub",
		UpdatedAt: time.Now(),
	}
	if !applyAirportEdit(airport, "新名", "https://new.example/sub") {
		t.Fatal("expected url change")
	}
	if airport.Name != "新名" || airport.URL != "https://new.example/sub" {
		t.Fatalf("unexpected airport: %+v", airport)
	}
	if !airport.UpdatedAt.IsZero() {
		t.Fatal("updated_at should reset when url changes")
	}
	if applyAirportEdit(airport, "", "https://new.example/sub") {
		t.Fatal("same url should not count as change")
	}
	if airport.Name != "新名" {
		t.Fatal("empty name should keep old name")
	}
}

func TestParseLegacyEnv(t *testing.T) {
	data := []byte("CLASH_SPEEDTEST_ARGS=-fast\nCLASH_SPEEDTEST_CONFIG=https://example.com/sub?token=abc\n")
	got := parseLegacyEnv(data)
	if got != "https://example.com/sub?token=abc" {
		t.Fatalf("got %q", got)
	}
}

func TestImportLegacyIfEmpty(t *testing.T) {
	dir := t.TempDir()
	p := Paths{Dir: dir}
	if err := os.WriteFile(p.LegacyEnvFile(), []byte("CLASH_SPEEDTEST_CONFIG=https://x.test/s\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := &Store{}
	if !p.ImportLegacyIfEmpty(store) {
		t.Fatal("expected import")
	}
	if len(store.Airports) != 1 || store.Airports[0].URL != "https://x.test/s" {
		t.Fatalf("unexpected import: %+v", store.Airports)
	}
}

func TestRedactURL(t *testing.T) {
	got := RedactURL("https://example.com/api?token=secret&flag=meta")
	if strings.Contains(got, "secret") {
		t.Fatalf("token should be redacted, got %s", got)
	}
	if !strings.Contains(got, "token=") || !strings.Contains(got, "REDACTED") {
		t.Fatalf("redacted url = %s", got)
	}
}

func TestMultiSubscriptionStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, StoreFileName)

	ap := &Airport{
		ID:         "ap1",
		Name:       "多订阅机场",
		WebsiteURL: "https://example.com",
		BackupURL:  "https://backup.example.com",
		Subscriptions: []*Subscription{
			{ID: "sub1", Name: "主号", URL: "https://example.com/sub1"},
			{ID: "sub2", Name: "备用号", URL: "https://example.com/sub2"},
		},
	}
	store := &Store{}
	store.Add(ap)
	if err := SaveStore(path, store); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Airports) != 1 {
		t.Fatalf("expected 1 airport, got %d", len(loaded.Airports))
	}
	loadedAp := loaded.Airports[0]
	if len(loadedAp.Subscriptions) != 2 {
		t.Fatalf("expected 2 subscriptions, got %d", len(loadedAp.Subscriptions))
	}
	if loadedAp.Subscriptions[0].Name != "主号" || loadedAp.Subscriptions[1].Name != "备用号" {
		t.Fatalf("unexpected subscriptions: %+v", loadedAp.Subscriptions)
	}

	foundAp, foundSub := loaded.FindSubscription("sub2")
	if foundAp == nil || foundSub == nil || foundSub.Name != "备用号" {
		t.Fatalf("failed to find subscription: ap=%+v, sub=%+v", foundAp, foundSub)
	}

	// Test migration of legacy airport with URL only
	legacyPath := filepath.Join(dir, "legacy.json")
	legacyData := []byte(`{"airports":[{"id":"legacy1","name":"旧机场","url":"https://legacy.example/sub"}]}`)
	if err := os.WriteFile(legacyPath, legacyData, 0o600); err != nil {
		t.Fatal(err)
	}
	legacyLoaded, err := LoadStore(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(legacyLoaded.Airports[0].Subscriptions) != 1 {
		t.Fatalf("expected auto-migrated subscription, got: %+v", legacyLoaded.Airports[0].Subscriptions)
	}
	if legacyLoaded.Airports[0].Subscriptions[0].ID != "legacy1" {
		t.Fatalf("expected sub id to match legacy airport id, got %s", legacyLoaded.Airports[0].Subscriptions[0].ID)
	}
}

