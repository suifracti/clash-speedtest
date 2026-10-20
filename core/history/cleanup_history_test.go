package history

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func cleanupAttemptFixture(t *testing.T, d *DB, kind, id, state, persistence string, requested time.Time, finished *time.Time) {
	t.Helper()
	ctx := context.Background()
	var table string
	if kind == "download" {
		table = "workbench_download_attempts"
		if err := d.CreateWorkbenchDownloadAttempt(ctx, workbenchDownloadFixture(id, "request-"+id, requested)); err != nil {
			t.Fatal(err)
		}
	} else {
		table = "workbench_public_service_attempts"
		if err := d.CreatePublicServiceAttempt(ctx, publicServiceAttemptFixture(id, "request-"+id, "profile", "node", "identity", "revision", "fixture", requested)); err != nil {
			t.Fatal(err)
		}
	}
	var finishedAt any
	if finished != nil {
		finishedAt = finished.UTC()
	}
	if _, err := d.db.Exec(`UPDATE `+table+` SET execution_state=?,persistence_state=?,finished_at=? WHERE attempt_id=?`, state, persistence, finishedAt, id); err != nil {
		t.Fatal(err)
	}
}

func cleanupLatencyFixture(t *testing.T, d *DB, at time.Time) {
	t.Helper()
	if err := d.SaveLatencyTest(context.Background(), &LatencyTest{AttemptID: "latency-old", ProfileID: "profile", NodeKey: "node", NodeIdentityKey: "identity", ConfigRevisionKey: "revision", DisplayName: "fixture", NodeType: "direct", TestProject: "latency", RequestedAt: at, StartedAt: at, FinishedAt: at, Status: "completed", Samples: []LatencyTestSample{{Seq: 0, Timestamp: at, Success: true}}}); err != nil {
		t.Fatal(err)
	}
}

func cleanupBatchFixture(t *testing.T, d *DB, id, state, itemState, persistence string, at time.Time) {
	t.Helper()
	if err := d.CreateLatencyBatch(context.Background(), &LatencyBatch{BatchID: id, RequestID: "request-" + id, TestProject: "latency", TimeoutSeconds: 1, RequestedAt: at, State: state, Items: []LatencyBatchItem{{ItemID: "item-" + id, ProfileID: "profile", NodeKey: "node", NodeIdentityKey: "identity", ConfigRevisionKey: "revision", ExecutionState: itemState, PersistenceState: persistence, RequestedAt: at}}}); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupWorkbenchHistoryIncludesInterruptedAndPreservesLiveWork(t *testing.T) {
	d, err := OpenDB(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	cutoff := time.Now().UTC().Add(-30 * 24 * time.Hour)
	old, recent := cutoff.Add(-time.Hour), cutoff.Add(time.Hour)
	for _, kind := range []string{"download", "service"} {
		cleanupAttemptFixture(t, d, kind, kind+"-interrupted", "interrupted", "not_applicable", old, nil)
		cleanupAttemptFixture(t, d, kind, kind+"-completed", "completed", "saved", old, &old)
		cleanupAttemptFixture(t, d, kind, kind+"-recent", "interrupted", "not_applicable", recent, nil)
		cleanupAttemptFixture(t, d, kind, kind+"-running", "running", "not_started", old, nil)
		cleanupAttemptFixture(t, d, kind, kind+"-saving", "completed", "saving", old, &old)
		cleanupAttemptFixture(t, d, kind, kind+"-late-finish", "completed", "saved", old, &recent)
	}
	cleanupLatencyFixture(t, d, old)
	cleanupBatchFixture(t, d, "batch-old", "interrupted", "interrupted", "not_applicable", old)
	cleanupBatchFixture(t, d, "batch-live", "running", "running", "pending", old)
	cleanupBatchFixture(t, d, "batch-saving", "completed", "completed", "saving", old)
	for _, entry := range []struct{ table, id string }{{"workbench_download_results", "download-completed"}, {"workbench_public_service_results", "service-completed"}} {
		if _, err := d.db.Exec(`INSERT INTO `+entry.table+` (attempt_id,result_json,saved_at) VALUES (?,'{}',?)`, entry.id, old); err != nil {
			t.Fatal(err)
		}
	}
	n, err := d.CleanupWorkbenchHistory(context.Background(), cutoff)
	if err != nil || n != 6 {
		t.Fatalf("cleanup count=%d err=%v, want 6 parent records", n, err)
	}
	for _, table := range []string{"workbench_download_attempts", "workbench_public_service_attempts"} {
		var count int
		if err := d.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 4 {
			t.Errorf("%s retained %d records, want recent/running/saving/late-finish", table, count)
		}
	}
	for _, table := range []string{"workbench_latency_samples", "workbench_download_results", "workbench_public_service_results"} {
		var count int
		if err := d.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("%s left %d children", table, count)
		}
	}
	var batches, items int
	if err := d.db.QueryRow(`SELECT COUNT(*) FROM workbench_latency_batches`).Scan(&batches); err != nil {
		t.Fatal(err)
	}
	if err := d.db.QueryRow(`SELECT COUNT(*) FROM workbench_latency_batch_items`).Scan(&items); err != nil {
		t.Fatal(err)
	}
	if batches != 2 || items != 2 {
		t.Fatalf("active batches/items removed: %d/%d", batches, items)
	}
}

func TestCleanupWorkbenchHistoryRollsBackAndReportsFailure(t *testing.T) {
	d, err := OpenDB(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	cutoff := time.Now().UTC().Add(-time.Hour)
	old := cutoff.Add(-time.Hour)
	cleanupLatencyFixture(t, d, old)
	cleanupBatchFixture(t, d, "batch-old", "completed", "completed", "saved", old)
	cleanupAttemptFixture(t, d, "service", "service-old", "completed", "saved", old, &old)
	cleanupAttemptFixture(t, d, "download", "download-old", "completed", "saved", old, &old)
	if _, err := d.db.Exec(`CREATE TRIGGER cleanup_fixture_failure BEFORE DELETE ON workbench_public_service_attempts BEGIN SELECT RAISE(ABORT,'fixture delete failure'); END`); err != nil {
		t.Fatal(err)
	}
	n, err := d.CleanupWorkbenchHistory(context.Background(), cutoff)
	if err == nil || n != 0 || !strings.Contains(err.Error(), "workbench_public_service_attempts") {
		t.Fatalf("delete failure hidden/count committed: %d %v", n, err)
	}
	for _, table := range []string{"workbench_latency_tests", "workbench_latency_samples", "workbench_latency_batches", "workbench_latency_batch_items", "workbench_public_service_attempts", "workbench_download_attempts"} {
		var count int
		if err := d.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("%s partially deleted despite failed cleanup: %d", table, count)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if n, err := d.CleanupWorkbenchHistory(ctx, cutoff); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled cleanup: %d %v", n, err)
	}
}
