package history

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Real OS unlink denial reproduces Sol D's post-publication cleanup failure.
// The sync error is synthetic; the permission denial is not mocked.
func TestV11UpgradeBackupCleanupPermissionFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Fatal("permission regression requires a non-root test runner")
	}
	dir, live, hashes := upgradeWALFixture(t)
	// A prior valid snapshot must survive both failure and the fresh retry.
	if err := createVerifiedV11Rollback(context.Background(), dir, upgradeBackupHooks{}); err != nil {
		t.Fatal(err)
	}
	prior := completedUpgradeSnapshots(t, dir)
	priorBytes, err := os.ReadFile(prior[0])
	if err != nil {
		t.Fatal(err)
	}
	priorHash := sha256.Sum256(priorBytes)
	var attempt string
	root := filepath.Join(dir, ".schema-upgrade-backups")
	t.Cleanup(func() {
		// Permissions are restored only on this test's synthetic directories.
		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err == nil && info.IsDir() {
				return os.Chmod(path, 0700)
			}
			return nil
		})
	})
	hooks := upgradeBackupHooks{sync: func(path string) error {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if info.IsDir() && strings.HasPrefix(filepath.Base(path), "v11-to-v12-") {
			attempt = path
			if err := os.Chmod(path, 0500); err != nil {
				return err
			}
			return errors.New("synthetic final attempt sync failure")
		}
		return syncUpgradeBackupPath(path)
	}}
	err = migrateSchema(live, func() error { return createVerifiedV11Rollback(context.Background(), dir, hooks) })
	if !errors.Is(err, ErrUpgradeBackup) {
		t.Fatalf("gate error=%v", err)
	}
	assertUpgradeVersion(t, live, 11)
	assertUpgradeSourceHashes(t, hashes)
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("cleanup permission failure was swallowed: %v", err)
	}
	if _, statErr := os.Stat(attempt); !os.IsNotExist(statErr) {
		t.Fatalf("failed attempt remains in completion namespace: stat=%v gate=%v", statErr, err)
	}
	quarantines, err := filepath.Glob(filepath.Join(root, ".failed-*"))
	if err != nil || len(quarantines) != 1 {
		t.Fatalf("failed quarantine=%v err=%v", quarantines, err)
	}
	failed := quarantines[0]
	if _, err := os.Stat(filepath.Join(failed, "complete.json")); err != nil {
		t.Fatalf("failed artifacts not observable: %v", err)
	}
	if got := len(completedUpgradeSnapshots(t, dir)); got != 1 {
		t.Fatalf("failed attempt counted as complete: %d", got)
	}
	// Restore permission only on the isolated fixture. Retry must not reuse it.
	if err := os.Chmod(failed, 0700); err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	assertUpgradeVersion(t, live, 12)
	if got := len(completedUpgradeSnapshots(t, dir)); got != 2 {
		t.Fatalf("fresh retry backups=%d want2", got)
	}
	b, err := os.ReadFile(prior[0])
	if err != nil || sha256.Sum256(b) != priorHash {
		t.Fatal("prior backup changed")
	}
	if _, err := os.Stat(filepath.Join(failed, "complete.json")); err != nil {
		t.Fatal("retry deleted failed quarantine")
	}
	t.Log("post-publication failure: source v11 and DB/WAL hashes unchanged; real cleanup denial surfaced; failed attempt quarantined; fresh retry preserves prior and failed artifacts")
}

func TestV11UpgradeBackupCleanupAndQuarantineDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Fatal("permission regression requires a non-root test runner")
	}
	dir, live, hashes := upgradeWALFixture(t)
	root := filepath.Join(dir, ".schema-upgrade-backups")
	var attempt string
	t.Cleanup(func() {
		os.Chmod(root, 0700)
		if attempt != "" {
			os.Chmod(attempt, 0700)
		}
	})
	hooks := upgradeBackupHooks{sync: func(path string) error {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if info.IsDir() && strings.HasPrefix(filepath.Base(path), "v11-to-v12-") {
			attempt = path
			if err := os.Chmod(path, 0500); err != nil {
				return err
			}
			if err := os.Chmod(root, 0500); err != nil {
				return err
			}
			return errors.New("synthetic directory sync failure with parent permission denial")
		}
		return syncUpgradeBackupPath(path)
	}}
	err := migrateSchema(live, func() error { return createVerifiedV11Rollback(context.Background(), dir, hooks) })
	if !errors.Is(err, ErrUpgradeBackup) || !errors.Is(err, os.ErrPermission) {
		t.Fatalf("failure did not retain gate/permission causes: %v", err)
	}
	if !strings.Contains(err.Error(), attempt) || !strings.Contains(err.Error(), "quarantine incomplete backup") {
		t.Fatalf("residual path/quarantine failure not observable: %v", err)
	}
	assertUpgradeVersion(t, live, 11)
	assertUpgradeSourceHashes(t, hashes)
	for _, path := range []string{root, attempt} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0500 {
			t.Fatalf("production changed fixture permissions at %s: %v %v", path, info, err)
		}
	}
	// A readable manifest can remain when both removal and rename are denied.
	// It is not proof of gate success; OpenDB must create a different attempt.
	manifest := filepath.Join(attempt, "complete.json")
	before, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(attempt, 0700); err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	assertUpgradeVersion(t, live, 12)
	paths, err := filepath.Glob(filepath.Join(root, "v11-to-v12-*", "complete.json"))
	if err != nil || len(paths) != 2 {
		t.Fatalf("retry did not create a distinct attempt: %v %v", paths, err)
	}
	after, err := os.ReadFile(manifest)
	if err != nil || sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("retry modified failed manifest")
	}
	t.Log("cleanup and quarantine both denied: joined errors expose residual path; source DB/WAL unchanged; permissions untouched; retry creates new backup without trusting residual complete.json")
}

func TestV11UpgradeBackupFailuresBeforeDDL(t *testing.T) {
	for _, mode := range []string{"copy", "corrupt", "wrong-version", "v12-column", "foreign-key", "snapshot-sync", "manifest-write", "manifest-sync", "manifest-publish", "directory-sync", "cancel-before", "cancel-after-copy"} {
		t.Run(mode, func(t *testing.T) {
			dir, live, hashes := upgradeWALFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			hooks := upgradeBackupHooks{}
			if mode == "cancel-before" {
				cancel()
			}
			hooks.backup = func(ctx context.Context, source, target string) error {
				if mode == "copy" {
					return errors.New("injected copy failure")
				}
				if err := BackupDatabase(ctx, source, target); err != nil {
					return err
				}
				if mode == "cancel-after-copy" {
					cancel()
					return nil
				}
				if mode == "corrupt" {
					return os.WriteFile(target, []byte("invalid synthetic SQLite"), 0600)
				}
				statement := map[string]string{
					"wrong-version": "UPDATE schema_meta SET schema_version=12",
					"v12-column":    "ALTER TABLE workbench_latency_tests ADD COLUMN network_path_json TEXT NOT NULL DEFAULT ''",
					"foreign-key":   "UPDATE workbench_latency_samples SET attempt_id='missing-parent' WHERE seq=3",
				}[mode]
				if statement != "" {
					raw, err := sql.Open("sqlite", target)
					if err != nil {
						return err
					}
					_, err = raw.Exec(statement)
					return errors.Join(err, raw.Close())
				}
				return nil
			}
			hooks.sync = func(path string) error {
				if mode == "manifest-write" && filepath.Base(path) == "history.db" {
					if err := os.Mkdir(filepath.Join(filepath.Dir(path), ".complete.tmp"), 0700); err != nil {
						return err
					}
				}
				if mode == "manifest-publish" && filepath.Base(path) == ".complete.tmp" {
					if err := os.WriteFile(filepath.Join(filepath.Dir(path), "complete.json"), []byte("keep"), 0600); err != nil {
						return err
					}
				}
				info, err := os.Stat(path)
				if err != nil {
					return err
				}
				if (mode == "snapshot-sync" && filepath.Base(path) == "history.db") ||
					(mode == "manifest-sync" && filepath.Base(path) == ".complete.tmp") ||
					(mode == "directory-sync" && info.IsDir()) {
					return errors.New("injected sync failure")
				}
				return syncUpgradeBackupPath(path)
			}
			err := migrateSchema(live, func() error { return createVerifiedV11Rollback(ctx, dir, hooks) })
			if !errors.Is(err, ErrUpgradeBackup) {
				t.Fatalf("gate failure=%v", err)
			}
			assertUpgradeVersion(t, live, 11)
			assertUpgradeSourceHashes(t, hashes)
			if len(completedUpgradeSnapshots(t, dir)) != 0 {
				t.Fatal("failed attempt left completed backup")
			}
			entries, err := os.ReadDir(filepath.Join(dir, ".schema-upgrade-backups"))
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatal("failed attempt left private files")
			}
			// Retry uses the production gate, not any leftover failed attempt.
			db, err := OpenDB(dir)
			if err != nil {
				t.Fatal(err)
			}
			db.Close()
			assertUpgradeVersion(t, live, 12)
			if len(completedUpgradeSnapshots(t, dir)) != 1 {
				t.Fatal("retry did not create fresh complete backup")
			}
		})
	}
}

func TestV11UpgradeBackupWriterCommitBlocksStaleMigration(t *testing.T) {
	for _, phase := range []string{"before-backup", "after-backup"} {
		t.Run(phase, func(t *testing.T) {
			dir, live, _ := upgradeWALFixture(t)
			update := func() error {
				_, err := live.Exec("UPDATE workbench_latency_samples SET latency_ms=53 WHERE seq=3")
				return err
			}
			err := migrateSchema(live, func() error {
				if phase == "before-backup" {
					if err := update(); err != nil {
						return err
					}
				}
				if err := createVerifiedV11Rollback(context.Background(), dir, upgradeBackupHooks{}); err != nil {
					return err
				}
				if phase == "after-backup" {
					return update()
				}
				return nil
			})
			if err == nil {
				t.Fatal("concurrent writer did not block stale migration")
			}
			migrationErr := err
			assertUpgradeVersion(t, live, 11)
			first := completedUpgradeSnapshots(t, dir)
			if len(first) != 1 {
				t.Fatal("valid snapshot lost after migration failed")
			}
			firstBytes, err := os.ReadFile(first[0])
			if err != nil {
				t.Fatal(err)
			}
			firstHash := sha256.Sum256(firstBytes)
			db, err := OpenDB(dir)
			if err != nil {
				t.Fatal(err)
			}
			db.Close()
			assertUpgradeVersion(t, live, 12)
			all := completedUpgradeSnapshots(t, dir)
			if len(all) != 2 {
				t.Fatal("retry reused stale snapshot")
			}
			for _, path := range all {
				if path != first[0] {
					raw, err := sql.Open("sqlite", path)
					if err != nil {
						t.Fatal(err)
					}
					var sample, version int
					if err = raw.QueryRow("SELECT latency_ms FROM workbench_latency_samples WHERE seq=3").Scan(&sample); err != nil {
						t.Fatal(err)
					}
					if err = raw.QueryRow("SELECT schema_version FROM schema_meta").Scan(&version); err != nil {
						t.Fatal(err)
					}
					raw.Close()
					if sample != 53 || version != 11 {
						t.Fatalf("fresh retry snapshot sample/schema=%d/%d", sample, version)
					}
				}
			}
			b, err := os.ReadFile(first[0])
			if err != nil || sha256.Sum256(b) != firstHash {
				t.Fatal("earlier complete backup overwritten")
			}
			t.Logf("%s: migration failed closed (%v); retry kept both verified snapshots", phase, migrationErr)
		})
	}
}

func TestV11UpgradeBackupOnlyExistingRecognizedV11(t *testing.T) {
	t.Run("new-and-v12", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".schema-upgrade-backups"), []byte("blocked"), 0600); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			db, err := OpenDB(dir)
			if err != nil {
				t.Fatal(err)
			}
			db.Close()
		}
	})
	t.Run("future", func(t *testing.T) {
		dir, live, _ := upgradeWALFixture(t)
		if _, err := live.Exec("UPDATE schema_meta SET schema_version=13"); err != nil {
			t.Fatal(err)
		}
		db, err := OpenDB(dir)
		if db != nil {
			db.Close()
		}
		if !errors.Is(err, ErrUnsupportedSchemaVersion) {
			t.Fatalf("future schema: %v", err)
		}
		assertUpgradeVersion(t, live, 13)
		if len(completedUpgradeSnapshots(t, dir)) != 0 {
			t.Fatal("unknown schema recorded as v11 backup")
		}
	})
	t.Run("no-private-bypass", func(t *testing.T) {
		_, live, _ := upgradeWALFixture(t)
		err := migrateSchema(live, nil)
		if !errors.Is(err, ErrUpgradeBackup) {
			t.Fatalf("missing callback: %v", err)
		}
		assertUpgradeVersion(t, live, 11)
	})
}

func TestV11UpgradeBackupConcurrentMigrators(t *testing.T) {
	dir, _, _ := upgradeWALFixture(t)
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			raw, err := sql.Open("sqlite", filepath.Join(dir, "history.db"))
			if err != nil {
				results <- err
				return
			}
			defer raw.Close()
			results <- migrateSchema(raw, func() error {
				ready <- struct{}{}
				<-release
				return createVerifiedV11Rollback(context.Background(), dir, upgradeBackupHooks{})
			})
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-ready:
		case <-time.After(5 * time.Second):
			t.Fatal("migrator did not reach gate")
		}
	}
	close(release)
	success := 0
	for i := 0; i < 2; i++ {
		select {
		case err := <-results:
			if err == nil {
				success++
			} else if !strings.Contains(strings.ToLower(err.Error()), "locked") && !strings.Contains(strings.ToLower(err.Error()), "busy") && !errors.Is(err, ErrUpgradeBackup) {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("migration hung")
		}
	}
	if success > 1 {
		t.Fatal("both stale v11 transactions upgraded")
	}
	if len(completedUpgradeSnapshots(t, dir)) == 0 {
		t.Fatal("no verified snapshot before concurrent DDL")
	}
	// A safe busy failure is recoverable via a whole fresh OpenDB attempt.
	db, err := OpenDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
}

// Removing the migration gate must fail these tests, even though migration itself succeeds.
func TestV11UpgradeBackupIncludesWALAndIsolatedRollback(t *testing.T) {
	dir, live, _ := upgradeWALFixture(t)
	db, err := OpenDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	assertUpgradeVersion(t, live, 12)
	snapshots := completedUpgradeSnapshots(t, dir)
	if len(snapshots) != 1 {
		t.Fatalf("completed pre-upgrade backups = %d, want 1", len(snapshots))
	}
	rollback, err := sql.Open("sqlite", snapshots[0])
	if err != nil {
		t.Fatal(err)
	}
	assertLegacyBackup(t, rollback)
	rollback.Close()
	// Restore into a new isolated directory without pointing the v12 authority at rollback.
	isolated := filepath.Join(t.TempDir(), "restored", "history.db")
	if err := BackupDatabase(context.Background(), filepath.Dir(snapshots[0]), isolated); err != nil {
		t.Fatal(err)
	}
	restored, err := sql.Open("sqlite", isolated)
	if err != nil {
		t.Fatal(err)
	}
	assertLegacyBackup(t, restored)
	restored.Close()
	candidate, err := OpenDB(filepath.Dir(isolated))
	if err != nil {
		t.Fatal(err)
	}
	var value int
	if err := candidate.db.QueryRow("SELECT latency_ms FROM workbench_latency_samples WHERE seq=3").Scan(&value); err != nil || value != 47 {
		t.Fatalf("restored WAL value=%d err=%v", value, err)
	}
	candidate.Close()
	rollback, err = sql.Open("sqlite", snapshots[0])
	if err != nil {
		t.Fatal(err)
	}
	defer rollback.Close()
	assertLegacyBackup(t, rollback)
	// Reopening v12 must not generate another backup.
	reopened, err := OpenDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	reopened.Close()
	if got := len(completedUpgradeSnapshots(t, dir)); got != 1 {
		t.Fatalf("v12 reopen created %d backups", got)
	}
}

func TestV11UpgradeBackupFailureBlocksAllOpeners(t *testing.T) {
	openers := map[string]func(string) error{
		"OpenDB": func(dir string) error {
			db, err := OpenDB(dir)
			if db != nil {
				db.Close()
			}
			return err
		},
		"ValidateDatabase": ValidateDatabase,
		"NewStore": func(dir string) error {
			s, err := NewStore(dir)
			if s != nil {
				s.Close()
			}
			return err
		},
		"NewStoreWithLegacyDir": func(dir string) error {
			s, err := NewStoreWithLegacyDir(dir, filepath.Join(dir, "legacy"))
			if s != nil {
				s.Close()
			}
			return err
		},
	}
	for name, open := range openers {
		t.Run(name, func(t *testing.T) {
			dir, live, hashes := upgradeWALFixture(t)
			if err := os.WriteFile(filepath.Join(dir, ".schema-upgrade-backups"), []byte("blocked"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := open(dir); err == nil {
				t.Fatal("backup failure did not block v11 upgrade")
			}
			assertUpgradeVersion(t, live, 11)
			assertUpgradeSourceHashes(t, hashes)
		})
	}
}

func TestV11UpgradeBackupRejectsSymlinkAndPermission(t *testing.T) {
	for _, mode := range []string{"symlink", "permission"} {
		t.Run(mode, func(t *testing.T) {
			dir, live, hashes := upgradeWALFixture(t)
			root := filepath.Join(dir, ".schema-upgrade-backups")
			if mode == "symlink" {
				if err := os.Symlink(t.TempDir(), root); err != nil {
					t.Fatal(err)
				}
			} else {
				if os.Geteuid() == 0 {
					t.Skip("root bypasses permissions")
				}
				if err := os.Mkdir(root, 0500); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.Chmod(root, 0700) })
			}
			db, err := OpenDB(dir)
			if db != nil {
				db.Close()
			}
			if err == nil {
				t.Fatal("unsafe backup destination did not block migration")
			}
			assertUpgradeVersion(t, live, 11)
			assertUpgradeSourceHashes(t, hashes)
		})
	}
}

func upgradeWALFixture(t *testing.T) (string, *sql.DB, map[string][32]byte) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "history")
	seedSchemaV11LatencyHistory(t, dir, time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC))
	path := filepath.Join(dir, "history.db")
	live, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { live.Close() })
	if _, err = live.Exec("PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=0; UPDATE workbench_latency_samples SET latency_ms=47 WHERE seq=3"); err != nil {
		t.Fatal(err)
	}
	hashes := map[string][32]byte{}
	for _, p := range []string{path, path + "-wal"} {
		b, err := os.ReadFile(p)
		if err != nil || len(b) == 0 {
			t.Fatal("missing active WAL fixture", err)
		}
		hashes[p] = sha256.Sum256(b)
	}
	return dir, live, hashes
}

func assertUpgradeSourceHashes(t *testing.T, hashes map[string][32]byte) {
	t.Helper()
	for p, want := range hashes {
		b, err := os.ReadFile(p)
		if err != nil || sha256.Sum256(b) != want {
			t.Fatalf("failed gate changed source %s: %v", p, err)
		}
	}
}

func assertUpgradeVersion(t *testing.T, raw *sql.DB, want int) {
	t.Helper()
	var got int
	if err := raw.QueryRow("SELECT schema_version FROM schema_meta WHERE singleton=1").Scan(&got); err != nil || got != want {
		t.Fatalf("schema=%d want=%d err=%v", got, want, err)
	}
}

func completedUpgradeSnapshots(t *testing.T, dir string) []string {
	t.Helper()
	manifests, err := filepath.Glob(filepath.Join(dir, ".schema-upgrade-backups", "v11-to-v12-*", "complete.json"))
	if err != nil {
		t.Fatal(err)
	}
	var snapshots []string
	for _, p := range manifests {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var m struct {
			From     int    `json:"from_schema"`
			To       int    `json:"to_schema"`
			Snapshot string `json:"snapshot"`
			SHA256   string `json:"sha256"`
		}
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		if m.From != 11 || m.To != 12 || m.Snapshot != "history.db" {
			t.Fatalf("invalid manifest %s", b)
		}
		path := filepath.Join(filepath.Dir(p), m.Snapshot)
		b, err = os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(b)
		if m.SHA256 != hex.EncodeToString(hash[:]) {
			t.Fatal("backup checksum mismatch")
		}
		for _, file := range []string{p, path} {
			info, err := os.Stat(file)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatalf("private file mode: %v %v", info, err)
			}
		}
		t.Logf("verified synthetic v11 snapshot SHA256=%s", m.SHA256)
		snapshots = append(snapshots, path)
	}
	return snapshots
}
