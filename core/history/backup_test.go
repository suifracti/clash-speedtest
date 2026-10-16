package history

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupDatabaseBeforeSchemaUpgrade(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "v11")
	at := time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)
	seedSchemaV11LatencyHistory(t, source, at)
	path := filepath.Join(source, "history.db")
	live, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if _, err := live.Exec("PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=0; UPDATE workbench_latency_samples SET latency_ms=47 WHERE seq=3"); err != nil {
		t.Fatal(err)
	}
	before := map[string][32]byte{}
	for _, p := range []string{path, path + "-wal"} {
		b, err := os.ReadFile(p)
		if err != nil || len(b) == 0 {
			t.Fatalf("active WAL fixture: %v", err)
		}
		before[p] = sha256.Sum256(b)
	}
	// A naked main-file copy loses the WAL-only committed sample.
	naked := filepath.Join(root, "naked.db")
	main, _ := os.ReadFile(path)
	if err := os.WriteFile(naked, main, 0600); err != nil {
		t.Fatal(err)
	}
	nakedDB, err := sql.Open("sqlite", naked)
	if err != nil {
		t.Fatal(err)
	}
	var nakedLatency int
	if err := nakedDB.QueryRow("SELECT latency_ms FROM workbench_latency_samples WHERE seq=3").Scan(&nakedLatency); err != nil {
		t.Fatal(err)
	}
	nakedDB.Close()
	if nakedLatency != 39 {
		t.Fatalf("negative control latency=%d, want 39", nakedLatency)
	}
	snapshot := filepath.Join(root, "rollback", "history.db")
	if err := BackupDatabase(context.Background(), source, snapshot); err != nil {
		t.Fatal(err)
	}
	for p, want := range before {
		b, err := os.ReadFile(p)
		if err != nil || sha256.Sum256(b) != want {
			t.Fatalf("backup modified source DB/WAL: %v", err)
		}
	}
	rollback, err := sql.Open("sqlite", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback.Close()
	assertLegacyBackup(t, rollback)
	// Only a separate candidate copy is ever handed to the v12 schema authority.
	candidate := filepath.Join(root, "candidate", "history.db")
	if err := BackupDatabase(context.Background(), filepath.Dir(snapshot), candidate); err != nil {
		t.Fatal(err)
	}
	upgraded, err := OpenDB(filepath.Dir(candidate))
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	got, err := upgraded.GetLatencyTest(context.Background(), "v11-attempt")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Samples) != 3 || got.Samples[2].LatencyMs != 47 {
		t.Fatalf("WAL sample after upgrade: %+v", got.Samples)
	}
	got.Samples[2].LatencyMs = 39 // compare the unchanged legacy fields separately from the WAL update
	assertV11LatencyHistoryPreserved(t, got, at)
	var latency, version int
	if err := upgraded.db.QueryRow("SELECT latency_ms FROM workbench_latency_samples WHERE seq=3").Scan(&latency); err != nil {
		t.Fatal(err)
	}
	if err := upgraded.db.QueryRow("SELECT schema_version FROM schema_meta").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if latency != 47 || version != 12 {
		t.Fatalf("candidate latency/version=%d/%d", latency, version)
	}
	assertLegacyBackup(t, rollback)
}

func assertLegacyBackup(t *testing.T, db *sql.DB) {
	t.Helper()
	var integrity string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("snapshot integrity=%s err=%v", integrity, err)
	}
	var version, latency, count int
	var identity string
	if err := db.QueryRow("SELECT schema_version FROM schema_meta").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT latency_ms FROM workbench_latency_samples WHERE seq=3").Scan(&latency); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT count(*) FROM workbench_latency_samples").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT node_identity_key FROM workbench_latency_tests WHERE attempt_id='v11-attempt'").Scan(&identity); err != nil {
		t.Fatal(err)
	}
	if version != 11 || latency != 47 || count != 3 || identity != "identity-a" {
		t.Fatalf("rollback version/latency/count/identity=%d/%d/%d/%s", version, latency, count, identity)
	}
	// Same v11 validator used when constructing the old schema; no migration.
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := validateCurrentSchema(tx, 11); err != nil {
		t.Fatal(err)
	}
	var absent any
	if err := tx.QueryRow("SELECT network_path_json FROM workbench_latency_tests").Scan(&absent); err == nil {
		t.Fatal("rollback contains v12 column")
	}
}

func TestBackupDatabaseFailsClosed(t *testing.T) {
	for _, mode := range []string{"existing", "cancelled", "permission", "missing"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "source")
			seedSchemaV11LatencyHistory(t, source, time.Now())
			target := filepath.Join(root, "out", "history.db")
			ctx := context.Background()
			switch mode {
			case "existing":
				os.MkdirAll(filepath.Dir(target), 0700)
				os.WriteFile(target, []byte("keep"), 0600)
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "permission":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses permission checks")
				}
				os.MkdirAll(filepath.Dir(target), 0700)
				os.Chmod(filepath.Dir(target), 0500)
				defer os.Chmod(filepath.Dir(target), 0700)
			case "missing":
				source = filepath.Join(root, "missing")
			}
			if err := BackupDatabase(ctx, source, target); err == nil {
				t.Fatal("failure reported success")
			}
			b, err := os.ReadFile(target)
			if mode == "existing" {
				if err != nil || string(b) != "keep" {
					t.Fatal("existing backup changed")
				}
			} else if !os.IsNotExist(err) {
				t.Fatalf("failed backup published target: %v", err)
			}
			if mode != "missing" {
				raw, err := sql.Open("sqlite", filepath.Join(source, "history.db"))
				if err != nil {
					t.Fatal(err)
				}
				var version int
				err = raw.QueryRow("SELECT schema_version FROM schema_meta").Scan(&version)
				raw.Close()
				if err != nil || version != 11 {
					t.Fatalf("failure changed source schema: %d %v", version, err)
				}
			}
			if mode == "missing" {
				if _, err := os.Stat(source); !os.IsNotExist(err) {
					t.Fatal("missing source created")
				}
			}
		})
	}
}

func TestBackupDatabaseConcurrentPublication(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	seedSchemaV11LatencyHistory(t, source, time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC))
	target := filepath.Join(root, "out", "history.db")
	ready := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { <-ready; results <- BackupDatabase(context.Background(), source, target) }()
	}
	close(ready)
	successes := 0
	for i := 0; i < 2; i++ {
		if <-results == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful publishers=%d, want exactly 1", successes)
	}
	raw, err := sql.Open("sqlite", target)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var version, count int
	if err := raw.QueryRow("SELECT schema_version FROM schema_meta").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := raw.QueryRow("SELECT count(*) FROM workbench_latency_samples").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if version != 11 || count != 3 {
		t.Fatalf("published version/count=%d/%d", version, count)
	}
	leftovers, err := filepath.Glob(filepath.Join(filepath.Dir(target), ".history-backup-*"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("private snapshot artifacts left: %v %v", leftovers, err)
	}
}

func TestBackupToInterruptedConnectionWaitCleansPrivateSnapshot(t *testing.T) {
	root := t.TempDir()
	db, err := OpenDB(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.db.SetMaxOpenConns(1)
	held, err := db.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	target := filepath.Join(root, "out", "history.db")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := db.BackupTo(ctx, target); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("interrupted wait error=%v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("interruption published target: %v", err)
	}
	leftovers, err := filepath.Glob(filepath.Join(filepath.Dir(target), ".history-backup-*"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("interruption leaked private snapshot: %v %v", leftovers, err)
	}
}

func TestBackupNilContexts(t *testing.T) {
	root := t.TempDir()
	db, err := OpenDB(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// BackupTo with nil context
	target1 := filepath.Join(root, "out1", "history.db")
	if err := db.BackupTo(nil, target1); err != nil {
		t.Fatalf("expected BackupTo with nil context to succeed, got %v", err)
	}
	if _, err := os.Stat(target1); err != nil {
		t.Fatalf("target1 does not exist: %v", err)
	}

	// BackupDatabase with nil context
	target2 := filepath.Join(root, "out2", "history.db")
	if err := BackupDatabase(nil, root, target2); err != nil {
		t.Fatalf("expected BackupDatabase with nil context to succeed, got %v", err)
	}
	if _, err := os.Stat(target2); err != nil {
		t.Fatalf("target2 does not exist: %v", err)
	}
}
