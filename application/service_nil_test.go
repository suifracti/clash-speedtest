package application

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/policy"
	"github.com/faceair/clash-speedtest/core/profiles"
)

func TestAppServiceNilReceiverSafety(t *testing.T) {
	var s *AppService = nil
	ctx := context.Background()
	now := time.Now()

	// 1. Basic lifecycle & accessors
	if em := s.Emitter(); em != nil {
		t.Fatalf("expected nil Emitter, got %v", em)
	}
	if hs := s.HistoryStore(); hs != nil {
		t.Fatalf("expected nil HistoryStore, got %v", hs)
	}
	st := s.Status()
	if st.IsRunning {
		t.Fatal("expected IsRunning to be false on nil service")
	}
	s.Stop() // Should not panic
	if err := s.Close(); err != nil {
		t.Fatalf("expected nil err on Close(), got %v", err)
	}

	// 2. Batch & Single test execution
	if err := s.StartBatch(BatchTestRequest{}, ""); err == nil {
		t.Fatal("expected error on StartBatch for nil service")
	}
	if _, err := s.TestSingle(SingleTestRequest{}, ""); err == nil {
		t.Fatal("expected error on TestSingle for nil service")
	}

	// 3. History operations
	if _, err := s.ListHistory(); err == nil {
		t.Fatal("expected error on ListHistory for nil service")
	}
	if _, err := s.GetHistory("id1"); err == nil {
		t.Fatal("expected error on GetHistory for nil service")
	}
	if err := s.DeleteHistory("id1"); err == nil {
		t.Fatal("expected error on DeleteHistory for nil service")
	}
	if _, err := s.GetAirportTimeline("a1"); err == nil {
		t.Fatal("expected error on GetAirportTimeline for nil service")
	}
	if _, err := s.CompareRuns("r1", "r2"); err == nil {
		t.Fatal("expected error on CompareRuns for nil service")
	}

	// 4. Monitor queries
	if _, err := s.QueryMonitorSamples(ctx, monitor.SampleFilter{}); err == nil {
		t.Fatal("expected error on QueryMonitorSamples for nil service")
	}
	if _, err := s.QueryMonitorRuns(ctx, "job1", 10); err == nil {
		t.Fatal("expected error on QueryMonitorRuns for nil service")
	}
	if _, err := s.GetNodeTimelineSamples(ctx, "node1", now); err == nil {
		t.Fatal("expected error on GetNodeTimelineSamples for nil service")
	}
	if _, err := s.QueryMonitorSamplesCursor(ctx, monitor.CursorFilter{}); err == nil {
		t.Fatal("expected error on QueryMonitorSamplesCursor for nil service")
	}
	if _, err := s.GetMonitorStats(ctx, monitor.StatsQuery{}); err == nil {
		t.Fatal("expected error on GetMonitorStats for nil service")
	}
	if _, err := s.ListNodeHistoryRevisions(ctx, "p1", "n1"); err == nil {
		t.Fatal("expected error on ListNodeHistoryRevisions for nil service")
	}
	if _, err := s.GetMonitorSampleFacets(ctx, nil, nil); err == nil {
		t.Fatal("expected error on GetMonitorSampleFacets for nil service")
	}
	if _, err := s.ApplyRetention(ctx, monitor.RetentionRequest{}); err == nil {
		t.Fatal("expected error on ApplyRetention for nil service")
	}
	if _, err := s.CleanupWorkbenchHistory(ctx, 30); err == nil {
		t.Fatal("expected error on CleanupWorkbenchHistory for nil service")
	}

	// 5. Workbench Latency
	if _, err := s.RunWorkbenchLatencyTest(ctx, WorkbenchLatencyTestRequest{}); err == nil {
		t.Fatal("expected error on RunWorkbenchLatencyTest for nil service")
	}
	if _, err := s.ListWorkbenchLatencyTests(ctx, WorkbenchLatencyHistoryQuery{ProfileID: "p1", NodeKey: "n1", NodeIdentityKey: "nid1", ConfigRevisionKey: "crk1"}); err == nil {
		t.Fatal("expected error on ListWorkbenchLatencyTests for nil service")
	}
	if _, err := s.GetWorkbenchLatencyTest(ctx, WorkbenchLatencyHistoryDetailQuery{ProfileID: "p1", NodeKey: "n1", NodeIdentityKey: "nid1", ConfigRevisionKey: "crk1", AttemptID: "att1"}); err == nil {
		t.Fatal("expected error on GetWorkbenchLatencyTest for nil service")
	}

	// 6. Workbench Latency Batch
	if _, err := s.StartWorkbenchLatencyBatch(ctx, WorkbenchLatencyBatchRequest{TargetID: "cloudflare"}); err == nil {
		t.Fatal("expected error on StartWorkbenchLatencyBatch for nil service")
	}
	if _, err := s.CancelWorkbenchLatencyBatch("b1"); err == nil {
		t.Fatal("expected error on CancelWorkbenchLatencyBatch for nil service")
	}
	if _, err := s.GetWorkbenchLatencyBatch(ctx, "b1"); err == nil {
		t.Fatal("expected error on GetWorkbenchLatencyBatch for nil service")
	}
	if _, err := s.ListWorkbenchLatencyBatches(ctx, 10); err == nil {
		t.Fatal("expected error on ListWorkbenchLatencyBatches for nil service")
	}
	if _, err := s.RetryWorkbenchLatencyBatchItem(ctx, "b1", "i1"); err == nil {
		t.Fatal("expected error on RetryWorkbenchLatencyBatchItem for nil service")
	}

	// 7. Workbench Public Service
	catalog := s.ListWorkbenchPublicServiceCatalog()
	if len(catalog) == 0 {
		t.Fatal("expected non-empty catalog on nil service")
	}
	if _, err := s.StartWorkbenchPublicServiceTest(ctx, WorkbenchPublicServiceTestRequest{}); err == nil {
		t.Fatal("expected error on StartWorkbenchPublicServiceTest for nil service")
	}
	if _, err := s.CancelWorkbenchPublicServiceTest(ctx, "ps1", WorkbenchPublicServiceHistoryQuery{ProfileID: "p1", NodeKey: "n1", NodeIdentityKey: "nid1", ConfigRevisionKey: "crk1", ServiceID: "youtube"}); err == nil {
		t.Fatal("expected error on CancelWorkbenchPublicServiceTest for nil service")
	}
	if _, err := s.RetrySaveWorkbenchPublicServiceTest(ctx, "ps1", WorkbenchPublicServiceHistoryQuery{ProfileID: "p1", NodeKey: "n1", NodeIdentityKey: "nid1", ConfigRevisionKey: "crk1", ServiceID: "youtube"}); err == nil {
		t.Fatal("expected error on RetrySaveWorkbenchPublicServiceTest for nil service")
	}
	if _, err := s.GetWorkbenchPublicServiceAttempt(ctx, "ps1", WorkbenchPublicServiceHistoryQuery{ProfileID: "p1", NodeKey: "n1", NodeIdentityKey: "nid1", ConfigRevisionKey: "crk1", ServiceID: "youtube"}); err == nil {
		t.Fatal("expected error on GetWorkbenchPublicServiceAttempt for nil service")
	}
	if _, err := s.ListWorkbenchPublicServiceTests(ctx, WorkbenchPublicServiceHistoryQuery{ProfileID: "p1", NodeKey: "n1", NodeIdentityKey: "nid1", ConfigRevisionKey: "crk1", ServiceID: "youtube"}); err == nil {
		t.Fatal("expected error on ListWorkbenchPublicServiceTests for nil service")
	}

	// 8. Workbench Download
	if _, err := s.StartWorkbenchDownloadTest(ctx, WorkbenchDownloadTestRequest{}); err == nil {
		t.Fatal("expected error on StartWorkbenchDownloadTest for nil service")
	}
	if _, err := s.CancelWorkbenchDownloadTest(ctx, "dl1", WorkbenchDownloadHistoryQuery{ProfileID: "p1", NodeKey: "n1", NodeIdentityKey: "nid1", ConfigRevisionKey: "crk1"}); err == nil {
		t.Fatal("expected error on CancelWorkbenchDownloadTest for nil service")
	}
	if _, err := s.RetrySaveWorkbenchDownloadTest(ctx, "dl1", WorkbenchDownloadHistoryQuery{ProfileID: "p1", NodeKey: "n1", NodeIdentityKey: "nid1", ConfigRevisionKey: "crk1"}); err == nil {
		t.Fatal("expected error on RetrySaveWorkbenchDownloadTest for nil service")
	}
	if _, err := s.GetWorkbenchDownloadAttempt(ctx, "dl1", WorkbenchDownloadHistoryQuery{ProfileID: "p1", NodeKey: "n1", NodeIdentityKey: "nid1", ConfigRevisionKey: "crk1"}); err == nil {
		t.Fatal("expected error on GetWorkbenchDownloadAttempt for nil service")
	}
	if _, err := s.ListWorkbenchDownloadTests(ctx, WorkbenchDownloadHistoryQuery{ProfileID: "p1", NodeKey: "n1", NodeIdentityKey: "nid1", ConfigRevisionKey: "crk1"}); err == nil {
		t.Fatal("expected error on ListWorkbenchDownloadTests for nil service")
	}

	// 9. Profile & Airport & Subscription
	if _, err := s.GetProfileSetup(); err == nil {
		t.Fatal("expected error on GetProfileSetup for nil service")
	}
	if err := s.InitializeEmptyProfileStore(); err == nil {
		t.Fatal("expected error on InitializeEmptyProfileStore for nil service")
	}
	if err := s.ImportProfileSource(ctx, "src"); err == nil {
		t.Fatal("expected error on ImportProfileSource for nil service")
	}
	if err := s.DiscardProfileImport(); err == nil {
		t.Fatal("expected error on DiscardProfileImport for nil service")
	}
	if _, err := s.ListAirports(); err == nil {
		t.Fatal("expected error on ListAirports for nil service")
	}
	if _, err := s.GetAirportURL("a1"); err == nil {
		t.Fatal("expected error on GetAirportURL for nil service")
	}
	if n := s.CountCachedNodes("a1"); n != 0 {
		t.Fatalf("expected 0 cached nodes for nil service, got %d", n)
	}
	if _, err := s.CreateAirport("n", "u", "ua"); err == nil {
		t.Fatal("expected error on CreateAirport for nil service")
	}
	if _, err := s.UpdateAirport("a1", "n", "u", "ua"); err == nil {
		t.Fatal("expected error on UpdateAirport for nil service")
	}
	if err := s.DeleteAirport("a1"); err == nil {
		t.Fatal("expected error on DeleteAirport for nil service")
	}
	if _, err := s.RefreshAirport("a1", "ua"); err == nil {
		t.Fatal("expected error on RefreshAirport for nil service")
	}
	if _, err := s.AddSubscription("a1", "s1", "u1", "note", "ua"); err == nil {
		t.Fatal("expected error on AddSubscription for nil service")
	}
	if _, err := s.UpdateSubscription("a1", "s1", "n1", "u1", "note", "ua"); err == nil {
		t.Fatal("expected error on UpdateSubscription for nil service")
	}
	if err := s.DeleteSubscription("a1", "s1"); err == nil {
		t.Fatal("expected error on DeleteSubscription for nil service")
	}
	if _, err := s.RefreshSubscription("a1", "s1", "ua"); err == nil {
		t.Fatal("expected error on RefreshSubscription for nil service")
	}
	if _, err := s.GetSubscriptionURL("s1"); err == nil {
		t.Fatal("expected error on GetSubscriptionURL for nil service")
	}
	if _, err := s.GetAirportNodes("a1"); err == nil {
		t.Fatal("expected error on GetAirportNodes for nil service")
	}
	if tok := s.GetAntigravityToken(); tok != "" {
		t.Fatalf("expected empty antigravity token for nil service, got %s", tok)
	}
	if src := s.GetAntigravitySource(); src != "" {
		t.Fatalf("expected empty antigravity source for nil service, got %s", src)
	}
	s.SetAntigravityToken("tok", "src") // Should not panic
	status := s.GetTokenStatus()
	if status.HasToken {
		t.Fatal("expected empty token status for nil service")
	}
	if _, err := s.ExportClashConfig("a1", nil); err == nil {
		t.Fatal("expected error on ExportClashConfig for nil service")
	}
	if _, err := s.ExportClashConfigFromResults("r1", nil); err == nil {
		t.Fatal("expected error on ExportClashConfigFromResults for nil service")
	}
	if _, err := s.GetSettings(); err == nil {
		t.Fatal("expected error on GetSettings for nil service")
	}
	if err := s.SaveSettings(&AppSettings{}); err == nil {
		t.Fatal("expected error on SaveSettings for nil service")
	}

	// 10. Controller
	s.SetController(nil, ControllerConfigDTO{}) // Should not panic
	if err := s.ConfigureController(ctx, ControllerConfigDTO{}); err == nil {
		t.Fatal("expected error on ConfigureController for nil service")
	}
	if _, err := s.GetControllerStatus(ctx); err == nil {
		t.Fatal("expected error on GetControllerStatus for nil service")
	}
	if _, err := s.ListControllerGroups(ctx); err == nil {
		t.Fatal("expected error on ListControllerGroups for nil service")
	}
	if err := s.SelectControllerNode(ctx, "grp", "node"); err == nil {
		t.Fatal("expected error on SelectControllerNode for nil service")
	}
	if _, err := s.GetSwitchPolicy(ctx); err == nil {
		t.Fatal("expected error on GetSwitchPolicy for nil service")
	}
	if err := s.UpdateSwitchPolicy(ctx, policy.SwitchPolicy{}); err == nil {
		t.Fatal("expected error on UpdateSwitchPolicy for nil service")
	}
	if _, err := s.GetSwitchAuditTrail(ctx); err == nil {
		t.Fatal("expected error on GetSwitchAuditTrail for nil service")
	}
	if _, err := s.EvaluateAndAutoSwitch(ctx, nil); err == nil {
		t.Fatal("expected error on EvaluateAndAutoSwitch for nil service")
	}

	// 11. 24/7 Monitor
	s.SetMonitorRunner(nil) // Should not panic
	if _, err := s.CreateMonitorJob(monitor.MonitorJob{}); err == nil {
		t.Fatal("expected error on CreateMonitorJob for nil service")
	}
	if err := s.StartMonitorJob("j1"); err == nil {
		t.Fatal("expected error on StartMonitorJob for nil service")
	}
	if err := s.PauseMonitorJob("j1"); err == nil {
		t.Fatal("expected error on PauseMonitorJob for nil service")
	}
	if err := s.ResumeMonitorJob("j1"); err == nil {
		t.Fatal("expected error on ResumeMonitorJob for nil service")
	}
	if err := s.StopMonitorJob("j1"); err == nil {
		t.Fatal("expected error on StopMonitorJob for nil service")
	}
	if err := s.SetMonitorJobResumeOnLaunch("j1", true); err == nil {
		t.Fatal("expected error on SetMonitorJobResumeOnLaunch for nil service")
	}
	if err := s.DeleteMonitorJob("j1"); err == nil {
		t.Fatal("expected error on DeleteMonitorJob for nil service")
	}
	s.StopAllMonitorJobs() // Should not panic
	if _, err := s.GetMonitorJob("j1"); err == nil {
		t.Fatal("expected error on GetMonitorJob for nil service")
	}
	if err := s.UpdateMonitorJobSamplingTier("j1", monitor.SamplingTierRegular); err == nil {
		t.Fatal("expected error on UpdateMonitorJobSamplingTier for nil service")
	}
	if mJobs := s.ListMonitorJobs(); len(mJobs) != 0 {
		t.Fatalf("expected empty monitor jobs for nil service, got %v", mJobs)
	}
	if _, err := s.TriggerMonitorJob("j1"); err == nil {
		t.Fatal("expected error on TriggerMonitorJob for nil service")
	}

	// 12. Backup, Migration & Maintenance
	var buf bytes.Buffer
	if err := s.ExportDataBackup(ctx, &buf); err == nil {
		t.Fatal("expected error on ExportDataBackup for nil service")
	}
	if _, err := s.GetDataMigration(); err == nil {
		t.Fatal("expected error on GetDataMigration for nil service")
	}
	if err := s.MigrateLegacyData(); err == nil {
		t.Fatal("expected error on MigrateLegacyData for nil service")
	}
	if err := s.SaveAirportMaintenance("a1", profiles.AirportMaintenance{}); err == nil {
		t.Fatal("expected error on SaveAirportMaintenance for nil service")
	}
	s.StartSubscriptionRefresh("ua") // Should not panic

	// 13. Periodic Sampling & Subscription Refresh
	if _, err := s.GetPeriodicSampling(); err == nil {
		t.Fatal("expected error on GetPeriodicSampling for nil service")
	}
	if _, err := s.ConfigurePeriodicSampling(PeriodicSamplingConfig{}); err == nil {
		t.Fatal("expected error on ConfigurePeriodicSampling for nil service")
	}
	if _, err := s.GetPeriodicServiceChecks(ctx, "p1", "n1"); err == nil {
		t.Fatal("expected error on GetPeriodicServiceChecks for nil service")
	}
	if _, err := s.RunPeriodicSamplingSample(ctx, PeriodicSampleRequest{}); err == nil {
		t.Fatal("expected error on RunPeriodicSamplingSample for nil service")
	}
	if _, err := s.PausePeriodicMeasurementsForSample(); err == nil {
		t.Fatal("expected error on PausePeriodicMeasurementsForSample for nil service")
	}
	if _, err := s.ListSubscriptionRefreshJobs(); err == nil {
		t.Fatal("expected error on ListSubscriptionRefreshJobs for nil service")
	}
	if _, err := s.GetSubscriptionRefresh("a1"); err == nil {
		t.Fatal("expected error on GetSubscriptionRefresh for nil service")
	}
	if _, err := s.StartSubscriptionRefreshJob(SubscriptionRefreshRequest{}, "ua"); err == nil {
		t.Fatal("expected error on StartSubscriptionRefreshJob for nil service")
	}
	if _, err := s.CancelSubscriptionRefresh("a1"); err == nil {
		t.Fatal("expected error on CancelSubscriptionRefresh for nil service")
	}
	if _, err := s.GetSubscriptionUsage(ctx, "", "", "", ""); err == nil {
		t.Fatal("expected error on GetSubscriptionUsage for nil service")
	}
	if _, err := s.UpdateSubscriptionUsage(ctx, "ua"); err == nil {
		t.Fatal("expected error on UpdateSubscriptionUsage for nil service")
	}

	// 14. Measurement Rounds & Monitor Budget/Storage/Scope/Recovery
	if err := s.CreateManualMeasurementRound(ctx, history.MeasurementRound{}); err == nil {
		t.Fatal("expected error on CreateManualMeasurementRound for nil service")
	}
	if _, err := s.ListMeasurementRoundNodes(ctx, "p1", "n1", "nid1", "rev1", 10); err == nil {
		t.Fatal("expected error on ListMeasurementRoundNodes for nil service")
	}
	if err := s.FinishManualMeasurementRound(ctx, "r1", "state"); err == nil {
		t.Fatal("expected error on FinishManualMeasurementRound for nil service")
	}
	if _, err := s.GetMonitorBudgetStatus(ctx); err == nil {
		t.Fatal("expected error on GetMonitorBudgetStatus for nil service")
	}
	if _, err := s.GetMonitorRecommendation(ctx, MonitorRecommendationRequest{}); err == nil {
		t.Fatal("expected error on GetMonitorRecommendation for nil service")
	}
	if _, err := s.ListMonitorNodeOptions(); err == nil {
		t.Fatal("expected error on ListMonitorNodeOptions for nil service")
	}
	if _, err := s.CreateMonitorJobFromRequest(MonitorJobCreateRequest{}); err == nil {
		t.Fatal("expected error on CreateMonitorJobFromRequest for nil service")
	}
	if _, err := s.ListMonitorJobDTOs(); err == nil {
		t.Fatal("expected error on ListMonitorJobDTOs for nil service")
	}
	if _, err := s.GetMonitorJobDTO("j1"); err == nil {
		t.Fatal("expected error on GetMonitorJobDTO for nil service")
	}
	if err := s.RecoverMonitorJobs(ctx); err == nil {
		t.Fatal("expected error on RecoverMonitorJobs for nil service")
	}
	if _, err := s.GetMonitorStorageUsage(); err == nil {
		t.Fatal("expected error on GetMonitorStorageUsage for nil service")
	}
	if _, err := s.PreviewMonitorRetention(ctx, monitor.RetentionRequest{}); err == nil {
		t.Fatal("expected error on PreviewMonitorRetention for nil service")
	}
}
