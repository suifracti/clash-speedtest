package profiles

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestImportPreservesProfileIDsAndCacheBytes(t *testing.T) {
	source := t.TempDir()
	targetRoot := t.TempDir()
	target := Paths{Dir: filepath.Join(targetRoot, "profiles")}
	storeBytes := []byte(`{
  "airports": [
    {"id":"profile-fixture","name":"Fixture","url":"https://fixture.invalid/sub"}
  ]
}
`)
	cacheBytes := []byte("proxies:\n  - name: fixture-node\n    type: ss\n    server: 192.0.2.10\n    port: 8388\n    cipher: aes-128-gcm\n    password: fixture-password\n")
	writeFixture(t, source, storeBytes, map[string][]byte{"profile-fixture.yaml": cacheBytes})

	info, err := InspectSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Available || info.ProfileCount != 1 || info.CacheCount != 1 {
		t.Fatalf("unexpected fixture summary: %+v", info)
	}
	if err := target.ImportFrom(context.Background(), source); err != nil {
		t.Fatal(err)
	}

	gotStore, err := os.ReadFile(target.StoreFile())
	if err != nil {
		t.Fatal(err)
	}
	gotCache, err := os.ReadFile(target.CacheFile("profile-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotStore, storeBytes) || !bytes.Equal(gotCache, cacheBytes) {
		t.Fatalf("import changed source bytes: store=%v cache=%v", bytes.Equal(gotStore, storeBytes), bytes.Equal(gotCache, cacheBytes))
	}
	loaded, err := LoadStore(target.StoreFile())
	if err != nil || len(loaded.Airports) != 1 || loaded.Airports[0].ID != "profile-fixture" {
		t.Fatalf("profile identity changed after import: store=%+v err=%v", loaded, err)
	}
	if !target.HasCache("profile-fixture") {
		t.Fatal("imported cache is not available")
	}
}

func TestImportMissingCacheIsExplicitButDoesNotInventNodes(t *testing.T) {
	source := t.TempDir()
	target := Paths{Dir: filepath.Join(t.TempDir(), "profiles")}
	storeBytes := []byte(`{"airports":[{"id":"profile-no-cache","name":"No cache","url":"https://fixture.invalid/no-cache"}]}`)
	writeFixture(t, source, storeBytes, nil)

	info, err := InspectSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Available || info.CacheCount != 0 || len(info.Missing) == 0 {
		t.Fatalf("missing cache was not surfaced: %+v", info)
	}
	if err := target.ImportFrom(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	if target.HasCache("profile-no-cache") {
		t.Fatal("missing cache was invented during import")
	}
}

func TestImportInterruptionLeavesOnlyInspectableStaging(t *testing.T) {
	source := t.TempDir()
	target := Paths{Dir: filepath.Join(t.TempDir(), "profiles")}
	writeFixture(t, source, []byte(`{"airports":[]}`), nil)

	ctx, cancel := context.WithCancel(context.Background())
	err := target.importWithOptions(ctx, source, cancel)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected pre-commit cancellation, got %v", err)
	}
	if _, err := os.Stat(target.Dir); !os.IsNotExist(err) {
		t.Fatalf("canonical profile was published after interruption: %v", err)
	}
	status := target.InspectSetup()
	if len(status.UnfinishedStaging) != 1 {
		t.Fatalf("interrupted staging was not surfaced: %+v", status)
	}
	if err := target.DiscardUnfinishedImport(); err != nil {
		t.Fatal(err)
	}
	if len(target.UnfinishedStaging()) != 0 || target.importLockPresent() {
		t.Fatal("explicit staging cleanup did not remove only unfinished import state")
	}
}

func TestImportTargetConflictAndLockAreExplicit(t *testing.T) {
	source := t.TempDir()
	targetRoot := t.TempDir()
	target := Paths{Dir: filepath.Join(targetRoot, "profiles")}
	writeFixture(t, source, []byte(`{"airports":[]}`), nil)
	if err := target.ImportFrom(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	if err := target.ImportFrom(context.Background(), source); !errors.Is(err, ErrProfileTargetConflict) {
		t.Fatalf("expected target conflict, got %v", err)
	}

	lockedTarget := Paths{Dir: filepath.Join(t.TempDir(), "profiles")}
	if err := os.MkdirAll(filepath.Dir(lockedTarget.Dir), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(lockedTarget.Dir), importLockName), []byte("held\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := lockedTarget.ImportFrom(context.Background(), source); !errors.Is(err, ErrProfileImportLocked) {
		t.Fatalf("expected import lock conflict, got %v", err)
	}
}

func TestInitializeEmptyIsDistinctFromUndecidedState(t *testing.T) {
	target := Paths{Dir: filepath.Join(t.TempDir(), "profiles")}
	if status := target.InspectSetup(); status.Initialized || status.TargetExists {
		t.Fatalf("fresh target should be undecided: %+v", status)
	}
	if err := target.InitializeEmpty(); err != nil {
		t.Fatal(err)
	}
	status := target.InspectSetup()
	if !status.Initialized || status.Error != "" {
		t.Fatalf("empty choice did not initialize canonical store: %+v", status)
	}
	if err := target.InitializeEmpty(); !errors.Is(err, ErrProfileTargetConflict) {
		t.Fatalf("second initialization should not overwrite target, got %v", err)
	}
}

func TestDamagedCanonicalProfileIsNotReportedAsEmpty(t *testing.T) {
	target := Paths{Dir: filepath.Join(t.TempDir(), "profiles")}
	if err := os.MkdirAll(target.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target.StoreFile(), []byte(`{"airports":[{"id":"..\\escape","name":"bad"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	status := target.InspectSetup()
	if status.Error == "" || status.Initialized {
		t.Fatalf("unsafe canonical profile was not rejected: %+v", status)
	}
}

func writeFixture(t *testing.T, dir string, store []byte, caches map[string][]byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, CacheDirName), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, StoreFileName), store, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, body := range caches {
		if err := os.WriteFile(filepath.Join(dir, CacheDirName, name), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
