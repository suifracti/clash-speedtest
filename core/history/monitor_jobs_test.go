package history

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
	_ "modernc.org/sqlite"
)

func TestSQLite_MonitorJobDefinitionPersistsOnlySafeReferences(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	definition := &monitor.MonitorJobDefinition{
		ID:        "job-persisted",
		Name:      "Night watch",
		ProfileID: "profile-a",
		Nodes: []monitor.MonitorJobNodeReference{{
			NodeKey:           "nk-profile-a-node",
			NodeIdentityKey:   "nid-profile-a-node",
			ConfigRevisionKey: "rev-profile-a-node",
			DisplayName:       "Shared name",
			Type:              "ss",
		}},
		ProbeSet:          monitor.ProbeSetLight,
		Interval:          30 * time.Second,
		Timeout:           5 * time.Second,
		CreatedAt:         time.Date(2026, 9, 22, 1, 0, 0, 0, time.UTC),
		UpdatedAt:         time.Date(2026, 9, 22, 1, 1, 0, 0, time.UTC),
		DefinitionVersion: monitor.MonitorJobDefinitionVersion,
	}
	if err := store.SaveMonitorJobDefinition(context.Background(), definition); err != nil {
		t.Fatalf("SaveMonitorJobDefinition: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}

	reopened, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer reopened.Close()
	definitions, err := reopened.ListMonitorJobDefinitions(context.Background())
	if err != nil {
		t.Fatalf("ListMonitorJobDefinitions: %v", err)
	}
	if len(definitions) != 1 {
		t.Fatalf("expected one definition, got %d", len(definitions))
	}
	got := definitions[0]
	if got.ID != definition.ID || got.ProfileID != definition.ProfileID || got.ProbeSet != definition.ProbeSet ||
		got.Interval != definition.Interval || got.Timeout != definition.Timeout || len(got.Nodes) != 1 {
		t.Fatalf("definition did not round-trip: got=%+v want=%+v", got, definition)
	}
	if got.Nodes[0] != definition.Nodes[0] {
		t.Fatalf("node reference did not round-trip: got=%+v want=%+v", got.Nodes[0], definition.Nodes[0])
	}

	var rawConfigColumns int
	if err := reopened.db.db.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('monitor_job_definitions')
		WHERE name IN ('raw_config', 'password', 'subscription_url', 'controller_secret')
	`).Scan(&rawConfigColumns); err != nil {
		t.Fatalf("inspect persisted job columns: %v", err)
	}
	if rawConfigColumns != 0 {
		t.Fatalf("persisted job schema contains credential-bearing columns: %d", rawConfigColumns)
	}
}

func TestSQLite_LegacySchemaUpgradeRecordsVersionAndIsIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "history.db")
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	_, err = rawDB.Exec(`
		CREATE TABLE monitor_runs (
			run_id TEXT PRIMARY KEY,
			job_id TEXT NOT NULL,
			scheduled_at DATETIME NOT NULL,
			started_at DATETIME NOT NULL,
			status TEXT NOT NULL
		);
		CREATE TABLE monitor_samples (
			sample_id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			node_key TEXT NOT NULL,
			profile_id TEXT NOT NULL,
			display_name_snapshot TEXT NOT NULL,
			probe_type TEXT NOT NULL,
			target TEXT NOT NULL,
			timestamp DATETIME NOT NULL,
			success INTEGER NOT NULL,
			latency_ms INTEGER NOT NULL DEFAULT 0,
			ttfb_ms INTEGER NOT NULL DEFAULT 0,
			error_class TEXT NOT NULL DEFAULT 'none'
		);
	`)
	if err != nil {
		t.Fatalf("create known legacy schema: %v", err)
	}
	if _, err := rawDB.Exec(`INSERT INTO monitor_samples (sample_id, run_id, node_key, profile_id, display_name_snapshot, probe_type, target, timestamp, success) VALUES ('legacy', 'run', 'nk', 'profile', 'node', 'rtt', 'target', ?, 1)`, time.Now().UTC()); err != nil {
		t.Fatalf("insert legacy raw sample: %v", err)
	}
	if err := rawDB.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("upgrade known legacy schema: %v", err)
	}
	var version int
	if err := store.db.db.QueryRow("SELECT schema_version FROM schema_meta WHERE singleton = 1").Scan(&version); err != nil {
		t.Fatalf("read migrated schema version: %v", err)
	}
	if version != CurrentSchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, CurrentSchemaVersion)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close migrated store: %v", err)
	}

	reopened, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("idempotent reopen: %v", err)
	}
	defer reopened.Close()
	var count int
	if err := reopened.db.db.QueryRow("SELECT COUNT(*) FROM monitor_samples WHERE sample_id = 'legacy'").Scan(&count); err != nil {
		t.Fatalf("read preserved legacy sample: %v", err)
	}
	if count != 1 {
		t.Fatalf("legacy sample count = %d, want 1", count)
	}
}

func TestSQLite_UnknownFutureSchemaFailsClosed(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "history.db")
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	if _, err := rawDB.Exec(`CREATE TABLE schema_meta (singleton INTEGER PRIMARY KEY CHECK (singleton = 1), schema_version INTEGER NOT NULL); INSERT INTO schema_meta (singleton, schema_version) VALUES (1, ?);`, CurrentSchemaVersion+1); err != nil {
		t.Fatalf("create future schema marker: %v", err)
	}
	if err := rawDB.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	store, err := NewStore(tmpDir)
	if err == nil {
		_ = store.Close()
		t.Fatal("expected future schema to fail closed")
	}
	if !errors.Is(err, ErrUnsupportedSchemaVersion) {
		t.Fatalf("expected ErrUnsupportedSchemaVersion, got %v", err)
	}

	check, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("reopen future schema for inspection: %v", err)
	}
	defer check.Close()
	var jobTables int
	if err := check.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'monitor_job_definitions'").Scan(&jobTables); err != nil {
		t.Fatalf("inspect failed migration: %v", err)
	}
	if jobTables != 0 {
		t.Fatal("future schema failure partially created monitor job tables")
	}
}
