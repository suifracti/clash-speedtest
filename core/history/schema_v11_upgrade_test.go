package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSchemaV11UpgradePreservesLatencyHistoryAndLeavesPathUnknown(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "history-v11")
	requestedAt := time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)
	seedSchemaV11LatencyHistory(t, dir, requestedAt)

	legacy, err := sql.Open("sqlite", filepath.Join(dir, "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	var version int
	if err := legacy.QueryRow("SELECT schema_version FROM schema_meta WHERE singleton=1").Scan(&version); err != nil {
		t.Fatal(err)
	}
	legacyColumns, err := legacy.Query("PRAGMA table_info(workbench_latency_tests)")
	if err != nil {
		t.Fatal(err)
	}
	for legacyColumns.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue sql.NullString
		if err := legacyColumns.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		if name == "network_path_json" {
			t.Fatal("v11 fixture unexpectedly contains the v12 path column")
		}
	}
	if err := legacyColumns.Err(); err != nil {
		t.Fatal(err)
	}
	if err := legacyColumns.Close(); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	if version != 11 {
		t.Fatalf("fixture schema version = %d, want 11", version)
	}

	db, err := OpenDB(dir)
	if err != nil {
		t.Fatalf("open and upgrade v11 database: %v", err)
	}
	ctx := context.Background()
	var upgradedVersion int
	if err := db.db.QueryRow("SELECT schema_version FROM schema_meta WHERE singleton=1").Scan(&upgradedVersion); err != nil {
		t.Fatal(err)
	}
	if upgradedVersion != CurrentSchemaVersion {
		t.Fatalf("upgraded schema version = %d, want %d", upgradedVersion, CurrentSchemaVersion)
	}

	got, err := db.GetLatencyTest(ctx, "v11-attempt")
	if err != nil {
		t.Fatalf("read v11 attempt after upgrade: %v", err)
	}
	assertV11LatencyHistoryPreserved(t, got, requestedAt)
	if len(got.NetworkPath) != 0 {
		t.Fatalf("v11 attempt path must remain unknown, got %s", got.NetworkPath)
	}

	pathEvidence := json.RawMessage(`{"method":"physical_socket_v2","interface":"en1","socket_bind_verified":true}`)
	postUpgrade := &LatencyTest{
		AttemptID: "v12-attempt", ProfileID: "profile-a", NodeKey: "node-a", NodeIdentityKey: "identity-a",
		ConfigRevisionKey: "revision-a", DisplayName: "A", NodeType: "http", TestProject: "latency_stability",
		Source: "single_node", Method: "physical_socket_v2", MethodVersion: 2, Target: "https://example.invalid", Unit: "ms",
		NetworkPath: pathEvidence, RequestedAt: requestedAt.Add(time.Minute), StartedAt: requestedAt.Add(time.Minute),
		FinishedAt: requestedAt.Add(2 * time.Minute), Status: "completed", TotalSamples: 1, SuccessSamples: 1,
		Samples: []LatencyTestSample{{Seq: 1, Timestamp: requestedAt.Add(90 * time.Second), LatencyMs: 27, Success: true, Target: "https://example.invalid"}},
	}
	if err := db.SaveLatencyTest(ctx, postUpgrade); err != nil {
		t.Fatalf("save v12 path evidence after upgrade: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenDB(dir)
	if err != nil {
		t.Fatalf("reopen upgraded database: %v", err)
	}
	defer reopened.Close()
	oldAfterReopen, err := reopened.GetLatencyTest(ctx, "v11-attempt")
	if err != nil {
		t.Fatalf("read v11 attempt after reopen: %v", err)
	}
	assertV11LatencyHistoryPreserved(t, oldAfterReopen, requestedAt)
	if len(oldAfterReopen.NetworkPath) != 0 {
		t.Fatalf("reopened v11 path must remain unknown, got %s", oldAfterReopen.NetworkPath)
	}
	newAfterReopen, err := reopened.GetLatencyTest(ctx, "v12-attempt")
	if err != nil {
		t.Fatalf("read v12 attempt after reopen: %v", err)
	}
	if string(newAfterReopen.NetworkPath) != string(pathEvidence) || len(newAfterReopen.Samples) != 1 || newAfterReopen.Samples[0].LatencyMs != 27 {
		t.Fatalf("v12 path evidence or sample changed after reopen: %+v", newAfterReopen)
	}
}

func seedSchemaV11LatencyHistory(t *testing.T, dir string, at time.Time) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", filepath.Join(dir, "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	tx, err := raw.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	// The shared base DDL predates v11; replay its historical migrations through
	// v11. The v12 path column is introduced only by migrateSchema's v11 step.
	v11DDL := ddlSchema
	if strings.Contains(v11DDL, "network_path_json") {
		t.Fatal("base DDL unexpectedly contains the v12 path column")
	}
	if _, err := tx.Exec(v11DDL); err != nil {
		t.Fatalf("create v11 base schema: %v", err)
	}
	if err := migrateLegacyMonitorColumns(tx); err != nil {
		t.Fatalf("create v11 monitor indexes: %v", err)
	}
	for _, ddl := range []string{monitorJobDDL, schemaMetaDDL, monitorBudgetDDL} {
		if _, err := tx.Exec(ddl); err != nil {
			t.Fatalf("create v11 base tables: %v", err)
		}
	}
	if _, err := tx.Exec("INSERT INTO schema_meta (singleton, schema_version) VALUES (1, 1)"); err != nil {
		t.Fatal(err)
	}
	for _, migration := range []func(*sql.Tx) error{
		migrateSamplingSourceV3,
		migrateMonitorLaunchRecoveryV4,
		migrateWorkbenchLatencyBatchesV5,
	} {
		if err := migration(tx); err != nil {
			t.Fatalf("build v11 historical schema: %v", err)
		}
	}
	for _, ddl := range []string{workbenchPublicServiceDDL, workbenchDownloadDDL} {
		if _, err := tx.Exec(ddl); err != nil {
			t.Fatalf("create v11 workbench tables: %v", err)
		}
	}
	for _, ddl := range []string{
		"ALTER TABLE workbench_latency_batches ADD COLUMN target_id TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE workbench_latency_samples ADD COLUMN target TEXT NOT NULL DEFAULT ''",
		subscriptionUsageDDL,
		measurementRoundSchema,
	} {
		if _, err := tx.Exec(ddl); err != nil {
			t.Fatalf("finish v11 schema: %v", err)
		}
	}
	if _, err := tx.Exec("UPDATE schema_meta SET schema_version=11 WHERE singleton=1"); err != nil {
		t.Fatal(err)
	}
	if err := validateCurrentSchema(tx, 11); err != nil {
		t.Fatalf("validate constructed v11 schema: %v", err)
	}
	if err := validateMonitorBudgetSchema(tx); err != nil {
		t.Fatalf("validate constructed v11 budget: %v", err)
	}

	base := at.Format(time.RFC3339Nano)
	if _, err := tx.Exec(`INSERT INTO workbench_latency_tests (
		attempt_id, profile_id, node_key, node_identity_key, config_revision_key, display_name, node_type,
		test_project, requested_at, started_at, finished_at, status, latency_ms, jitter_ms, packet_loss,
		total_samples, success_samples, failure_samples, error_message, source, method, method_version, target, unit
	) VALUES ('v11-attempt','profile-a','node-a','identity-a','revision-a','A','http',
		'latency_stability',?,?,?,'completed',35,4,0.25,3,2,1,NULL,'single_node','tcp_connect',1,'https://example.invalid','ms')`, base, base, base); err != nil {
		t.Fatalf("seed v11 latency attempt: %v", err)
	}
	for _, sample := range []struct {
		seq     int
		at      time.Time
		latency int
		success int
		errText any
	}{
		{seq: 1, at: at, latency: 31, success: 1},
		{seq: 2, at: at.Add(time.Second), latency: 0, success: 0, errText: "timeout"},
		{seq: 3, at: at.Add(2 * time.Second), latency: 39, success: 1},
	} {
		if _, err := tx.Exec(`INSERT INTO workbench_latency_samples(attempt_id,seq,timestamp,latency_ms,success,error,target) VALUES('v11-attempt',?,?,?,?,?,?)`, sample.seq, sample.at.Format(time.RFC3339Nano), sample.latency, sample.success, sample.errText, "https://example.invalid"); err != nil {
			t.Fatalf("seed v11 raw sample %d: %v", sample.seq, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit v11 fixture: %v", err)
	}
}

func assertV11LatencyHistoryPreserved(t *testing.T, got *LatencyTest, at time.Time) {
	t.Helper()
	if got.AttemptID != "v11-attempt" || got.ProfileID != "profile-a" || got.NodeKey != "node-a" || got.NodeIdentityKey != "identity-a" || got.ConfigRevisionKey != "revision-a" || got.DisplayName != "A" || got.NodeType != "http" || got.TestProject != "latency_stability" {
		t.Fatalf("v11 identity changed: %+v", got)
	}
	if got.Status != "completed" || got.LatencyMs != 35 || got.JitterMs != 4 || got.PacketLoss != 0.25 || got.TotalSamples != 3 || got.SuccessSamples != 2 || got.FailureSamples != 1 || got.Method != "tcp_connect" || got.MethodVersion != 1 || got.Target != "https://example.invalid" || got.Unit != "ms" {
		t.Fatalf("v11 attempt fields changed: %+v", got)
	}
	if len(got.Samples) != 3 {
		t.Fatalf("v11 raw sample count = %d, want 3: %+v", len(got.Samples), got.Samples)
	}
	wantSamples := []LatencyTestSample{
		{Seq: 1, Timestamp: at, LatencyMs: 31, Success: true, Target: "https://example.invalid"},
		{Seq: 2, Timestamp: at.Add(time.Second), Success: false, Error: "timeout", Target: "https://example.invalid"},
		{Seq: 3, Timestamp: at.Add(2 * time.Second), LatencyMs: 39, Success: true, Target: "https://example.invalid"},
	}
	for i, sample := range got.Samples {
		want := wantSamples[i]
		if sample.Seq != want.Seq || !sample.Timestamp.Equal(want.Timestamp) || sample.LatencyMs != want.LatencyMs || sample.Success != want.Success || sample.Error != want.Error || sample.Target != want.Target {
			t.Fatalf("v11 raw sample %d changed: got=%+v want=%+v", i, sample, want)
		}
	}
}
