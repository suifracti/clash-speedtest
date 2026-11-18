package history

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestMeasurementRoundKeepsUnexecutedChildrenAndRejectsChangedScope(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	r := MeasurementRound{RoundID: "round-a", TriggerType: "scheduled", StartedAt: time.Now().UTC(), Items: []MeasurementRoundItem{
		{RequestID: "request-a", Project: "service", ServiceID: "youtube", ProfileID: "p", NodeKey: "n", NodeIdentityKey: "i", ConfigRevisionKey: "v"},
		{RequestID: "request-b", Project: "service", ServiceID: "netflix", ProfileID: "p", NodeKey: "n", NodeIdentityKey: "i", ConfigRevisionKey: "v"},
	}}
	if err := db.CreateMeasurementRound(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateMeasurementRound(ctx, r); err != nil {
		t.Fatal("idempotent retry", err)
	}
	r.Items[1].ServiceID = "other"
	if err := db.CreateMeasurementRound(ctx, r); err == nil {
		t.Fatal("changed frozen scope accepted")
	}
	if err := db.FinishMeasurementRound(ctx, "round-a", "interrupted"); err != nil {
		t.Fatal(err)
	}
	got, err := db.QueryMeasurementRoundNodes(ctx, "p", "n", "i", "v", 16)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].Items) != 2 || got[0].Items[0].ExecutionState != "not_executed" || got[0].TriggerType != "scheduled" {
		t.Fatalf("planned children disappeared or were treated as failures: %+v", got)
	}
	db.Close()
	db, err = OpenDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err = db.QueryMeasurementRoundNodes(ctx, "p", "n", "i", "v", 16)
	if err != nil || len(got) != 1 || len(got[0].Items) != 2 {
		t.Fatalf("lost durable linkage: %+v %v", got, err)
	}
}

func TestManualRoundDirectoryPagesResultsAndPreservesExactAccounts(t *testing.T) {
	db, err := OpenDB(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	at := time.Date(2026, 11, 18, 6, 54, 0, 0, time.UTC)
	add := func(round MeasurementRound) {
		t.Helper()
		if err := db.CreateMeasurementRound(ctx, round); err != nil {
			t.Fatal(err)
		}
	}
	fixture := MeasurementRoundItem{ProfileID: "a", NodeKey: "same", NodeIdentityKey: "same-identity", ConfigRevisionKey: "rev", DisplayName: "Same name", ServiceID: "netflix_unlock", Project: "service"}
	for i := 0; i < 31; i++ {
		id := fmt.Sprintf("round-%02d", i)
		item := fixture
		item.RequestID = id
		add(MeasurementRound{RoundID: id, TriggerType: "manual", StartedAt: at, Items: []MeasurementRoundItem{item}})
	}
	background := fixture
	background.RequestID = "scheduled-request"
	add(MeasurementRound{RoundID: "scheduled", TriggerType: "scheduled", StartedAt: at.Add(time.Second), Items: []MeasurementRoundItem{background}})
	latencyOnly := fixture
	latencyOnly.RequestID, latencyOnly.Project, latencyOnly.ServiceID = "latency-request", "latency", ""
	add(MeasurementRound{RoundID: "latency", TriggerType: "manual", StartedAt: at.Add(time.Second), Items: []MeasurementRoundItem{latencyOnly}})
	long := MeasurementRound{RoundID: "unlock", TriggerType: "manual", StartedAt: at.Add(time.Second)}
	for i := 0; i < 51; i++ {
		item := fixture
		item.RequestID = fmt.Sprintf("unlock-%02d", i)
		if i == 50 {
			item.ProfileID = "b"
		}
		long.Items = append(long.Items, item)
	}
	add(long)
	if err := db.MarkMeasurementNotExecuted(ctx, "unlock-50", "规则明确跳过"); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishMeasurementRound(ctx, "unlock", "finished"); err != nil {
		t.Fatal(err)
	}
	first, err := db.ListManualRounds(ctx, "")
	if err != nil || len(first.Rounds) != 30 || first.NextBefore != "round-02" {
		t.Fatalf("manual directory first page: %+v %v", first, err)
	}
	if latest := first.Rounds[0]; latest.RoundID != long.RoundID || latest.NodeCount != 2 || latest.ServiceCount != 51 || latest.State != "finished" || !latest.StartedAt.Equal(long.StartedAt) || len(latest.ServiceIDs) != 1 || latest.ServiceIDs[0] != fixture.ServiceID {
		t.Fatalf("manual round summary: %+v", latest)
	}
	older, err := db.ListManualRounds(ctx, first.NextBefore)
	if err != nil || len(older.Rounds) != 2 || older.NextBefore != "" || older.Rounds[0].RoundID != "round-01" {
		t.Fatalf("manual directory stable cursor: %+v %v", older, err)
	}
	detail, err := db.ReadManualRound(ctx, long.RoundID, 0)
	if err != nil || len(detail.Items) != 50 || detail.NextOffset != 50 || detail.Items[49].ProfileID != "a" {
		t.Fatalf("first detail page lost frozen account: %+v %v", detail, err)
	}
	last, err := db.ReadManualRound(ctx, long.RoundID, detail.NextOffset)
	if err != nil || len(last.Items) != 1 || last.NextOffset != 0 || last.Items[0].ProfileID != "b" || last.Items[0].ExecutionState != "not_executed" || last.Items[0].NotExecutedReason != "规则明确跳过" {
		t.Fatalf("second detail page lost skip or scope: %+v %v", last, err)
	}
	if _, err := db.ReadManualRound(ctx, "scheduled", 0); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("scheduled round entered manual detail: %v", err)
	}
}

func TestMeasurementRoundSaveRetryChangesOnlyPersistenceAndLeavesLegacyUnknown(t *testing.T) {
	db, err := OpenDB(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	at := time.Now().UTC()
	r := MeasurementRound{RoundID: "save-root", TriggerType: "scheduled", StartedAt: at, Items: []MeasurementRoundItem{{RequestID: "save-child", Project: "download", ProfileID: "profile-a", NodeKey: "node-a", NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a"}}}
	if err = db.CreateMeasurementRound(ctx, r); err != nil {
		t.Fatal(err)
	}
	a := workbenchDownloadFixture("actual-child", "save-child", at)
	if err = db.CreateWorkbenchDownloadAttempt(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err = db.BeginWorkbenchDownloadAttempt(ctx, a.AttemptID, at); err != nil {
		t.Fatal(err)
	}
	m := WorkbenchDownloadMeasurement{Outcome: "time_limit", BytesRead: 4, StartedAt: at, FinishedAt: at.Add(time.Second), DurationNS: int64(time.Second), PartialMeasurement: true, Phase: "response_body", EndReason: "time_limit"}
	if err = db.StageWorkbenchDownloadResult(ctx, a.AttemptID, "completed", m); err != nil {
		t.Fatal(err)
	}
	if err = db.MarkWorkbenchDownloadSaveFailed(ctx, a.AttemptID, "injected disk error"); err != nil {
		t.Fatal(err)
	}
	before, err := db.QueryMeasurementRoundNodes(ctx, "profile-a", "node-a", "identity-a", "revision-a", 16)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 1 || len(before[0].Items) != 1 || before[0].Items[0].PersistenceState != "failed" || before[0].Items[0].Download.TriggerType != "scheduled" {
		t.Fatalf("save error changed round or execution: %+v", before)
	}
	if err = db.CommitWorkbenchDownloadResult(ctx, a.AttemptID); err != nil {
		t.Fatal(err)
	}
	after, err := db.QueryMeasurementRoundNodes(ctx, "profile-a", "node-a", "identity-a", "revision-a", 16)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || len(after[0].Items) != 1 || after[0].Items[0].Download.AttemptID != "actual-child" || after[0].Items[0].PersistenceState != "saved" || after[0].Items[0].Download.Result.BytesRead != 4 {
		t.Fatalf("save retry manufactured a measurement: %+v", after)
	}
	old := workbenchDownloadFixture("legacy", "legacy-request", at)
	if err = db.CreateWorkbenchDownloadAttempt(ctx, old); err != nil {
		t.Fatal(err)
	}
	legacy, err := db.GetWorkbenchDownloadAttemptByRequestID(ctx, "legacy-request")
	if err != nil || legacy.RoundID != "" || legacy.TriggerType != "" {
		t.Fatalf("unlinked old record was inferred: %+v %v", legacy, err)
	}
}
