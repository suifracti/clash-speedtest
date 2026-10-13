package application

import (
	"context"
	"encoding/json"
	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/publicservice"
	"github.com/faceair/clash-speedtest/core/speedtester"
	"runtime"
	"testing"
	"time"
)

func TestLatencyPhysicalPathIsPreservedAndPreparationIsNotNodeFailure(t *testing.T) {
	var result speedtester.Result
	raw := `{"network_path":{"method":"physical_socket_v1","interface":"en1","tun_evidence":"packet_route_not_observed","failure_reason":"physical_dns_unavailable"},"latency_error_class":"physical_path","latency_error":"未执行：物理出口准备未完成"}`
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	record := buildWorkbenchLatencyRecord("fixture", "p", monitor.MonitoredNode{NodeKey: "n", NodeIdentityKey: "i", ConfigRevisionKey: "r"}, WorkbenchLatencyProject, at, at, at, &result)
	if len(record.NetworkPath) == 0 || record.Status != "not_executed" || record.TotalSamples != 0 || record.FailureSamples != 0 || record.PacketLoss != 0 {
		t.Fatalf("preparation incorrectly counted as failed node or path discarded: %+v", record)
	}
	var path speedtester.DownloadNetworkPath
	if err := json.Unmarshal(record.NetworkPath, &path); err != nil {
		t.Fatal(err)
	}
	if path.FailureReason != "physical_dns_unavailable" || path.SocketBindVerified || path.TUNEvidence != "packet_route_not_observed" {
		t.Fatalf("path inflated: %+v", path)
	}
}

func TestLatencyWindowProjectionExcludesLocalPhysicalFailureButKeepsTCPFailure(t *testing.T) {
	at := time.Now().UTC()
	record := history.LatencyTest{Status: "not_executed", Samples: []history.LatencyTestSample{{Seq: 1, Timestamp: at, Success: false, Error: "未执行：物理出口约束未核实"}}}
	got := projectLatencyTestForWindow(record)
	if got.TotalSamples != 0 || got.FailureSamples != 0 || got.PacketLoss != 0 || len(got.Samples) != 1 {
		t.Fatalf("local rejection counted as failed network sample or evidence erased: %+v", got)
	}
	record.Status = "failed"
	record.Samples[0].Error = "EOF"
	got = projectLatencyTestForWindow(record)
	if got.TotalSamples != 1 || got.FailureSamples != 1 || got.PacketLoss != 100 {
		t.Fatalf("genuine failure no longer counted: %+v", got)
	}
}

func TestWorkbenchPhysicalLatencyPathAndPreparationFailureSaveAndReopen(t *testing.T) {
	service, store, paths, options := newWorkbenchBatchFixture(t, 2)
	service.latencyMeasureHook = func(_ context.Context, node monitor.MonitoredNode, _ time.Duration) (*speedtester.Result, string, error) {
		result := knownWorkbenchLatencyResult(true)
		result.NetworkPath = &speedtester.DownloadNetworkPath{Method: "physical_socket_v1", Interface: "fixture-physical", AddressFamily: "ipv4", AddressSource: "physical_ipv4_dns", DNSMode: "physical_interface_dns_v1", DNSBindVerified: true, SocketBindVerified: true, TCPBindings: 1, TUNEvidence: "packet_route_not_observed"}
		if node.NodeIdentityKey == options[1].NodeIdentityKey {
			result.LatencySamples = nil
			result.Latency = 0
			result.LatencyErrorClass = "physical_path"
			result.LatencyError = "未执行：物理出口准备未完成"
			result.NetworkPath.SocketBindVerified = false
			result.NetworkPath.TCPBindings = 0
			result.NetworkPath.FailureReason = "physical_dns_unavailable"
		}
		return result, "https://speed.cloudflare.com/__down?bytes=1", nil
	}
	started, err := service.StartWorkbenchLatencyBatch(context.Background(), WorkbenchLatencyBatchRequest{RequestID: "physical-fixture", TestProject: WorkbenchLatencyProject, TargetID: "cloudflare", SampleCount: 1, TimeoutSeconds: 1, Selections: []WorkbenchLatencyBatchSelection{batchSelection(options[0]), batchSelection(options[1])}})
	if err != nil {
		t.Fatal(err)
	}
	waitWorkbenchLatencyBatch(t, service)
	batch, err := store.GetLatencyBatch(context.Background(), started.BatchID)
	if err != nil {
		t.Fatal(err)
	}
	for i, item := range batch.Items {
		if item.PersistenceState != "saved" || item.Result == nil || len(item.Result.NetworkPath) == 0 || item.Result.MethodVersion != 2 {
			t.Fatalf("path/save missing: %+v", item)
		}
		if i == 1 && (item.ExecutionState != "not_executed" || item.Result.FailureSamples != 0) {
			t.Fatalf("local failure blamed on node: %+v", item)
		}
	}
	dir := store.Dir()
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := history.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored := NewAppService(reopened, paths, NewMemoryEventEmitter())
	defer restored.Close()
	got, err := restored.GetWorkbenchLatencyBatch(context.Background(), started.BatchID)
	if err != nil || len(got.Items) != 2 {
		t.Fatalf("reopen: %v", err)
	}
	for i, item := range got.Items {
		if string(item.Result.NetworkPath) != string(batch.Items[i].Result.NetworkPath) || item.PersistenceState != "saved" {
			t.Fatal("reopen changed physical evidence")
		}
	}
}

func TestWorkbenchDefaultMacServiceLocalPreflightSavesZeroRequestsAndReopens(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mac physical path only")
	}
	// Empty config is rejected before interface discovery/DNS/HTTP. No real peer.
	node := monitor.MonitoredNode{NodeKey: "node-same", NodeIdentityKey: "i", ConfigRevisionKey: "r", DisplayName: "fixture"}
	app, store := newPublicServiceApplication(t, func(string, string) (monitor.MonitoredNode, error) { return node, nil }, nil)
	app.publicServiceChecker = publicservice.Checker{}
	started, err := app.StartWorkbenchPublicServiceTest(context.Background(), publicServiceRequest("physical-service-fixture", "p", "i", "r", "cloudflare_204"))
	if err != nil {
		t.Fatal(err)
	}
	app.publicServiceWG.Wait()
	got, err := app.GetWorkbenchPublicServiceAttempt(context.Background(), started.AttemptID, publicServiceQueryForTest(started))
	if err != nil || got.PersistenceState != "saved" || got.ExecutionState != "not_executed" || got.Result == nil || got.Result.RequestCount != 0 || got.Result.Details["network_path"] == "" {
		t.Fatalf("preflight persistence: %v %+v", err, got)
	}
	dir := store.Dir()
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := history.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	saved, err := reopened.GetPublicServiceAttemptByRequestID(context.Background(), "physical-service-fixture")
	if err != nil || saved.Result.Details["network_path"] != got.Result.Details["network_path"] || saved.Result.RequestCount != 0 {
		t.Fatalf("reopen path/zero request evidence changed: %v", err)
	}
}
