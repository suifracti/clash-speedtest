package history

import (
	"context"

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
