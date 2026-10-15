package application

import (
	"context"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
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
}
