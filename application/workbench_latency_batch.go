package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/speedtester"
)

const (
	workbenchLatencyBatchConcurrency = 4
	workbenchLatencyBatchMaxItems    = 200
	workbenchLatencySourceBatch      = "workbench_batch_latency"
	workbenchLatencyMethod           = "http_get_via_proxy_first_byte"
)

type workbenchLatencyBatchRuntime struct {
	batchID         string
	ctx             context.Context
	cancel          context.CancelFunc
	mu              sync.Mutex
	cancelRequested bool
	shutdown        bool
}

func (s *AppService) beginWorkbenchSingle() error {
	s.workbenchMu.Lock()
	defer s.workbenchMu.Unlock()
	if s.workbenchClosed {
		return fmt.Errorf("应用正在关闭，不能启动延迟测试")
	}
	if s.workbenchActiveBatch != nil {
		return fmt.Errorf("批量延迟操作正在运行，单节点延迟操作暂不可启动")
	}
	s.workbenchActiveSingles++
	s.workbenchWG.Add(1)
	return nil
}

func (s *AppService) endWorkbenchSingle() {
	s.workbenchMu.Lock()
	if s.workbenchActiveSingles > 0 {
		s.workbenchActiveSingles--
	}
	s.workbenchMu.Unlock()
	s.workbenchWG.Done()
}

func (s *AppService) StartWorkbenchLatencyBatch(_ context.Context, req WorkbenchLatencyBatchRequest) (*WorkbenchLatencyBatchDTO, error) {
	if s.historyStore == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	requestID := strings.TrimSpace(req.RequestID)
	if requestID == "" {
		return nil, monitor.NewValidationError("request_id 不能为空")
	}
	if strings.TrimSpace(req.TestProject) != WorkbenchLatencyProject {
		return nil, monitor.NewValidationError("test_project 必须是 latency_stability")
	}
	timeoutSeconds := req.TimeoutSeconds
	if timeoutSeconds == 0 {
		timeoutSeconds = 5
	}
	if timeoutSeconds < minWorkbenchLatencyTimeoutSeconds || timeoutSeconds > maxWorkbenchLatencyTimeoutSeconds {
		return nil, monitor.NewValidationError(fmt.Sprintf("timeout_seconds 必须在 %d 到 %d 之间", minWorkbenchLatencyTimeoutSeconds, maxWorkbenchLatencyTimeoutSeconds))
	}
	if len(req.Selections) == 0 || len(req.Selections) > workbenchLatencyBatchMaxItems {
		return nil, monitor.NewValidationError(fmt.Sprintf("批量节点数必须在 1 到 %d 之间", workbenchLatencyBatchMaxItems))
	}
	selections := make([]WorkbenchLatencyBatchSelection, 0, len(req.Selections))
	seen := make(map[[4]string]struct{}, len(req.Selections))
	for _, item := range req.Selections {
		item.ProfileID = strings.TrimSpace(item.ProfileID)
		item.NodeKey = strings.TrimSpace(item.NodeKey)
		item.NodeIdentityKey = strings.TrimSpace(item.NodeIdentityKey)
		item.ConfigRevisionKey = strings.TrimSpace(item.ConfigRevisionKey)
		if item.ProfileID == "" || item.NodeKey == "" || item.NodeIdentityKey == "" || item.ConfigRevisionKey == "" {
			return nil, monitor.NewValidationError("每个节点都必须包含订阅、稳定节点身份和配置 revision")
		}
		key := [4]string{item.ProfileID, item.NodeKey, item.NodeIdentityKey, item.ConfigRevisionKey}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		selections = append(selections, item)
	}
	if len(selections) == 0 {
		return nil, monitor.NewValidationError("没有可执行的去重节点")
	}
	requestedAt := time.Now().UTC()
	batch := &history.LatencyBatch{BatchID: newWorkbenchLatencyBatchID(), RequestID: requestID, TestProject: WorkbenchLatencyProject, TimeoutSeconds: timeoutSeconds, RequestedAt: requestedAt, State: "queued", ItemCount: len(selections), Items: make([]history.LatencyBatchItem, 0, len(selections))}
	for i, selection := range selections {
		batch.Items = append(batch.Items, history.LatencyBatchItem{
			ItemID: newWorkbenchLatencyBatchItemID(), BatchID: batch.BatchID, Ordinal: i,
			ProfileID: selection.ProfileID, NodeKey: selection.NodeKey,
			NodeIdentityKey: selection.NodeIdentityKey, ConfigRevisionKey: selection.ConfigRevisionKey,
			DisplayName: selection.DisplayName, NodeType: selection.NodeType,
			ExecutionState: "queued", PersistenceState: "pending", RequestedAt: requestedAt,
		})
	}

	s.workbenchMu.Lock()
	if s.workbenchClosed {
		s.workbenchMu.Unlock()
		return nil, fmt.Errorf("应用正在关闭")
	}
	if prior, err := s.historyStore.FindLatencyBatchByRequestID(context.Background(), requestID); err != nil {
		s.workbenchMu.Unlock()
		return nil, fmt.Errorf("检查重复批次请求: %w", err)
	} else if prior != nil {
		s.workbenchMu.Unlock()
		return workbenchLatencyBatchDTO(*prior), nil
	}
	if s.workbenchActiveBatch != nil || s.workbenchActiveSingles > 0 {
		s.workbenchMu.Unlock()
		return nil, fmt.Errorf("已有 Workbench 延迟测试正在运行")
	}
	runCtx, cancel := context.WithCancel(context.Background())
	runtime := &workbenchLatencyBatchRuntime{batchID: batch.BatchID, ctx: runCtx, cancel: cancel}
	s.workbenchActiveBatch = runtime
	s.workbenchWG.Add(1)
	// The full frozen selection is committed before the worker can issue a request.
	if err := s.historyStore.CreateLatencyBatch(context.Background(), batch); err != nil {
		s.workbenchActiveBatch = nil
		s.workbenchMu.Unlock()
		cancel()
		s.workbenchWG.Done()
		return nil, fmt.Errorf("保存批量延迟选择失败，未发送网络请求: %w", err)
	}
	s.workbenchMu.Unlock()
	response := workbenchLatencyBatchDTO(*batch)
	s.emitWorkbenchLatencyBatch(batch)
	go s.runWorkbenchLatencyBatch(runtime, batch)
	return response, nil
}

func (s *AppService) CancelWorkbenchLatencyBatch(batchID string) (*WorkbenchLatencyBatchDTO, error) {
	batchID = strings.TrimSpace(batchID)
	s.workbenchMu.Lock()
	runtime := s.workbenchActiveBatch
	if runtime == nil || runtime.batchID != batchID {
		s.workbenchMu.Unlock()
		return nil, fmt.Errorf("指定批次当前没有可取消的执行")
	}
	runtime.mu.Lock()
	runtime.cancelRequested = true
	runtime.cancel()
	stateErr := s.historyStore.UpdateLatencyBatchState(context.Background(), batchID, "cancelling")
	runtime.mu.Unlock()
	s.workbenchMu.Unlock()
	if stateErr != nil {
		return nil, stateErr
	}
	batch, err := s.historyStore.GetLatencyBatch(context.Background(), batchID)
	if err != nil {
		return nil, err
	}
	s.emitWorkbenchLatencyBatch(batch)
	return workbenchLatencyBatchDTO(*batch), nil
}

func (s *AppService) GetWorkbenchLatencyBatch(ctx context.Context, batchID string) (*WorkbenchLatencyBatchDTO, error) {
	if s.historyStore == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	batch, err := s.historyStore.GetLatencyBatch(ctx, strings.TrimSpace(batchID))
	if err != nil {
		return nil, err
	}
	return workbenchLatencyBatchDTO(*batch), nil
}

func (s *AppService) ListWorkbenchLatencyBatches(ctx context.Context, limit int) ([]WorkbenchLatencyBatchDTO, error) {
	if s.historyStore == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	batches, err := s.historyStore.ListLatencyBatches(ctx, limit)
	if err != nil {
		return nil, err
	}
	result := make([]WorkbenchLatencyBatchDTO, 0, len(batches))
	for _, batch := range batches {
		result = append(result, *workbenchLatencyBatchDTO(batch))
	}
	return result, nil
}

func (s *AppService) RetryWorkbenchLatencyBatchItem(ctx context.Context, batchID, itemID string) (*WorkbenchLatencyBatchDTO, error) {
	s.workbenchRetryMu.Lock()
	defer s.workbenchRetryMu.Unlock()
	if err := s.beginWorkbenchSingle(); err != nil {
		return nil, err
	}
	defer s.endWorkbenchSingle()

	batch, err := s.historyStore.GetLatencyBatch(ctx, batchID)
	if err != nil {
		return nil, err
	}
	for i := range batch.Items {
		item := &batch.Items[i]
		if item.ItemID != itemID {
			continue
		}
		if item.Result == nil || item.AttemptID == "" {
			return nil, fmt.Errorf("该项没有可重试保存的测量结果")
		}
		if item.PersistenceState != "failed" {
			return nil, fmt.Errorf("该项保存状态为 %s，只有保存失败的结果可重试", item.PersistenceState)
		}
		if err := s.saveWorkbenchLatencyBatchAttempt(ctx, item.Result); err != nil {
			item.PersistenceState = "failed"
			item.PersistenceError = err.Error()
			item.ResultStaged = true
			_ = s.historyStore.UpdateLatencyBatchItem(ctx, item)
			return workbenchLatencyBatchDTO(*batch), err
		}
		item.PersistenceState = "saved"
		item.PersistenceError = ""
		item.ResultStaged = false
		if err := s.historyStore.UpdateLatencyBatchItem(ctx, item); err != nil {
			return nil, err
		}
		batch.State = deriveLatencyBatchState(batch.Items, false, false)
		if err := s.historyStore.UpdateLatencyBatchState(ctx, batchID, batch.State); err != nil {
			return nil, err
		}
		return s.GetWorkbenchLatencyBatch(ctx, batchID)
	}
	return nil, fmt.Errorf("latency batch item %s not found", itemID)
}

func (s *AppService) runWorkbenchLatencyBatch(runtime *workbenchLatencyBatchRuntime, batch *history.LatencyBatch) {
	defer s.workbenchWG.Done()
	var stateMu sync.Mutex
	update := func(index int, mutate func(*history.LatencyBatchItem)) error {
		stateMu.Lock()
		defer stateMu.Unlock()
		item := &batch.Items[index]
		mutate(item)
		if err := s.historyStore.UpdateLatencyBatchItem(context.Background(), item); err != nil {
			item.PersistenceState = "failed"
			item.PersistenceError = err.Error()
			if item.Result != nil {
				item.ResultStaged = true
			}
			runtime.mu.Lock()
			cancelling, shutdown := runtime.cancelRequested, runtime.shutdown
			runtime.mu.Unlock()
			batch.State = deriveLatencyBatchState(batch.Items, cancelling, shutdown)
			s.emitWorkbenchLatencyBatch(batch)
			return err
		}
		runtime.mu.Lock()
		batch.State = deriveLatencyBatchState(batch.Items, runtime.cancelRequested, runtime.shutdown)
		err := s.historyStore.UpdateLatencyBatchState(context.Background(), batch.BatchID, batch.State)
		runtime.mu.Unlock()
		if err != nil {
			return err
		}
		s.emitWorkbenchLatencyBatch(batch)
		return nil
	}
	stateMu.Lock()
	runtime.mu.Lock()
	batch.State = deriveLatencyBatchState(batch.Items, runtime.cancelRequested, runtime.shutdown)
	_ = s.historyStore.UpdateLatencyBatchState(context.Background(), batch.BatchID, batch.State)
	runtime.mu.Unlock()
	stateMu.Unlock()
	s.emitWorkbenchLatencyBatch(batch)

	workerCount := len(batch.Items)
	if workerCount > workbenchLatencyBatchConcurrency {
		workerCount = workbenchLatencyBatchConcurrency
	}
	jobs := make(chan int)
	var workers sync.WaitGroup
	for n := 0; n < workerCount; n++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				runtime.mu.Lock()
				cancelled := runtime.cancelRequested || runtime.shutdown
				runtime.mu.Unlock()
				if cancelled {
					continue
				}
				s.runWorkbenchLatencyBatchItem(runtime, batch, index, update)
			}
		}()
	}
	for index := range batch.Items {
		select {
		case <-runtime.ctx.Done():
			goto dispatchDone
		case jobs <- index:
		}
	}
dispatchDone:
	close(jobs)
	workers.Wait()
	runtime.mu.Lock()
	cancelling, shutdown := runtime.cancelRequested, runtime.shutdown
	runtime.mu.Unlock()
	for i := range batch.Items {
		if batch.Items[i].ExecutionState != "queued" {
			continue
		}
		state := "cancelled"
		if shutdown {
			state = "not_executed"
		} else if !cancelling {
			state = "not_executed"
		}
		_ = update(i, func(item *history.LatencyBatchItem) {
			item.ExecutionState = state
			item.PersistenceState = "not_applicable"
			item.FinishedAt = time.Now().UTC()
		})
	}
	stateMu.Lock()
	runtime.mu.Lock()
	batch.State = deriveLatencyBatchState(batch.Items, runtime.cancelRequested, runtime.shutdown)
	_ = s.historyStore.UpdateLatencyBatchState(context.Background(), batch.BatchID, batch.State)
	runtime.mu.Unlock()
	stateMu.Unlock()
	s.emitWorkbenchLatencyBatch(batch)
	runtime.cancel()
	s.workbenchMu.Lock()
	if s.workbenchActiveBatch == runtime {
		s.workbenchActiveBatch = nil
	}
	s.workbenchMu.Unlock()
}

func (s *AppService) runWorkbenchLatencyBatchItem(runtime *workbenchLatencyBatchRuntime, batch *history.LatencyBatch, index int, update func(int, func(*history.LatencyBatchItem)) error) {
	runtime.mu.Lock()
	if runtime.cancelRequested || runtime.shutdown {
		runtime.mu.Unlock()
		return
	}
	runtime.mu.Unlock()
	if err := update(index, func(item *history.LatencyBatchItem) {
		item.ExecutionState = "running"
		item.StartedAt = time.Now().UTC()
	}); err != nil {
		return
	}
	snapshot := batch.Items[index]
	selected, executionName, proxy, err := s.resolveWorkbenchLatencyProxy(snapshot.ProfileID, snapshot.NodeKey)
	if err != nil {
		_ = update(index, func(item *history.LatencyBatchItem) {
			item.ExecutionState = "skipped_config"
			item.PersistenceState = "not_applicable"
			item.ErrorMessage = err.Error()
			item.FinishedAt = time.Now().UTC()
		})
		return
	}
	if selected.NodeIdentityKey != snapshot.NodeIdentityKey || selected.ConfigRevisionKey != snapshot.ConfigRevisionKey {
		_ = update(index, func(item *history.LatencyBatchItem) {
			item.ExecutionState = "skipped_config"
			item.PersistenceState = "not_applicable"
			item.ErrorMessage = "节点身份或配置 revision 已变化，请重新选择"
			item.FinishedAt = time.Now().UTC()
		})
		return
	}
	runtime.mu.Lock()
	if runtime.cancelRequested || runtime.shutdown {
		runtime.mu.Unlock()
		_ = update(index, func(item *history.LatencyBatchItem) {
			item.ExecutionState = "cancelled"
			item.PersistenceState = "not_applicable"
			item.FinishedAt = time.Now().UTC()
		})
		return
	}
	runtime.mu.Unlock()
	requestedAt := snapshot.RequestedAt
	if requestedAt.IsZero() {
		requestedAt = batch.RequestedAt
	}
	startedAt := time.Now().UTC()
	attemptID := newWorkbenchLatencyAttemptID()
	if err := update(index, func(item *history.LatencyBatchItem) {
		item.AttemptID = attemptID
		item.StartedAt = startedAt
		item.PersistenceState = "saving"
	}); err != nil {
		return
	}
	var measured *speedtester.Result
	target := ""
	timeout := time.Duration(batch.TimeoutSeconds) * time.Second
	if s.latencyMeasureHook != nil {
		measured, target, err = s.latencyMeasureHook(context.Background(), selected, timeout)
	} else {
		st, createErr := speedtester.New(&speedtester.Config{ConfigPaths: s.profilePaths.CacheFile(snapshot.ProfileID), Mode: speedtester.SpeedModeFast, Metrics: speedtester.MetricSet{Latency: true}, Concurrent: 1, Timeout: timeout, Rounds: 1})
		if createErr != nil {
			err = createErr
		} else {
			target = st.LatencyProbeTarget()
			measured = st.TestSingle(executionName, proxy, func(*speedtester.Result) bool { return true })
		}
	}
	finishedAt := time.Now().UTC()
	if err != nil || measured == nil {
		message := "延迟测试未产生结果"
		if err != nil {
			message = err.Error()
		}
		_ = update(index, func(item *history.LatencyBatchItem) {
			item.ExecutionState = "failed"
			item.PersistenceState = "not_applicable"
			item.ErrorMessage = message
			item.FinishedAt = finishedAt
		})
		return
	}
	record := buildWorkbenchLatencyRecord(attemptID, snapshot.ProfileID, selected, batch.TestProject, requestedAt, startedAt, finishedAt, measured)
	record.Source = workbenchLatencySourceBatch
	record.Method = workbenchLatencyMethod
	record.MethodVersion = 1
	record.Target = target
	record.Unit = "ms"
	executionState := "completed"
	if record.SuccessSamples == 0 {
		executionState = "failed"
	}
	if err := update(index, func(item *history.LatencyBatchItem) {
		item.ExecutionState = executionState
		item.PersistenceState = "saving"
		item.FinishedAt = finishedAt
		item.ErrorMessage = record.ErrorMessage
		item.Result = record
		item.ResultStaged = true
	}); err != nil {
		return
	}
	saveCtx, cancelSave := context.WithTimeout(context.Background(), workbenchLatencyPersistenceTimeout)
	saveErr := s.saveWorkbenchLatencyBatchAttempt(saveCtx, record)
	cancelSave()
	if saveErr != nil {
		_ = update(index, func(item *history.LatencyBatchItem) {
			item.PersistenceState = "failed"
			item.PersistenceError = saveErr.Error()
			item.Result = record
			item.ResultStaged = true
		})
		return
	}
	_ = update(index, func(item *history.LatencyBatchItem) {
		item.PersistenceState = "saved"
		item.PersistenceError = ""
		item.Result = record
		item.ResultStaged = false
	})
}

func (s *AppService) saveWorkbenchLatencyBatchAttempt(ctx context.Context, record *history.LatencyTest) error {
	if s.latencySaveHook != nil {
		if err := s.latencySaveHook(ctx, record); err != nil {
			return err
		}
	}
	if s.historyStore == nil {
		return fmt.Errorf("history store is not initialized")
	}
	return s.historyStore.SaveLatencyTest(ctx, record)
}

func (s *AppService) reconcileWorkbenchLatencyBatches() error {
	if s.historyStore == nil {
		return nil
	}
	batches, err := s.historyStore.ListLatencyBatches(context.Background(), 100)
	if err != nil {
		return err
	}
	for _, summary := range batches {
		if summary.State != "queued" && summary.State != "running" && summary.State != "cancelling" {
			continue
		}
		userCancelled := summary.State == "cancelling"
		batch, err := s.historyStore.GetLatencyBatch(context.Background(), summary.BatchID)
		if err != nil {
			return err
		}
		for i := range batch.Items {
			item := &batch.Items[i]
			if item.ExecutionState == "queued" {
				item.ExecutionState = "not_executed"
				if userCancelled {
					item.ExecutionState = "cancelled"
				}
				item.PersistenceState = "not_applicable"
			}
			if item.ExecutionState == "running" {
				if item.Result != nil && item.ResultStaged {
					item.ExecutionState = "completed"
					if item.Result.SuccessSamples == 0 {
						item.ExecutionState = "failed"
					}
					item.PersistenceState = "failed"
					item.PersistenceError = "应用在保存该结果前退出；可重试保存"
				} else if item.Result != nil {
					item.ExecutionState = "completed"
					if item.Result.SuccessSamples == 0 {
						item.ExecutionState = "failed"
					}
					item.PersistenceState = "saved"
				} else {
					item.ExecutionState = "interrupted"
					item.PersistenceState = "not_applicable"
				}
			}
			if item.PersistenceState == "saving" && item.Result != nil {
				item.PersistenceState = "failed"
				item.PersistenceError = "应用退出时保存未完成；可重试保存"
				item.ResultStaged = true
			}
			if err := s.historyStore.UpdateLatencyBatchItem(context.Background(), item); err != nil {
				return err
			}
		}
		batch.State = "interrupted"
		if err := s.historyStore.UpdateLatencyBatchState(context.Background(), batch.BatchID, batch.State); err != nil {
			return err
		}
	}
	return nil
}

func deriveLatencyBatchState(items []history.LatencyBatchItem, cancelling, shutdown bool) string {
	queuedOrRunning := false
	anyCancelled, anyIssue, anySaveFailure := false, false, false
	for _, item := range items {
		switch item.ExecutionState {
		case "queued", "running":
			queuedOrRunning = true
		case "cancelled":
			anyCancelled = true
		case "failed", "skipped_config", "not_executed", "interrupted":
			anyIssue = true
		}
		if item.PersistenceState == "failed" {
			anySaveFailure = true
		}
	}
	if queuedOrRunning {
		if cancelling {
			return "cancelling"
		}
		return "running"
	}
	if shutdown {
		if anySaveFailure {
			return "interrupted_with_save_failures"
		}
		if anyIssue {
			return "interrupted_with_issues"
		}
		return "interrupted"
	}
	if anyCancelled {
		if anySaveFailure {
			return "cancelled_with_save_failures"
		}
		if anyIssue {
			return "cancelled_with_issues"
		}
		return "cancelled"
	}
	if anySaveFailure {
		return "completed_with_save_failures"
	}
	if anyIssue {
		return "completed_with_issues"
	}
	return "completed"
}

func workbenchLatencyBatchDTO(batch history.LatencyBatch) *WorkbenchLatencyBatchDTO {
	dto := &WorkbenchLatencyBatchDTO{BatchID: batch.BatchID, RequestID: batch.RequestID, TestProject: batch.TestProject, TimeoutSeconds: batch.TimeoutSeconds, RequestedAt: batch.RequestedAt, State: batch.State, ItemCount: batch.ItemCount}
	if len(batch.Items) == 0 {
		return dto
	}
	dto.Items = make([]WorkbenchLatencyBatchItemDTO, 0, len(batch.Items))
	for _, item := range batch.Items {
		itemDTO := WorkbenchLatencyBatchItemDTO{ItemID: item.ItemID, BatchID: item.BatchID, Ordinal: item.Ordinal, ProfileID: item.ProfileID, NodeKey: item.NodeKey, NodeIdentityKey: item.NodeIdentityKey, ConfigRevisionKey: item.ConfigRevisionKey, DisplayName: item.DisplayName, NodeType: item.NodeType, ExecutionState: item.ExecutionState, PersistenceState: item.PersistenceState, AttemptID: item.AttemptID, RequestedAt: item.RequestedAt, StartedAt: item.StartedAt, FinishedAt: item.FinishedAt, ErrorMessage: item.ErrorMessage, PersistenceError: item.PersistenceError}
		if item.Result != nil {
			result := workbenchLatencyDTO(*item.Result, item.PersistenceState, item.PersistenceError)
			itemDTO.Result = &result
		}
		dto.Items = append(dto.Items, itemDTO)
	}
	return dto
}

func (s *AppService) emitWorkbenchLatencyBatch(batch *history.LatencyBatch) {
	if s.emitter == nil || batch == nil {
		return
	}
	copyBatch := *batch
	copyBatch.Items = append([]history.LatencyBatchItem(nil), batch.Items...)
	s.emitter.Emit(Event{Type: "workbench_latency_batch_updated", Payload: workbenchLatencyBatchDTO(copyBatch)})
}

func newWorkbenchLatencyBatchID() string     { return newWorkbenchLatencyID("latency_batch_") }
func newWorkbenchLatencyBatchItemID() string { return newWorkbenchLatencyID("latency_item_") }
func newWorkbenchLatencyID(prefix string) string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return prefix + hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
}
