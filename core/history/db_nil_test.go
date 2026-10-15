package history

import (
	"context"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
)

func TestDBNilReceiverSafety(t *testing.T) {
	var d *DB = nil
	ctx := context.Background()
	now := time.Now()

	// Ensure no panics on nil *DB
	d.SetTestBatchFailAt(5)
	if err := d.Close(); err != nil {
		t.Fatalf("expected nil err on Close() for nil *DB, got %v", err)
	}
	if err := d.SaveMonitorJobDefinition(ctx, &monitor.MonitorJobDefinition{ID: "job1"}); err == nil {
		t.Fatal("expected error on SaveMonitorJobDefinition")
	}
	if err := d.UpdateMonitorJobLaunchIntent(ctx, "job1", true, monitor.JobStateRunning, now); err == nil {
		t.Fatal("expected error on UpdateMonitorJobLaunchIntent")
	}
	if err := d.DeleteMonitorJobDefinition(ctx, "job1"); err == nil {
		t.Fatal("expected error on DeleteMonitorJobDefinition")
	}
	if err := d.UpdateMonitorJobSamplingTier(ctx, "job1", monitor.SamplingTierRegular, now); err == nil {
		t.Fatal("expected error on UpdateMonitorJobSamplingTier")
	}
	if _, err := d.ListMonitorJobDefinitions(ctx); err == nil {
		t.Fatal("expected error on ListMonitorJobDefinitions")
	}
	if _, err := d.listMonitorJobNodes(ctx, "job1"); err == nil {
		t.Fatal("expected error on listMonitorJobNodes")
	}
	if err := d.SaveMonitorRun(ctx, &monitor.MonitorRun{RunID: "run1"}); err == nil {
		t.Fatal("expected error on SaveMonitorRun")
	}
	if err := d.UpdateMonitorRun(ctx, &monitor.MonitorRun{RunID: "run1"}); err == nil {
		t.Fatal("expected error on UpdateMonitorRun")
	}
	if err := d.MarkRunningMonitorRunsInterrupted(ctx); err == nil {
		t.Fatal("expected error on MarkRunningMonitorRunsInterrupted")
	}
	if err := d.SaveMonitorSamples(ctx, []*monitor.MonitorSample{{SampleID: "s1"}}); err == nil {
		t.Fatal("expected error on SaveMonitorSamples")
	}
	if err := d.SaveLatencyTest(ctx, &LatencyTest{AttemptID: "att1", ProfileID: "p1", NodeKey: "n1", TestProject: "proj1"}); err == nil {
		t.Fatal("expected error on SaveLatencyTest")
	}
	if _, err := d.QueryLatencyTests(ctx, LatencyTestFilter{ProfileID: "p1", NodeKey: "n1"}); err == nil {
		t.Fatal("expected error on QueryLatencyTests")
	}
	if _, err := d.GetLatencyTest(ctx, "att1"); err == nil {
		t.Fatal("expected error on GetLatencyTest")
	}
	if _, err := d.GetLatencyTestInWindow(ctx, "att1", nil, nil); err == nil {
		t.Fatal("expected error on GetLatencyTestInWindow")
	}
	if _, err := d.ListNodeHistoryRevisions(ctx, "p1", "id1"); err == nil {
		t.Fatal("expected error on ListNodeHistoryRevisions")
	}
	if _, err := d.QueryMonitorRuns(ctx, "job1", 10); err == nil {
		t.Fatal("expected error on QueryMonitorRuns")
	}
	if _, err := d.QueryMonitorSamples(ctx, monitor.SampleFilter{}); err == nil {
		t.Fatal("expected error on QueryMonitorSamples")
	}
	if _, err := d.GetNodeTimelineSamples(ctx, "n1", now); err == nil {
		t.Fatal("expected error on GetNodeTimelineSamples")
	}
	if _, err := d.QueryMonitorSamplesCursor(ctx, monitor.CursorFilter{}); err == nil {
		t.Fatal("expected error on QueryMonitorSamplesCursor")
	}
	if _, err := d.GetDerivedStats(ctx, monitor.StatsQuery{}); err == nil {
		t.Fatal("expected error on GetDerivedStats")
	}
	if _, err := d.GetMonitorSampleFacets(ctx, now, now, 10, 10); err == nil {
		t.Fatal("expected error on GetMonitorSampleFacets")
	}
	if _, _, err := d.distinctSampleValues(ctx, "target", now, now, 10); err == nil {
		t.Fatal("expected error on distinctSampleValues")
	}
	if _, err := d.ApplyRetention(ctx, monitor.RetentionRequest{Policy: monitor.Retention30d}); err == nil {
		t.Fatal("expected error on ApplyRetention")
	}
	if _, err := d.CleanupWorkbenchHistory(ctx, now); err == nil {
		t.Fatal("expected error on CleanupWorkbenchHistory")
	}

	// IP Reputation & Budget
	if _, err := d.LoadIPReputation(ctx, "key1", now); err == nil {
		t.Fatal("expected error on LoadIPReputation")
	}
	if err := d.SaveIPReputation(ctx, "key1", "{}", now); err == nil {
		t.Fatal("expected error on SaveIPReputation")
	}
	if _, err := d.CheckIPReputationBudget(ctx, now); err == nil {
		t.Fatal("expected error on CheckIPReputationBudget")
	}
	if _, err := d.ReserveIPReputation(ctx, now); err == nil {
		t.Fatal("expected error on ReserveIPReputation")
	}
	if err := d.CoolIPReputation(ctx, now); err == nil {
		t.Fatal("expected error on CoolIPReputation")
	}

	// Public service latest
	if _, err := d.LatestPublicServiceChecks(ctx, "p", "id", "rev"); err == nil {
		t.Fatal("expected error on LatestPublicServiceChecks")
	}

	// Measurement round
	if err := d.CreateMeasurementRound(ctx, MeasurementRound{RoundID: "r1", TriggerType: "manual", StartedAt: now, Items: []MeasurementRoundItem{{RequestID: "req1", Project: "download", ProfileID: "p1", NodeKey: "n1", NodeIdentityKey: "nid1", ConfigRevisionKey: "crk1"}}}); err == nil {
		t.Fatal("expected error on CreateMeasurementRound")
	}
	if err := d.FinishMeasurementRound(ctx, "r1", "finished"); err == nil {
		t.Fatal("expected error on FinishMeasurementRound")
	}
	if err := d.ReconcileMeasurementRounds(ctx); err == nil {
		t.Fatal("expected error on ReconcileMeasurementRounds")
	}
	if _, err := d.MeasurementRequestTrigger(ctx, "req1", "p1", "nid1", "rev1", "download", ""); err == nil {
		t.Fatal("expected error on MeasurementRequestTrigger")
	}
	if err := d.MarkMeasurementNotExecuted(ctx, "req1", "reason"); err == nil {
		t.Fatal("expected error on MarkMeasurementNotExecuted")
	}
	if _, err := d.QueryMeasurementRoundNodes(ctx, "p1", "n1", "nid1", "crk1", 10); err == nil {
		t.Fatal("expected error on QueryMeasurementRoundNodes")
	}
	if err := d.FinishManualMeasurementRound(ctx, "r1", "finished"); err == nil {
		t.Fatal("expected error on FinishManualMeasurementRound")
	}
	if err := d.FinishAutomaticMeasurementRound(ctx, "req1"); err == nil {
		t.Fatal("expected error on FinishAutomaticMeasurementRound")
	}

	// Latency batch
	if err := d.CreateLatencyBatch(ctx, &LatencyBatch{BatchID: "b1", RequestID: "r1", Items: []LatencyBatchItem{{ItemID: "i1"}}}); err == nil {
		t.Fatal("expected error on CreateLatencyBatch")
	}
	if err := d.UpdateLatencyBatchItem(ctx, &LatencyBatchItem{ItemID: "i1", BatchID: "b1"}); err == nil {
		t.Fatal("expected error on UpdateLatencyBatchItem")
	}
	if err := d.UpdateLatencyBatchState(ctx, "b1", "completed"); err == nil {
		t.Fatal("expected error on UpdateLatencyBatchState")
	}
	if _, err := d.GetLatencyBatch(ctx, "b1"); err == nil {
		t.Fatal("expected error on GetLatencyBatch")
	}
	if _, err := d.FindLatencyBatchByRequestID(ctx, "req1"); err == nil {
		t.Fatal("expected error on FindLatencyBatchByRequestID")
	}
	if _, err := d.ListLatencyBatches(ctx, 10); err == nil {
		t.Fatal("expected error on ListLatencyBatches")
	}
	if _, err := d.ListLatencyBatchesNeedingRecovery(ctx); err == nil {
		t.Fatal("expected error on ListLatencyBatchesNeedingRecovery")
	}

	// Public service attempt
	if err := d.CreatePublicServiceAttempt(ctx, &PublicServiceAttempt{AttemptID: "ps1"}); err == nil {
		t.Fatal("expected error on CreatePublicServiceAttempt")
	}
	if err := d.BeginPublicServiceAttempt(ctx, "ps1", now); err == nil {
		t.Fatal("expected error on BeginPublicServiceAttempt")
	}
	if _, err := d.RequestPublicServiceCancellation(ctx, "ps1"); err == nil {
		t.Fatal("expected error on RequestPublicServiceCancellation")
	}
	if err := d.StagePublicServiceResult(ctx, "ps1", "completed", PublicServiceMeasurement{}); err == nil {
		t.Fatal("expected error on StagePublicServiceResult")
	}
	if err := d.MarkPublicServiceStageFailed(ctx, "ps1", "failed", now, "err"); err == nil {
		t.Fatal("expected error on MarkPublicServiceStageFailed")
	}
	if err := d.MarkPublicServiceSaveFailed(ctx, "ps1", "err"); err == nil {
		t.Fatal("expected error on MarkPublicServiceSaveFailed")
	}
	if err := d.CommitPublicServiceResult(ctx, "ps1"); err == nil {
		t.Fatal("expected error on CommitPublicServiceResult")
	}
	if err := d.ReconcilePublicServiceAttempts(ctx); err == nil {
		t.Fatal("expected error on ReconcilePublicServiceAttempts")
	}
	if _, err := d.GetPublicServiceAttemptByRequestID(ctx, "req1"); err == nil {
		t.Fatal("expected error on GetPublicServiceAttemptByRequestID")
	}
	if _, err := d.GetPublicServiceAttempt(ctx, "ps1", PublicServiceFilter{}); err == nil {
		t.Fatal("expected error on GetPublicServiceAttempt")
	}
	if _, err := d.QueryPublicServiceAttempts(ctx, PublicServiceFilter{}); err == nil {
		t.Fatal("expected error on QueryPublicServiceAttempts")
	}

	// Workbench download attempt
	if err := d.CreateWorkbenchDownloadAttempt(ctx, &WorkbenchDownloadAttempt{AttemptID: "dl1"}); err == nil {
		t.Fatal("expected error on CreateWorkbenchDownloadAttempt")
	}
	if err := d.BeginWorkbenchDownloadAttempt(ctx, "dl1", now); err == nil {
		t.Fatal("expected error on BeginWorkbenchDownloadAttempt")
	}
	if _, err := d.RequestWorkbenchDownloadCancellation(ctx, "dl1"); err == nil {
		t.Fatal("expected error on RequestWorkbenchDownloadCancellation")
	}
	if err := d.StageWorkbenchDownloadResult(ctx, "dl1", "completed", WorkbenchDownloadMeasurement{}); err == nil {
		t.Fatal("expected error on StageWorkbenchDownloadResult")
	}
	if err := d.MarkWorkbenchDownloadStageFailed(ctx, "dl1", "failed", now, "err"); err == nil {
		t.Fatal("expected error on MarkWorkbenchDownloadStageFailed")
	}
	if err := d.MarkWorkbenchDownloadSaveFailed(ctx, "dl1", "err"); err == nil {
		t.Fatal("expected error on MarkWorkbenchDownloadSaveFailed")
	}
	if err := d.CommitWorkbenchDownloadResult(ctx, "dl1"); err == nil {
		t.Fatal("expected error on CommitWorkbenchDownloadResult")
	}
	if err := d.ReconcileWorkbenchDownloadAttempts(ctx); err == nil {
		t.Fatal("expected error on ReconcileWorkbenchDownloadAttempts")
	}
	if _, err := d.GetWorkbenchDownloadAttemptByRequestID(ctx, "req1"); err == nil {
		t.Fatal("expected error on GetWorkbenchDownloadAttemptByRequestID")
	}
	if _, err := d.GetWorkbenchDownloadAttempt(ctx, "dl1", WorkbenchDownloadFilter{}); err == nil {
		t.Fatal("expected error on GetWorkbenchDownloadAttempt")
	}
	if _, err := d.QueryWorkbenchDownloadAttempts(ctx, WorkbenchDownloadFilter{}); err == nil {
		t.Fatal("expected error on QueryWorkbenchDownloadAttempts")
	}
}

func TestDBClosedSafety(t *testing.T) {
	d := &DB{}
	ctx := context.Background()
	now := time.Now()

	// Ensure no panics on &DB{db: nil}
	d.SetTestBatchFailAt(5)
	if err := d.Close(); err != nil {
		t.Fatalf("expected nil err on Close() for &DB{db:nil}, got %v", err)
	}
	if err := d.SaveMonitorJobDefinition(ctx, &monitor.MonitorJobDefinition{ID: "job1"}); err == nil {
		t.Fatal("expected error on SaveMonitorJobDefinition")
	}
	if err := d.UpdateMonitorJobLaunchIntent(ctx, "job1", true, monitor.JobStateRunning, now); err == nil {
		t.Fatal("expected error on UpdateMonitorJobLaunchIntent")
	}
	if err := d.DeleteMonitorJobDefinition(ctx, "job1"); err == nil {
		t.Fatal("expected error on DeleteMonitorJobDefinition")
	}
	if err := d.UpdateMonitorJobSamplingTier(ctx, "job1", monitor.SamplingTierRegular, now); err == nil {
		t.Fatal("expected error on UpdateMonitorJobSamplingTier")
	}
	if _, err := d.ListMonitorJobDefinitions(ctx); err == nil {
		t.Fatal("expected error on ListMonitorJobDefinitions")
	}
	if _, err := d.listMonitorJobNodes(ctx, "job1"); err == nil {
		t.Fatal("expected error on listMonitorJobNodes")
	}
	if err := d.SaveMonitorRun(ctx, &monitor.MonitorRun{RunID: "run1"}); err == nil {
		t.Fatal("expected error on SaveMonitorRun")
	}
	if err := d.UpdateMonitorRun(ctx, &monitor.MonitorRun{RunID: "run1"}); err == nil {
		t.Fatal("expected error on UpdateMonitorRun")
	}
	if err := d.MarkRunningMonitorRunsInterrupted(ctx); err == nil {
		t.Fatal("expected error on MarkRunningMonitorRunsInterrupted")
	}
	if err := d.SaveMonitorSamples(ctx, []*monitor.MonitorSample{{SampleID: "s1"}}); err == nil {
		t.Fatal("expected error on SaveMonitorSamples")
	}
	if err := d.SaveLatencyTest(ctx, &LatencyTest{AttemptID: "att1", ProfileID: "p1", NodeKey: "n1", TestProject: "proj1"}); err == nil {
		t.Fatal("expected error on SaveLatencyTest")
	}
	if _, err := d.QueryLatencyTests(ctx, LatencyTestFilter{ProfileID: "p1", NodeKey: "n1"}); err == nil {
		t.Fatal("expected error on QueryLatencyTests")
	}
	if _, err := d.GetLatencyTest(ctx, "att1"); err == nil {
		t.Fatal("expected error on GetLatencyTest")
	}
	if _, err := d.GetLatencyTestInWindow(ctx, "att1", nil, nil); err == nil {
		t.Fatal("expected error on GetLatencyTestInWindow")
	}
	if _, err := d.ListNodeHistoryRevisions(ctx, "p1", "id1"); err == nil {
		t.Fatal("expected error on ListNodeHistoryRevisions")
	}
	if _, err := d.QueryMonitorRuns(ctx, "job1", 10); err == nil {
		t.Fatal("expected error on QueryMonitorRuns")
	}
	if _, err := d.QueryMonitorSamples(ctx, monitor.SampleFilter{}); err == nil {
		t.Fatal("expected error on QueryMonitorSamples")
	}
	if _, err := d.GetNodeTimelineSamples(ctx, "n1", now); err == nil {
		t.Fatal("expected error on GetNodeTimelineSamples")
	}
	if _, err := d.QueryMonitorSamplesCursor(ctx, monitor.CursorFilter{}); err == nil {
		t.Fatal("expected error on QueryMonitorSamplesCursor")
	}
	if _, err := d.GetDerivedStats(ctx, monitor.StatsQuery{}); err == nil {
		t.Fatal("expected error on GetDerivedStats")
	}
	if _, err := d.GetMonitorSampleFacets(ctx, now, now, 10, 10); err == nil {
		t.Fatal("expected error on GetMonitorSampleFacets")
	}
	if _, _, err := d.distinctSampleValues(ctx, "target", now, now, 10); err == nil {
		t.Fatal("expected error on distinctSampleValues")
	}
	if _, err := d.ApplyRetention(ctx, monitor.RetentionRequest{Policy: monitor.Retention30d}); err == nil {
		t.Fatal("expected error on ApplyRetention")
	}
	if _, err := d.CleanupWorkbenchHistory(ctx, now); err == nil {
		t.Fatal("expected error on CleanupWorkbenchHistory")
	}

	// IP Reputation & Budget
	if _, err := d.LoadIPReputation(ctx, "key1", now); err == nil {
		t.Fatal("expected error on LoadIPReputation")
	}
	if err := d.SaveIPReputation(ctx, "key1", "{}", now); err == nil {
		t.Fatal("expected error on SaveIPReputation")
	}
	if _, err := d.CheckIPReputationBudget(ctx, now); err == nil {
		t.Fatal("expected error on CheckIPReputationBudget")
	}
	if _, err := d.ReserveIPReputation(ctx, now); err == nil {
		t.Fatal("expected error on ReserveIPReputation")
	}
	if err := d.CoolIPReputation(ctx, now); err == nil {
		t.Fatal("expected error on CoolIPReputation")
	}

	// Public service latest
	if _, err := d.LatestPublicServiceChecks(ctx, "p", "id", "rev"); err == nil {
		t.Fatal("expected error on LatestPublicServiceChecks")
	}

	// Measurement round
	if err := d.CreateMeasurementRound(ctx, MeasurementRound{RoundID: "r1", TriggerType: "manual", StartedAt: now, Items: []MeasurementRoundItem{{RequestID: "req1", Project: "download", ProfileID: "p1", NodeKey: "n1", NodeIdentityKey: "nid1", ConfigRevisionKey: "crk1"}}}); err == nil {
		t.Fatal("expected error on CreateMeasurementRound")
	}
	if err := d.FinishMeasurementRound(ctx, "r1", "finished"); err == nil {
		t.Fatal("expected error on FinishMeasurementRound")
	}
	if err := d.ReconcileMeasurementRounds(ctx); err == nil {
		t.Fatal("expected error on ReconcileMeasurementRounds")
	}
	if _, err := d.MeasurementRequestTrigger(ctx, "req1", "p1", "nid1", "rev1", "download", ""); err == nil {
		t.Fatal("expected error on MeasurementRequestTrigger")
	}
	if err := d.MarkMeasurementNotExecuted(ctx, "req1", "reason"); err == nil {
		t.Fatal("expected error on MarkMeasurementNotExecuted")
	}
	if _, err := d.QueryMeasurementRoundNodes(ctx, "p1", "n1", "nid1", "crk1", 10); err == nil {
		t.Fatal("expected error on QueryMeasurementRoundNodes")
	}
	if err := d.FinishManualMeasurementRound(ctx, "r1", "finished"); err == nil {
		t.Fatal("expected error on FinishManualMeasurementRound")
	}
	if err := d.FinishAutomaticMeasurementRound(ctx, "req1"); err == nil {
		t.Fatal("expected error on FinishAutomaticMeasurementRound")
	}

	// Latency batch
	if err := d.CreateLatencyBatch(ctx, &LatencyBatch{BatchID: "b1", RequestID: "r1", Items: []LatencyBatchItem{{ItemID: "i1"}}}); err == nil {
		t.Fatal("expected error on CreateLatencyBatch")
	}
	if err := d.UpdateLatencyBatchItem(ctx, &LatencyBatchItem{ItemID: "i1", BatchID: "b1"}); err == nil {
		t.Fatal("expected error on UpdateLatencyBatchItem")
	}
	if err := d.UpdateLatencyBatchState(ctx, "b1", "completed"); err == nil {
		t.Fatal("expected error on UpdateLatencyBatchState")
	}
	if _, err := d.GetLatencyBatch(ctx, "b1"); err == nil {
		t.Fatal("expected error on GetLatencyBatch")
	}
	if _, err := d.FindLatencyBatchByRequestID(ctx, "req1"); err == nil {
		t.Fatal("expected error on FindLatencyBatchByRequestID")
	}
	if _, err := d.ListLatencyBatches(ctx, 10); err == nil {
		t.Fatal("expected error on ListLatencyBatches")
	}
	if _, err := d.ListLatencyBatchesNeedingRecovery(ctx); err == nil {
		t.Fatal("expected error on ListLatencyBatchesNeedingRecovery")
	}

	// Public service attempt
	if err := d.CreatePublicServiceAttempt(ctx, &PublicServiceAttempt{AttemptID: "ps1"}); err == nil {
		t.Fatal("expected error on CreatePublicServiceAttempt")
	}
	if err := d.BeginPublicServiceAttempt(ctx, "ps1", now); err == nil {
		t.Fatal("expected error on BeginPublicServiceAttempt")
	}
	if _, err := d.RequestPublicServiceCancellation(ctx, "ps1"); err == nil {
		t.Fatal("expected error on RequestPublicServiceCancellation")
	}
	if err := d.StagePublicServiceResult(ctx, "ps1", "completed", PublicServiceMeasurement{}); err == nil {
		t.Fatal("expected error on StagePublicServiceResult")
	}
	if err := d.MarkPublicServiceStageFailed(ctx, "ps1", "failed", now, "err"); err == nil {
		t.Fatal("expected error on MarkPublicServiceStageFailed")
	}
	if err := d.MarkPublicServiceSaveFailed(ctx, "ps1", "err"); err == nil {
		t.Fatal("expected error on MarkPublicServiceSaveFailed")
	}
	if err := d.CommitPublicServiceResult(ctx, "ps1"); err == nil {
		t.Fatal("expected error on CommitPublicServiceResult")
	}
	if err := d.ReconcilePublicServiceAttempts(ctx); err == nil {
		t.Fatal("expected error on ReconcilePublicServiceAttempts")
	}
	if _, err := d.GetPublicServiceAttemptByRequestID(ctx, "req1"); err == nil {
		t.Fatal("expected error on GetPublicServiceAttemptByRequestID")
	}
	if _, err := d.GetPublicServiceAttempt(ctx, "ps1", PublicServiceFilter{}); err == nil {
		t.Fatal("expected error on GetPublicServiceAttempt")
	}
	if _, err := d.QueryPublicServiceAttempts(ctx, PublicServiceFilter{}); err == nil {
		t.Fatal("expected error on QueryPublicServiceAttempts")
	}

	// Workbench download attempt
	if err := d.CreateWorkbenchDownloadAttempt(ctx, &WorkbenchDownloadAttempt{AttemptID: "dl1"}); err == nil {
		t.Fatal("expected error on CreateWorkbenchDownloadAttempt")
	}
	if err := d.BeginWorkbenchDownloadAttempt(ctx, "dl1", now); err == nil {
		t.Fatal("expected error on BeginWorkbenchDownloadAttempt")
	}
	if _, err := d.RequestWorkbenchDownloadCancellation(ctx, "dl1"); err == nil {
		t.Fatal("expected error on RequestWorkbenchDownloadCancellation")
	}
	if err := d.StageWorkbenchDownloadResult(ctx, "dl1", "completed", WorkbenchDownloadMeasurement{}); err == nil {
		t.Fatal("expected error on StageWorkbenchDownloadResult")
	}
	if err := d.MarkWorkbenchDownloadStageFailed(ctx, "dl1", "failed", now, "err"); err == nil {
		t.Fatal("expected error on MarkWorkbenchDownloadStageFailed")
	}
	if err := d.MarkWorkbenchDownloadSaveFailed(ctx, "dl1", "err"); err == nil {
		t.Fatal("expected error on MarkWorkbenchDownloadSaveFailed")
	}
	if err := d.CommitWorkbenchDownloadResult(ctx, "dl1"); err == nil {
		t.Fatal("expected error on CommitWorkbenchDownloadResult")
	}
	if err := d.ReconcileWorkbenchDownloadAttempts(ctx); err == nil {
		t.Fatal("expected error on ReconcileWorkbenchDownloadAttempts")
	}
	if _, err := d.GetWorkbenchDownloadAttemptByRequestID(ctx, "req1"); err == nil {
		t.Fatal("expected error on GetWorkbenchDownloadAttemptByRequestID")
	}
	if _, err := d.GetWorkbenchDownloadAttempt(ctx, "dl1", WorkbenchDownloadFilter{}); err == nil {
		t.Fatal("expected error on GetWorkbenchDownloadAttempt")
	}
	if _, err := d.QueryWorkbenchDownloadAttempts(ctx, WorkbenchDownloadFilter{}); err == nil {
		t.Fatal("expected error on QueryWorkbenchDownloadAttempts")
	}
}
