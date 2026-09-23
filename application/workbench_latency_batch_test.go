package application

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/profiles"
	"github.com/faceair/clash-speedtest/core/speedtester"
)

func TestWorkbenchLatencyBatchKeepsCrossProfileIdentityAndReopens(t *testing.T) {
	service, store, paths, options := newWorkbenchBatchFixture(t, 2)
	var measured atomic.Int32
	service.latencyMeasureHook = func(context.Context, monitor.MonitoredNode, time.Duration) (*speedtester.Result, string, error) {
		measured.Add(1)
		return knownWorkbenchLatencyResult(true), "https://probe.example/__down?bytes=1", nil
	}
	selections := []WorkbenchLatencyBatchSelection{batchSelection(options[0]), batchSelection(options[1]), batchSelection(options[0])}
	created, err := service.StartWorkbenchLatencyBatch(context.Background(), WorkbenchLatencyBatchRequest{RequestID: "same-request", TestProject: WorkbenchLatencyProject, TimeoutSeconds: 1, Selections: selections})
	if err != nil {
		t.Fatalf("start batch: %v", err)
	}
	waitWorkbenchLatencyBatch(t, service)
	batch, err := store.GetLatencyBatch(context.Background(), created.BatchID)
	if err != nil {
		t.Fatalf("get batch: %v", err)
	}
	if len(batch.Items) != 2 || measured.Load() != 2 {
		t.Fatalf("duplicate full identity must be measured once; items=%d measurements=%d", len(batch.Items), measured.Load())
	}
	if batch.Items[0].ProfileID == batch.Items[1].ProfileID || batch.Items[0].DisplayName != batch.Items[1].DisplayName {
		t.Fatalf("same-name cross-profile nodes were merged: %+v", batch.Items)
	}
	for _, item := range batch.Items {
		if item.ExecutionState != "completed" || item.PersistenceState != "saved" || item.Result == nil {
			t.Fatalf("unexpected persisted item: %+v", item)
		}
		if item.Result.AttemptID != item.AttemptID || item.Result.Source != workbenchLatencySourceBatch || item.Result.Method != workbenchLatencyMethod || item.Result.MethodVersion != 1 || item.Result.Unit != "ms" || item.Result.Target != "https://probe.example/__down?bytes=1" {
			t.Fatalf("attempt metadata or association mismatch: %+v", item)
		}
		if len(item.Result.Samples) != 1 || item.Result.Samples[0].LatencyMs != 42 {
			t.Fatalf("raw sample was not retained: %+v", item.Result.Samples)
		}
	}
	storeDir := store.Dir()
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	reopened, err := history.NewStore(storeDir)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	restarted := NewAppService(reopened, paths, NewMemoryEventEmitter())
	t.Cleanup(func() { _ = restarted.Close() })
	reopenedBatch, err := restarted.GetWorkbenchLatencyBatch(context.Background(), created.BatchID)
	if err != nil {
		t.Fatalf("reopen batch query: %v", err)
	}
	if len(reopenedBatch.Items) != 2 {
		t.Fatalf("reopened item count = %d", len(reopenedBatch.Items))
	}
	for _, item := range reopenedBatch.Items {
		if item.Result == nil || item.Result.ProfileID != item.ProfileID || item.Result.NodeKey != item.NodeKey || len(item.Result.Samples) != 1 {
			t.Fatalf("reopened item association mismatch: %+v", item)
		}
	}
}

func TestWorkbenchLatencyBatchPreservesPartialFailureAndRetriesSameAttempt(t *testing.T) {
	service, store, _, options := newWorkbenchBatchFixture(t, 3)
	var measured atomic.Int32
	service.latencyMeasureHook = func(_ context.Context, node monitor.MonitoredNode, _ time.Duration) (*speedtester.Result, string, error) {
		measured.Add(1)
		return knownWorkbenchLatencyResult(node.DisplayName != "失败节点"), "https://probe.example/__down?bytes=1", nil
	}
	// The test fixture names the third node explicitly; make the middle measurement fail.
	writeLatencyProxyCache(t, service.profilePaths, options[1].ProfileID, "失败节点", newLatencyProxy(t, 0))
	refreshed, err := service.ListMonitorNodeOptions()
	if err != nil {
		t.Fatalf("refresh fixture options: %v", err)
	}
	for i := range options {
		for _, current := range refreshed {
			if current.ProfileID == options[i].ProfileID {
				options[i] = current
			}
		}
	}
	stale := batchSelection(options[2])
	stale.ConfigRevisionKey = "rev_prior-selection"
	var failOnce atomic.Bool
	failOnce.Store(true)
	service.latencySaveHook = func(_ context.Context, record *history.LatencyTest) error {
		if record.ProfileID == options[0].ProfileID && failOnce.Swap(false) {
			return errors.New("injected history write failure")
		}
		return nil
	}
	created, err := service.StartWorkbenchLatencyBatch(context.Background(), WorkbenchLatencyBatchRequest{RequestID: "partial", TestProject: WorkbenchLatencyProject, TimeoutSeconds: 1, Selections: []WorkbenchLatencyBatchSelection{batchSelection(options[0]), batchSelection(options[1]), stale}})
	if err != nil {
		t.Fatalf("start batch: %v", err)
	}
	waitWorkbenchLatencyBatch(t, service)
	batch, err := store.GetLatencyBatch(context.Background(), created.BatchID)
	if err != nil {
		t.Fatalf("get batch: %v", err)
	}
	if batch.State != "completed_with_save_failures" {
		t.Fatalf("batch must expose partial persistence failure, state=%s", batch.State)
	}
	byProfile := map[string]history.LatencyBatchItem{}
	for _, item := range batch.Items {
		byProfile[item.ProfileID] = item
	}
	failedSave := byProfile[options[0].ProfileID]
	failedProbe := byProfile[options[1].ProfileID]
	skipped := byProfile[options[2].ProfileID]
	if failedSave.PersistenceState != "failed" || failedSave.Result == nil || !failedSave.ResultStaged || failedSave.AttemptID == "" {
		t.Fatalf("failed save did not retain staged result: %+v", failedSave)
	}
	if failedProbe.ExecutionState != "failed" || failedProbe.PersistenceState != "saved" || failedProbe.Result == nil || failedProbe.Result.FailureSamples != 1 {
		t.Fatalf("real probe failure was not saved as raw evidence: %+v", failedProbe)
	}
	if skipped.ExecutionState != "skipped_config" || skipped.PersistenceState != "not_applicable" || skipped.AttemptID != "" {
		t.Fatalf("stale revision should skip without attempt: %+v", skipped)
	}
	if measured.Load() != 2 {
		t.Fatalf("stale-revision item must not probe, measured %d items", measured.Load())
	}
	oldAttempt := failedSave.AttemptID
	if _, err := service.RetryWorkbenchLatencyBatchItem(context.Background(), batch.BatchID, failedSave.ItemID); err != nil {
		t.Fatalf("retry save: %v", err)
	}
	retried, err := store.GetLatencyBatch(context.Background(), batch.BatchID)
	if err != nil {
		t.Fatalf("get after retry: %v", err)
	}
	for _, item := range retried.Items {
		if item.ItemID == failedSave.ItemID && (item.AttemptID != oldAttempt || item.PersistenceState != "saved" || item.Result == nil) {
			t.Fatalf("retry changed attempt identity or failed to save: %+v", item)
		}
	}
	record, err := store.GetLatencyTest(context.Background(), oldAttempt)
	if err != nil || len(record.Samples) != 1 {
		t.Fatalf("retry should create exactly one raw sample: record=%+v err=%v", record, err)
	}
}

func TestWorkbenchLatencyBatchCancellationBoundsConcurrencyAndDeduplicatesRequest(t *testing.T) {
	service, store, _, options := newWorkbenchBatchFixture(t, 6)
	release := make(chan struct{})
	started := make(chan struct{}, 8)
	var active, maximum, measured atomic.Int32
	service.latencyMeasureHook = func(context.Context, monitor.MonitoredNode, time.Duration) (*speedtester.Result, string, error) {
		count := active.Add(1)
		for {
			old := maximum.Load()
			if count <= old || maximum.CompareAndSwap(old, count) {
				break
			}
		}
		measured.Add(1)
		started <- struct{}{}
		<-release
		active.Add(-1)
		return knownWorkbenchLatencyResult(true), "https://probe.example/__down?bytes=1", nil
	}
	selections := make([]WorkbenchLatencyBatchSelection, 0, len(options))
	for _, option := range options {
		selections = append(selections, batchSelection(option))
	}
	request := WorkbenchLatencyBatchRequest{RequestID: "cancel-once", TestProject: WorkbenchLatencyProject, TimeoutSeconds: 1, Selections: selections}
	created, err := service.StartWorkbenchLatencyBatch(context.Background(), request)
	if err != nil {
		t.Fatalf("start batch: %v", err)
	}
	for i := 0; i < workbenchLatencyBatchConcurrency; i++ {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("worker did not reach controlled measurement")
		}
	}
	duplicate, err := service.StartWorkbenchLatencyBatch(context.Background(), request)
	if err != nil || duplicate.BatchID != created.BatchID {
		t.Fatalf("duplicate request was not idempotent: duplicate=%+v err=%v", duplicate, err)
	}
	if _, err := service.RunWorkbenchLatencyTest(context.Background(), WorkbenchLatencyTestRequest{ProfileID: options[0].ProfileID, NodeKey: options[0].NodeKey, TestProject: WorkbenchLatencyProject, TimeoutSeconds: 1}); err == nil {
		t.Fatal("independent single-node entry bypassed active batch limit")
	}
	if _, err := service.TestSingle(SingleTestRequest{Config: TestConfig{Metrics: []string{"latency"}}}, ""); err == nil {
		t.Fatal("legacy single-node latency entry bypassed active batch limit")
	}
	if _, err := service.TestSingle(SingleTestRequest{Config: TestConfig{Metrics: []string{"unknown"}}}, ""); err == nil || !strings.Contains(err.Error(), "批量延迟操作正在运行") {
		t.Fatalf("empty parsed metrics must not fall back to latency during a batch: %v", err)
	}
	if _, err := service.TestSingle(SingleTestRequest{Config: TestConfig{Metrics: []string{"download"}}}, ""); err == nil || strings.Contains(err.Error(), "Workbench") {
		t.Fatalf("non-latency single metric was unexpectedly blocked by the batch: %v", err)
	}
	if maximum.Load() != workbenchLatencyBatchConcurrency {
		t.Fatalf("observed concurrency %d, want exactly %d", maximum.Load(), workbenchLatencyBatchConcurrency)
	}
	if _, err := service.CancelWorkbenchLatencyBatch(created.BatchID); err != nil {
		t.Fatalf("cancel batch: %v", err)
	}
	close(release)
	waitWorkbenchLatencyBatch(t, service)
	batch, err := store.GetLatencyBatch(context.Background(), created.BatchID)
	if err != nil {
		t.Fatalf("get batch: %v", err)
	}
	if measured.Load() != workbenchLatencyBatchConcurrency || batch.State != "cancelled" {
		t.Fatalf("cancel must stop pending dispatch: measured=%d state=%s", measured.Load(), batch.State)
	}
	completed, cancelled := 0, 0
	for _, item := range batch.Items {
		if item.ExecutionState == "completed" && item.PersistenceState == "saved" {
			completed++
		}
		if item.ExecutionState == "cancelled" && item.PersistenceState == "not_applicable" {
			cancelled++
		}
	}
	if completed != 4 || cancelled != 2 {
		t.Fatalf("completed results or pending cancellation were lost: completed=%d cancelled=%d items=%+v", completed, cancelled, batch.Items)
	}
}

func TestWorkbenchLatencyBatchShutdownStopsDispatch(t *testing.T) {
	service, store, _, options := newWorkbenchBatchFixture(t, 6)
	release := make(chan struct{})
	started := make(chan struct{}, 8)
	var measured atomic.Int32
	service.latencyMeasureHook = func(context.Context, monitor.MonitoredNode, time.Duration) (*speedtester.Result, string, error) {
		measured.Add(1)
		started <- struct{}{}
		<-release
		return knownWorkbenchLatencyResult(true), "https://probe.example/__down?bytes=1", nil
	}
	selections := make([]WorkbenchLatencyBatchSelection, 0, len(options))
	for _, option := range options {
		selections = append(selections, batchSelection(option))
	}
	created, err := service.StartWorkbenchLatencyBatch(context.Background(), WorkbenchLatencyBatchRequest{RequestID: "shutdown", TestProject: WorkbenchLatencyProject, TimeoutSeconds: 1, Selections: selections})
	if err != nil {
		t.Fatalf("start batch: %v", err)
	}
	for i := 0; i < workbenchLatencyBatchConcurrency; i++ {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("workers did not start")
		}
	}
	service.workbenchMu.Lock()
	runtime := service.workbenchActiveBatch
	service.workbenchMu.Unlock()
	closed := make(chan error, 1)
	go func() { closed <- service.Close() }()
	select {
	case <-runtime.ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("application shutdown did not cancel batch dispatch")
	}
	close(release)
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("close did not drain bounded batch work")
	}
	if measured.Load() != workbenchLatencyBatchConcurrency {
		t.Fatalf("shutdown dispatched pending items: measured=%d", measured.Load())
	}
	_ = created
	_ = store
}

func TestWorkbenchLatencyBatchCreationFailureSendsNoRequests(t *testing.T) {
	service, store, _, options := newWorkbenchBatchFixture(t, 1)
	var requests atomic.Int32
	service.latencyMeasureHook = func(context.Context, monitor.MonitoredNode, time.Duration) (*speedtester.Result, string, error) {
		requests.Add(1)
		return knownWorkbenchLatencyResult(true), "https://probe.example/__down?bytes=1", nil
	}
	raw, err := sql.Open("sqlite", filepath.Join(store.Dir(), "history.db"))
	if err != nil {
		t.Fatalf("open trigger connection: %v", err)
	}
	if _, err := raw.Exec(`CREATE TRIGGER reject_latency_batch BEFORE INSERT ON workbench_latency_batches BEGIN SELECT RAISE(FAIL, 'injected batch create failure'); END`); err != nil {
		_ = raw.Close()
		t.Fatalf("install create failure trigger: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close trigger connection: %v", err)
	}
	_, err = service.StartWorkbenchLatencyBatch(context.Background(), WorkbenchLatencyBatchRequest{RequestID: "cannot-persist", TestProject: WorkbenchLatencyProject, TimeoutSeconds: 1, Selections: []WorkbenchLatencyBatchSelection{batchSelection(options[0])}})
	if err == nil || !strings.Contains(err.Error(), "未发送网络请求") || requests.Load() != 0 {
		t.Fatalf("failed durable creation must send zero requests: err=%v requests=%d", err, requests.Load())
	}
}

func TestWorkbenchLatencyBatchReopenMarksPendingItemsWithoutRerunning(t *testing.T) {
	service, store, paths, options := newWorkbenchBatchFixture(t, 3)
	now := time.Now().UTC()
	batch := &history.LatencyBatch{BatchID: "crash-batch", RequestID: "crash-request", TestProject: WorkbenchLatencyProject, TimeoutSeconds: 1, RequestedAt: now, State: "running", Items: []history.LatencyBatchItem{
		{ItemID: "queued-item", ProfileID: options[0].ProfileID, NodeKey: options[0].NodeKey, NodeIdentityKey: options[0].NodeIdentityKey, ConfigRevisionKey: options[0].ConfigRevisionKey, DisplayName: options[0].DisplayName, NodeType: options[0].Type, ExecutionState: "queued", PersistenceState: "pending", RequestedAt: now},
		{ItemID: "uncommitted-item", ProfileID: options[1].ProfileID, NodeKey: options[1].NodeKey, NodeIdentityKey: options[1].NodeIdentityKey, ConfigRevisionKey: options[1].ConfigRevisionKey, DisplayName: options[1].DisplayName, NodeType: options[1].Type, ExecutionState: "running", PersistenceState: "saving", AttemptID: "attempt-interrupted", RequestedAt: now, StartedAt: now},
		{ItemID: "committed-item", ProfileID: options[2].ProfileID, NodeKey: options[2].NodeKey, NodeIdentityKey: options[2].NodeIdentityKey, ConfigRevisionKey: options[2].ConfigRevisionKey, DisplayName: options[2].DisplayName, NodeType: options[2].Type, ExecutionState: "running", PersistenceState: "saving", AttemptID: "attempt-committed", RequestedAt: now, StartedAt: now},
	}}
	if err := store.CreateLatencyBatch(context.Background(), batch); err != nil {
		t.Fatalf("create crash fixture: %v", err)
	}
	committedNode := monitor.MonitoredNode{NodeKey: options[2].NodeKey, NodeIdentityKey: options[2].NodeIdentityKey, ConfigRevisionKey: options[2].ConfigRevisionKey, DisplayName: options[2].DisplayName, Type: options[2].Type}
	record := buildWorkbenchLatencyRecord("attempt-committed", options[2].ProfileID, committedNode, WorkbenchLatencyProject, now, now, now, knownWorkbenchLatencyResult(true))
	record.Source = workbenchLatencySourceBatch
	record.Method = workbenchLatencyMethod
	record.MethodVersion = 1
	record.Target = "https://probe.example/__down?bytes=1"
	record.Unit = "ms"
	if err := store.SaveLatencyTest(context.Background(), record); err != nil {
		t.Fatalf("save committed attempt fixture: %v", err)
	}
	var requests atomic.Int32
	service.latencyMeasureHook = func(context.Context, monitor.MonitoredNode, time.Duration) (*speedtester.Result, string, error) {
		requests.Add(1)
		return knownWorkbenchLatencyResult(true), "https://probe.example/__down?bytes=1", nil
	}
	if err := service.reconcileWorkbenchLatencyBatches(); err != nil {
		t.Fatalf("reconcile interrupted batch: %v", err)
	}
	reopened, err := store.GetLatencyBatch(context.Background(), batch.BatchID)
	if err != nil {
		t.Fatalf("get reconciled batch: %v", err)
	}
	states := map[string]history.LatencyBatchItem{}
	for _, item := range reopened.Items {
		states[item.ItemID] = item
	}
	if reopened.State != "interrupted" || states["queued-item"].ExecutionState != "not_executed" || states["uncommitted-item"].ExecutionState != "interrupted" {
		t.Fatalf("unstarted work must stay stopped after reopen: batch=%+v", reopened)
	}
	committed := states["committed-item"]
	if committed.ExecutionState != "completed" || committed.PersistenceState != "saved" || committed.Result == nil || len(committed.Result.Samples) != 1 {
		t.Fatalf("committed raw attempt was not retained: %+v", committed)
	}
	if requests.Load() != 0 {
		t.Fatalf("reopening a batch must never run requests: %d", requests.Load())
	}
	_ = paths
}

func newWorkbenchBatchFixture(t *testing.T, count int) (*AppService, *history.Store, profiles.Paths, []MonitorNodeOptionDTO) {
	t.Helper()
	paths := profiles.Paths{Dir: t.TempDir()}
	airports := make([]*profiles.Airport, 0, count)
	for i := 0; i < count; i++ {
		profileID := fmt.Sprintf("profile-%02d", i)
		airports = append(airports, &profiles.Airport{ID: profileID, Name: fmt.Sprintf("订阅 %02d", i)})
		writeLatencyProxyCache(t, paths, profileID, "同名节点", newLatencyProxy(t, 0))
	}
	if err := profiles.SaveStore(paths.StoreFile(), &profiles.Store{Airports: airports}); err != nil {
		t.Fatalf("SaveStore: %v", err)
	}
	store, err := history.NewStore(filepath.Join(t.TempDir(), "history"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	service := &AppService{historyStore: store, profilePaths: paths, emitter: NewMemoryEventEmitter()}
	t.Cleanup(func() { _ = store.Close() })
	options, err := service.ListMonitorNodeOptions()
	if err != nil {
		_ = store.Close()
		t.Fatalf("ListMonitorNodeOptions: %v", err)
	}
	if len(options) != count {
		_ = store.Close()
		t.Fatalf("option count %d, want %d", len(options), count)
	}
	return service, store, paths, options
}

func batchSelection(option MonitorNodeOptionDTO) WorkbenchLatencyBatchSelection {
	return WorkbenchLatencyBatchSelection{ProfileID: option.ProfileID, NodeKey: option.NodeKey, NodeIdentityKey: option.NodeIdentityKey, ConfigRevisionKey: option.ConfigRevisionKey, DisplayName: option.DisplayName, NodeType: option.Type}
}

func knownWorkbenchLatencyResult(success bool) *speedtester.Result {
	now := time.Now().UTC()
	sample := speedtester.LatencySample{Seq: 1, Timestamp: now, LatencyMs: 42, Success: success}
	result := &speedtester.Result{Latency: 42 * time.Millisecond, Jitter: 3 * time.Millisecond, LatencySamples: []speedtester.LatencySample{sample}}
	if success {
		result.PacketLoss = 0
	} else {
		sample.Error = "controlled connection failure"
		result.LatencySamples[0] = sample
		result.PacketLoss = 100
		result.Latency = 0
	}
	return result
}

func waitWorkbenchLatencyBatch(t *testing.T, service *AppService) {
	t.Helper()
	done := make(chan struct{})
	go func() { service.workbenchWG.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("batch did not finish within bounded controlled work")
	}
}
