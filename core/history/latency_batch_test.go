package history

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestSchemaV4UpgradePreservesLatencyRawAndMonitorBudget(t *testing.T) {
	dir := t.TempDir()
	raw, err := sql.Open("sqlite", filepath.Join(dir, "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := raw.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := installSchemaVersion1(tx); err != nil {
		t.Fatalf("install v1: %v", err)
	}
	if _, err := tx.Exec(monitorBudgetDDL); err != nil {
		t.Fatalf("create budget ledger: %v", err)
	}
	if _, err := tx.Exec("UPDATE schema_meta SET schema_version=2 WHERE singleton=1"); err != nil {
		t.Fatal(err)
	}
	if err := migrateSamplingSourceV3(tx); err != nil {
		t.Fatalf("migrate v3: %v", err)
	}
	if _, err := tx.Exec("UPDATE schema_meta SET schema_version=3 WHERE singleton=1"); err != nil {
		t.Fatal(err)
	}
	if err := migrateMonitorLaunchRecoveryV4(tx); err != nil {
		t.Fatalf("migrate v4: %v", err)
	}
	if _, err := tx.Exec("UPDATE schema_meta SET schema_version=4 WHERE singleton=1"); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC)
	nowText := now.Format(time.RFC3339Nano)
	if _, err := tx.Exec(`INSERT INTO workbench_latency_tests (attempt_id,profile_id,node_key,node_identity_key,config_revision_key,display_name,node_type,test_project,requested_at,started_at,finished_at,status,latency_ms,jitter_ms,packet_loss,total_samples,success_samples,failure_samples,error_message) VALUES ('legacy-attempt','profile-a','node-a','identity-a','rev-a','A','http','latency_stability',?,?,?,?,?,?,?,1,1,0,NULL)`, nowText, nowText, nowText, "completed", 42, 3, 0); err != nil {
		t.Fatalf("seed old attempt: %v", err)
	}
	if _, err := tx.Exec(`INSERT INTO workbench_latency_samples(attempt_id,seq,timestamp,latency_ms,success,error) VALUES('legacy-attempt',1,?,42,1,NULL)`, nowText); err != nil {
		t.Fatalf("seed raw sample: %v", err)
	}
	if _, err := tx.Exec(`UPDATE monitor_budget_usage SET utc_day='2026-09-23',requests_used=11,bytes_used=1234 WHERE singleton=1`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	upgraded, err := OpenDB(dir)
	if err != nil {
		t.Fatalf("upgrade v4 database: %v", err)
	}
	defer upgraded.Close()
	var version int
	if err := upgraded.db.QueryRow("SELECT schema_version FROM schema_meta WHERE singleton=1").Scan(&version); err != nil || version != CurrentSchemaVersion {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
	attempt, err := upgraded.GetLatencyTest(context.Background(), "legacy-attempt")
	if err != nil {
		t.Fatalf("read upgraded attempt: %v", err)
	}
	if len(attempt.Samples) != 1 || attempt.Samples[0].LatencyMs != 42 || attempt.Source != "" || attempt.Method != "" || attempt.MethodVersion != 0 || attempt.Target != "" || attempt.Unit != "" {
		t.Fatalf("old raw attempt or unknown metadata changed: %+v", attempt)
	}
	usage, err := upgraded.MonitorBudgetUsage(context.Background(), "2026-09-23")
	if err != nil || usage.RequestsUsed != 11 || usage.BytesUsed != 1234 {
		t.Fatalf("Monitor ledger changed during migration: usage=%+v err=%v", usage, err)
	}
}
