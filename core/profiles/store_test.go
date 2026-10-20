package profiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/faceair/clash-speedtest/core/speedtester"
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

	gotSecret := RedactURL("https://example.com/api?client_secret=123456")
	if strings.Contains(gotSecret, "123456") || !strings.Contains(gotSecret, "REDACTED") {
		t.Fatalf("secret param should be redacted, got %s", gotSecret)
	}

	gotUserPass := RedactURL("https://alice:supersecret@example.com/sub")
	if strings.Contains(gotUserPass, "supersecret") {
		t.Fatalf("userinfo password should be redacted, got %s", gotUserPass)
	}
	if !strings.Contains(gotUserPass, "alice:REDACTED@example.com") {
		t.Fatalf("expected alice:REDACTED, got %s", gotUserPass)
	}

	gotTokenUser := RedactURL("https://rawtoken123@example.com/sub")
	if strings.Contains(gotTokenUser, "rawtoken123") {
		t.Fatalf("userinfo token username should be redacted, got %s", gotTokenUser)
	}
	if !strings.Contains(gotTokenUser, "REDACTED@example.com") {
		t.Fatalf("expected REDACTED@example.com, got %s", gotTokenUser)
	}
}

func TestRedactURLPathTokens(t *testing.T) {
	for _, raw := range []string{"https://example.test/link/short-secret/clash", "https://example.test/link/encoded%2Fprivate-token", "https://example.test/0123456789abcdefghijklmnop"} {
		got := RedactURL(raw)
		if !strings.Contains(got, "REDACTED") || strings.Contains(got, "secret") || strings.Contains(got, "private-token") || strings.Contains(got, "0123456789") {
			t.Fatalf("path token exposed: %s", got)
		}
	}
	if got := RedactURL("https://example.test/docs/help"); got != "https://example.test/docs/help" {
		t.Fatal(got)
	}
}

func TestCompactDisplayValidUTF8(t *testing.T) {
	s := "a" + strings.Repeat("中", 30)
	got := compactDisplay(s, 72)
	if !utf8.ValidString(got) {
		t.Fatalf("compactDisplay produced invalid UTF-8: %q", got)
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

func TestStoreNilSafety(t *testing.T) {
	var nilStore *Store
	nilStore.Add(&Airport{ID: "test"})
	if nilStore.Get("test") != nil {
		t.Fatal("expected nil from nil store Get")
	}
	if nilStore.Remove("test") != nil {
		t.Fatal("expected nil from nil store Remove")
	}
	if ap, sub := nilStore.FindSubscription("test"); ap != nil || sub != nil {
		t.Fatal("expected nil from nil store FindSubscription")
	}

	store := &Store{
		Airports: []*Airport{
			nil,
			{ID: "ap1", Name: "正常机场"},
			nil,
		},
	}
	if got := store.Get("ap1"); got == nil || got.Name != "正常机场" {
		t.Fatalf("expected to find ap1 despite nil elements, got %+v", got)
	}
	if got := store.Get("nonexistent"); got != nil {
		t.Fatalf("expected nil for nonexistent id, got %+v", got)
	}
	if removed := store.Remove("ap1"); removed == nil || removed.ID != "ap1" {
		t.Fatalf("expected to remove ap1, got %+v", removed)
	}
	if ap, sub := store.FindSubscription("none"); ap != nil || sub != nil {
		t.Fatal("expected nil from FindSubscription with nil elements")
	}
}

func TestFlagFromCodeSanitization(t *testing.T) {
	if got := flagFromCode("12"); got != "🏳️" {
		t.Fatalf("expected 🏳️ for numeric 2-char code, got %q", got)
	}
	if got := flagFromCode("!@"); got != "🏳️" {
		t.Fatalf("expected 🏳️ for symbol 2-char code, got %q", got)
	}
	if got := flagFromCode("HK"); got != "🇭🇰" {
		t.Fatalf("expected 🇭🇰 for HK, got %q", got)
	}
	if got := flagFromCode("US"); got != "🇺🇸" {
		t.Fatalf("expected 🇺🇸 for US, got %q", got)
	}
	if got := flagFromCode(""); got != "🏳️" {
		t.Fatalf("expected 🏳️ for empty code, got %q", got)
	}
}

func TestValidateStoreNilAndEdge(t *testing.T) {
	if err := validateStore(nil); err == nil {
		t.Fatal("expected error from validateStore(nil)")
	}
	if err := validateStore(&Store{}); err != nil {
		t.Fatalf("expected no error from validateStore(&Store{}), got %v", err)
	}
}

func TestSummarizeCachesNilStore(t *testing.T) {
	count, missing := summarizeCaches(sourceSnapshot{})
	if count != 0 || len(missing) != 0 {
		t.Fatalf("expected count=0 missing=0 for nil store, got count=%d missing=%v", count, missing)
	}
}

func TestSnapshotsEqualMissingKeyDefense(t *testing.T) {
	a := sourceSnapshot{
		storeBytes: []byte("test"),
		caches:     map[string][]byte{"keyA": {}},
	}
	b := sourceSnapshot{
		storeBytes: []byte("test"),
		caches:     map[string][]byte{"keyB": {}},
	}
	if snapshotsEqual(a, b) {
		t.Fatal("expected snapshots with different cache keys to be unequal")
	}
}

func TestAirportNilSubscriptionDefense(t *testing.T) {
	ap := &Airport{
		ID:   "ap1",
		Name: "测试",
		Subscriptions: []*Subscription{
			nil,
			{ID: "sub1", URL: "https://example.com/s1"},
		},
	}
	if got := ap.GetSubscription("sub1"); got == nil || got.ID != "sub1" {
		t.Fatalf("expected to find sub1 despite nil elements, got %+v", got)
	}
	if got := ap.GetSubscription("none"); got != nil {
		t.Fatalf("expected nil for nonexistent sub, got %+v", got)
	}
	if rem := ap.RemoveSubscription("sub1"); rem == nil || rem.ID != "sub1" {
		t.Fatalf("expected to remove sub1, got %+v", rem)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "nil_sub.json")
	storeWithNilSub := &Store{
		Airports: []*Airport{
			{
				ID: "ap_nil",
				Subscriptions: []*Subscription{
					nil,
				},
			},
		},
	}
	if err := SaveStore(path, storeWithNilSub); err != nil {
		t.Fatalf("expected SaveStore to succeed with nil sub, got %v", err)
	}
}

func TestAppNilReceiverSafety(t *testing.T) {
	var a *App = nil
	// printf should not panic
	a.printf("hello %s", "world")

	if _, err := a.readLine("prompt"); err == nil {
		t.Fatal("expected error from readLine on nil App")
	}
	if yes, err := a.confirm("prompt", true); !yes || err == nil {
		t.Fatal("expected defaultYes and error from confirm on nil App")
	}
	if _, err := a.SelectAirport(); err == nil {
		t.Fatal("expected error from SelectAirport on nil App")
	}
	// printAirportMenu should not panic
	a.printAirportMenu(nil)
	a.printAirportMenu(&Store{Airports: []*Airport{nil}})

	if _, _, err := a.SelectCountries([]string{"HK"}); err == nil {
		t.Fatal("expected error from SelectCountries on nil App")
	}
	if _, err := a.SelectMetrics(); err == nil {
		t.Fatal("expected error from SelectMetrics on nil App")
	}
	if _, err := a.SelectDuration(); err == nil {
		t.Fatal("expected error from SelectDuration on nil App")
	}
	if _, _, err := a.SelectPlan(speedtester.MetricSet{}); err == nil {
		t.Fatal("expected error from SelectPlan on nil App")
	}
	if _, err := a.SelectTestPlan(); err == nil {
		t.Fatal("expected error from SelectTestPlan on nil App")
	}
}
