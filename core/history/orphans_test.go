package history

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestInspectOrphansReadOnlyWithoutMigrationOrRepair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture #& history.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = raw.Exec(`
		CREATE TABLE schema_meta (singleton INTEGER PRIMARY KEY, schema_version INTEGER);
		INSERT INTO schema_meta VALUES (1,99);
		CREATE TABLE parents (id TEXT PRIMARY KEY);
		CREATE TABLE children (id TEXT PRIMARY KEY, parent_id TEXT REFERENCES parents(id));
		INSERT INTO parents VALUES ('present');
		INSERT INTO children VALUES ('valid','present'),('orphan1','missing'),('orphan2','missing');
		CREATE TABLE monitor_runs (run_id TEXT PRIMARY KEY);
		CREATE TABLE monitor_samples (sample_id TEXT PRIMARY KEY, run_id TEXT);
		INSERT INTO monitor_runs VALUES ('present');
		INSERT INTO monitor_samples VALUES ('valid','present'),('orphan','missing');
	`)
	if err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	report, err := InspectOrphans(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	want := []OrphanGroup{
		{Table: "children", Parent: "parents", Kind: "foreign_key", Rows: 2},
		{Table: "monitor_samples", Parent: "monitor_runs", Kind: "monitor_run", Rows: 1},
	}
	if !report.ReadOnly || report.TotalViolations != 3 || !reflect.DeepEqual(report.Groups, want) {
		t.Fatalf("wrong report: %+v", report)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("read-only report changed database bytes")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("report created migration/journal artifacts: %v %v", entries, err)
	}
}

func TestInspectOrphansDoesNotCreateMissingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	if _, err := InspectOrphans(context.Background(), path); err == nil {
		t.Fatal("missing database accepted")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("database was created: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := InspectOrphans(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}
