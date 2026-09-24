package history

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func workbenchDownloadFixture(attemptID, requestID string, at time.Time) *WorkbenchDownloadAttempt {
	return &WorkbenchDownloadAttempt{
		AttemptID: attemptID, RequestID: requestID, ProfileID: "profile-a", NodeKey: "node-a",
		NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a", DisplayName: "download node", NodeType: "http",
		Source: WorkbenchDownloadSource, RequestedAt: at.UTC(), ExecutionState: "queued", PersistenceState: "not_started",
		Rule: WorkbenchDownloadRuleSnapshot{RuleVersion: 1, TargetURL: "https://speed.cloudflare.com/__down?bytes=1001", Method: "GET", MaximumBytes: 1000, MaximumDurationNS: int64(10 * time.Second), SampleEveryBytes: 256 << 10, SampleEveryNS: int64(100 * time.Millisecond)},
	}
}

func TestWorkbenchDownloadRecoveryPreservesStagedResultAndMarksUnfinishedInterrupted(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "history")
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	at := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	unfinished := workbenchDownloadFixture("download-interrupted", "request-interrupted", at)
	staged := workbenchDownloadFixture("download-staged", "request-staged", at.Add(time.Second))
	for _, item := range []*WorkbenchDownloadAttempt{unfinished, staged} {
		if err := store.CreateWorkbenchDownloadAttempt(ctx, item); err != nil {
			t.Fatal(err)
		}
		if err := store.BeginWorkbenchDownloadAttempt(ctx, item.AttemptID, at); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.StageWorkbenchDownloadResult(ctx, staged.AttemptID, "completed", WorkbenchDownloadMeasurement{
		Outcome: "byte_limit", BytesRead: 1000, StartedAt: at, FinishedAt: at.Add(800 * time.Millisecond), DurationNS: int64(800 * time.Millisecond),
		Samples: []WorkbenchDownloadSample{{ElapsedNS: int64(800 * time.Millisecond), IntervalNS: int64(800 * time.Millisecond), DeltaBytes: 1000, CumulativeBytes: 1000}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.ReconcileWorkbenchDownloadAttempts(ctx); err != nil {
		t.Fatal(err)
	}
	filter := WorkbenchDownloadFilter{ProfileID: "profile-a", NodeKey: "node-a", NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a"}
	gotInterrupted, err := reopened.GetWorkbenchDownloadAttempt(ctx, unfinished.AttemptID, filter)
	if err != nil || gotInterrupted.ExecutionState != "interrupted" || gotInterrupted.Result != nil || gotInterrupted.PersistenceState != "not_applicable" {
		t.Fatalf("unfinished attempt recovery = %+v, err=%v", gotInterrupted, err)
	}
	gotStaged, err := reopened.GetWorkbenchDownloadAttempt(ctx, staged.AttemptID, filter)
	if err != nil || gotStaged.ExecutionState != "completed" || gotStaged.PersistenceState != "failed" || gotStaged.Result == nil || gotStaged.Result.BytesRead != 1000 {
		t.Fatalf("staged result recovery = %+v, err=%v", gotStaged, err)
	}
	if err := reopened.CommitWorkbenchDownloadResult(ctx, staged.AttemptID); err != nil {
		t.Fatal(err)
	}
	gotSaved, err := reopened.GetWorkbenchDownloadAttempt(ctx, staged.AttemptID, filter)
	if err != nil || gotSaved.PersistenceState != "saved" || gotSaved.AttemptID != staged.AttemptID {
		t.Fatalf("same-attempt retry result = %+v, err=%v", gotSaved, err)
	}
	if _, err := reopened.GetWorkbenchDownloadAttempt(ctx, staged.AttemptID, WorkbenchDownloadFilter{ProfileID: "profile-b", NodeKey: "node-a", NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a"}); err != ErrWorkbenchDownloadAttemptNotFound {
		t.Fatalf("cross-profile history read err = %v", err)
	}
}

func TestSchemaV6UpgradeAddsWorkbenchDownloadTablesAndRetainsMonitorBudget(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.db.Exec(`UPDATE schema_meta SET schema_version=6 WHERE singleton=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.db.Exec(`DROP TABLE workbench_download_results; DROP TABLE workbench_download_attempts`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.db.Exec(`UPDATE monitor_budget_usage SET utc_day='2026-09-23', requests_used=29, bytes_used=8192 WHERE singleton=1`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	upgraded, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var version, requests int
	var bytes int64
	if err := upgraded.db.db.QueryRow(`SELECT schema_version FROM schema_meta WHERE singleton=1`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := upgraded.db.db.QueryRow(`SELECT requests_used,bytes_used FROM monitor_budget_usage WHERE singleton=1`).Scan(&requests, &bytes); err != nil {
		t.Fatal(err)
	}
	if version != CurrentSchemaVersion || requests != 29 || bytes != 8192 {
		t.Fatalf("v6 upgrade changed existing facts: version=%d requests=%d bytes=%d", version, requests, bytes)
	}
	var attempts, results int
	if err := upgraded.db.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='workbench_download_attempts'`).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if err := upgraded.db.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='workbench_download_results'`).Scan(&results); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || results != 1 {
		t.Fatalf("download history tables missing: attempts=%d results=%d", attempts, results)
	}
}
