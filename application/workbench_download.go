package application

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/speedtester"
)

const workbenchDownloadRuleVersion = 3

type workbenchDownloadRuntime struct {
	timingNS        map[string]int64
	physicalEgress  *speedtester.PhysicalDownloadEgress
	preparePhysical func(context.Context, *speedtester.CProxy) (*speedtester.CProxy, *speedtester.PhysicalDownloadEgress, error)
	networkDone     chan struct{}
	attemptID       string
	requestID       string
	ctx             context.Context
	cancel          context.CancelFunc
	ready           chan struct{}
	mu              sync.Mutex
	cleanupOnce     sync.Once
	shutdown        bool
	node            monitor.MonitoredNode
	proxy           *speedtester.CProxy
	engine          *speedtester.SpeedTester
	rule            history.WorkbenchDownloadRuleSnapshot
}

func (s *AppService) StartWorkbenchDownloadTest(ctx context.Context, req WorkbenchDownloadTestRequest) (returnedAttempt *history.WorkbenchDownloadAttempt, startErr error) {
	received := time.Now()
	defer func() {
		if s.historyStore != nil && startErr == nil && returnedAttempt != nil {
			_ = s.historyStore.DB().MarkMeasurementNotExecuted(context.Background(), req.RequestID, "")
			return
		}
		if s.historyStore == nil || startErr == nil || errors.Is(startErr, errWorkbenchAdmissionBusy) {
			return
		}
		if _, err := s.historyStore.GetWorkbenchDownloadAttemptByRequestID(context.Background(), req.RequestID); errors.Is(err, history.ErrWorkbenchDownloadAttemptNotFound) {
			_ = s.historyStore.DB().MarkMeasurementNotExecuted(context.Background(), req.RequestID, startErr.Error())
			_ = s.historyStore.DB().FinishAutomaticMeasurementRound(context.Background(), req.RequestID)
		}
	}()
	if s.historyStore == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	requestID := strings.TrimSpace(req.RequestID)
	profileID, nodeKey := strings.TrimSpace(req.ProfileID), strings.TrimSpace(req.NodeKey)
	identityKey, revisionKey := strings.TrimSpace(req.NodeIdentityKey), strings.TrimSpace(req.ConfigRevisionKey)
	if requestID == "" || len(requestID) > 128 {
		return nil, monitor.NewValidationError("request_id 必须为 1 到 128 个字符")
	}
	if profileID == "" || nodeKey == "" || identityKey == "" || revisionKey == "" {
		return nil, monitor.NewValidationError("下载请求必须包含 profile、node_key、稳定身份和配置 revision")
	}
	timeoutSeconds := req.TimeoutSeconds
	if timeoutSeconds == 0 {
		timeoutSeconds = int64(speedtester.DefaultDownloadStreamTimeout.Seconds())
	}
	if timeoutSeconds < 1 || timeoutSeconds > int64(speedtester.MaximumDownloadStreamTimeout.Seconds()) {
		return nil, monitor.NewValidationError("下载时长上限必须在 1 到 30 秒之间")
	}
	maximumBytes := req.MaximumBytes
	if maximumBytes == 0 {
		maximumBytes = speedtester.DefaultDownloadStreamBytes
	}
	if maximumBytes < 1 || maximumBytes > speedtester.MaximumDownloadStreamBytes {
		return nil, monitor.NewValidationError("下载响应体读取上限必须在 1 字节到 100 MiB 之间")
	}
	prior, err := s.historyStore.GetWorkbenchDownloadAttemptByRequestID(context.Background(), requestID)
	if err == nil {
		if err := validateWorkbenchDownloadRequest(prior, profileID, nodeKey, identityKey, revisionKey, maximumBytes, timeoutSeconds); err != nil {
			return nil, err
		}
		return prior, nil
	}
	if err != history.ErrWorkbenchDownloadAttemptNotFound {
		return nil, fmt.Errorf("检查下载请求身份失败: %w", err)
	}

	if err := s.validateMeasurementRequest(ctx, requestID, profileID, nodeKey, identityKey, revisionKey, "download", ""); err != nil {
		return nil, err
	}
	node, proxy, err := s.resolveWorkbenchDownloadNode(profileID, nodeKey)
	if err != nil {
		return nil, monitor.NewValidationError("无法安全解析当前节点配置；未发出下载请求")
	}
	if node.NodeKey != nodeKey || node.NodeIdentityKey != identityKey || node.ConfigRevisionKey != revisionKey {
		return nil, monitor.NewValidationError("所选节点身份或配置 revision 已变化；未发出下载请求")
	}
	if len(node.RawConfig) == 0 && s.workbenchDownloadResolveHook == nil {
		return nil, monitor.NewValidationError("当前节点缓存配置不可用；未发出下载请求")
	}
	handedOff := false
	defer func() {
		if !handedOff {
			closeWorkbenchDownloadProxy(proxy)
		}
	}()
	engine, err := speedtester.New(&speedtester.Config{
		ConfigPaths: s.profilePaths.CacheFile(profileID), ServerURL: speedtester.DefaultSpeedServer,
		Mode: speedtester.SpeedModeDownload, Metrics: speedtester.MetricSet{Download: true}, Concurrent: 1,
	})
	if err != nil {
		return nil, fmt.Errorf("初始化固定下载目标失败；未发出请求: %w", err)
	}
	target, err := engine.DownloadStreamTarget(maximumBytes)
	if err != nil {
		return nil, fmt.Errorf("初始化固定下载目标失败；未发出请求: %w", err)
	}
	if err := s.checkDownloadRetryAfter(ctx, req, target); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	rule := history.WorkbenchDownloadRuleSnapshot{
		RuleVersion: workbenchDownloadRuleVersion, TargetURL: target, Method: http.MethodGet,
		MaximumBytes: maximumBytes, MaximumDurationNS: int64(time.Duration(timeoutSeconds) * time.Second),
		SampleEveryBytes: speedtester.DefaultDownloadSampleBytes, SampleEveryNS: int64(100 * time.Millisecond),
	}
	runCtx, cancel := newWorkbenchDownloadLifecycleContext(ctx, time.Duration(timeoutSeconds)*time.Second)
	runtime := &workbenchDownloadRuntime{timingNS: probeTiming(ctx), networkDone: make(chan struct{}),
		attemptID: newWorkbenchLatencyID("workbench_download_attempt_"), requestID: requestID,
		ctx: runCtx, cancel: cancel, ready: make(chan struct{}), node: node, proxy: proxy, engine: engine, rule: rule,
	}
	if s.workbenchDownloadResolveHook == nil {
		runtime.preparePhysical = speedtester.PreparePhysicalDownloadProxy
	}
	waitForExisting, duplicate, err := s.beginWorkbenchDownload(runtime)
	if err != nil {
		cancel()
		return nil, err
	}
	if duplicate {
		cancel()
		<-waitForExisting
		prior, err := s.historyStore.GetWorkbenchDownloadAttemptByRequestID(context.Background(), requestID)
		if err != nil {
			return nil, fmt.Errorf("相同 request_id 的下载检测未能建立: %w", err)
		}
		if err := validateWorkbenchDownloadRequest(prior, profileID, nodeKey, identityKey, revisionKey, maximumBytes, timeoutSeconds); err != nil {
			return nil, err
		}
		return prior, nil
	}
	abortStart := func() {
		cancel()
		close(runtime.ready)
		s.endWorkbenchDownload(runtime)
	}
	// Another call can finish this request while node resolution is in progress.
	// Recheck after reserving the execution slot, before creating a new identity.
	prior, err = s.historyStore.GetWorkbenchDownloadAttemptByRequestID(context.Background(), requestID)
	if err == nil {
		abortStart()
		if err := validateWorkbenchDownloadRequest(prior, profileID, nodeKey, identityKey, revisionKey, maximumBytes, timeoutSeconds); err != nil {
			return nil, err
		}
		return prior, nil
	}
	if err != history.ErrWorkbenchDownloadAttemptNotFound {
		abortStart()
		return nil, fmt.Errorf("检查下载检测请求失败: %w", err)
	}
	if runtime.preparePhysical != nil {
		pathPlan := speedtester.PlanPhysicalDownloadPath(runtime.ctx, proxy)
		applyWorkbenchDownloadRulePath(&rule, pathPlan)
		runtime.rule = rule
	}
	runtime.timingNS["start_setup_ns"] = time.Since(received).Nanoseconds()
	attempt := &history.WorkbenchDownloadAttempt{
		AttemptID: runtime.attemptID, RequestID: requestID, ProfileID: profileID, NodeKey: nodeKey,
		NodeIdentityKey: identityKey, ConfigRevisionKey: revisionKey, DisplayName: node.DisplayName,
		NodeType: node.Type, Source: history.WorkbenchDownloadSource, RequestedAt: now,
		ExecutionState: "queued", PersistenceState: "not_started", Rule: rule,
	}
	if s.workbenchDownloadAttemptCreateHook != nil {
		err = s.workbenchDownloadAttemptCreateHook(context.Background(), attempt)
	} else {
		err = s.historyStore.CreateWorkbenchDownloadAttempt(context.Background(), attempt)
	}
	if err != nil {
		abortStart()
		return nil, fmt.Errorf("无法保存下载检测身份；未发出网络请求: %w", err)
	}
	startedAt := time.Now().UTC()
	if err := s.historyStore.BeginWorkbenchDownloadAttempt(context.Background(), attempt.AttemptID, startedAt); err != nil {
		// No download can be running while this slot is reserved. Use the existing
		// recovery state to finish the durable identity without issuing a request.
		if recoveryErr := s.reconcileWorkbenchDownloadAttempts(); recoveryErr != nil {
			err = fmt.Errorf("%w; 更新未启动检测状态失败: %v", err, recoveryErr)
		}
		abortStart()
		return nil, fmt.Errorf("无法登记下载检测启动；未发出网络请求: %w", err)
	}
	attempt.StartedAt = &startedAt
	attempt.ExecutionState = "running"
	close(runtime.ready)
	handedOff = true
	go s.executeWorkbenchDownload(runtime)
	return attempt, nil
}

func validateWorkbenchDownloadRequest(prior *history.WorkbenchDownloadAttempt, profileID, nodeKey, identityKey, revisionKey string, maximumBytes, timeoutSeconds int64) error {
	if prior.ProfileID != profileID || prior.NodeKey != nodeKey || prior.NodeIdentityKey != identityKey || prior.ConfigRevisionKey != revisionKey ||
		prior.Rule.MaximumBytes != maximumBytes || prior.Rule.MaximumDurationNS != (time.Duration(timeoutSeconds)*time.Second).Nanoseconds() {
		return monitor.NewValidationError("request_id 已用于另一组下载检测参数")
	}
	return nil
}

func (s *AppService) resolveWorkbenchDownloadNode(profileID, nodeKey string) (monitor.MonitoredNode, *speedtester.CProxy, error) {
	if s.workbenchDownloadResolveHook != nil {
		return s.workbenchDownloadResolveHook(profileID, nodeKey)
	}
	node, _, proxy, err := s.resolveWorkbenchLatencyProxy(profileID, nodeKey)
	return node, proxy, err
}

func (s *AppService) beginWorkbenchDownload(runtime *workbenchDownloadRuntime) (<-chan struct{}, bool, error) {
	s.workbenchMu.Lock()
	defer s.workbenchMu.Unlock()
	if s.workbenchClosed {
		return nil, false, fmt.Errorf("应用正在关闭，不能启动下载测试")
	}
	if s.workbenchActiveDownload != nil {
		if s.workbenchActiveDownload.requestID == runtime.requestID {
			return s.workbenchActiveDownload.ready, true, nil
		}
		return nil, false, fmt.Errorf("已有 Workbench 主动测试正在运行，下载测试暂不可启动: %w", errWorkbenchAdmissionBusy)
	}
	if s.workbenchActiveBatch != nil || s.workbenchActiveSingles > 0 {
		return nil, false, fmt.Errorf("已有 Workbench 主动测试正在运行，下载测试暂不可启动: %w", errWorkbenchAdmissionBusy)
	}
	if s.workbenchPublicServiceStarting > 0 {
		return nil, false, fmt.Errorf("公共服务检测正在启动，下载测试暂不可启动: %w", errWorkbenchAdmissionBusy)
	}
	s.publicServiceMu.Lock()
	defer s.publicServiceMu.Unlock()
	if s.publicServiceClosed || s.publicServiceTransition {
		return nil, false, fmt.Errorf("服务正在关闭或存储正在切换，未执行下载")
	}
	for _, active := range s.publicServiceActive {
		if !active.periodic || probeNetworkRunning(active.networkDone) {
			return nil, false, fmt.Errorf("已有公共服务检测正在运行，下载测试暂不可启动: %w", errWorkbenchAdmissionBusy)
		}
	}
	s.workbenchActiveDownload = runtime
	s.workbenchWG.Add(1)
	return nil, false, nil
}

func (s *AppService) endWorkbenchDownload(runtime *workbenchDownloadRuntime) {
	s.workbenchMu.Lock()
	if s.workbenchActiveDownload == runtime {
		s.workbenchActiveDownload = nil
	}
	s.workbenchMu.Unlock()
	s.workbenchWG.Done()
}

func (s *AppService) executeWorkbenchDownload(runtime *workbenchDownloadRuntime) {
	finishNetwork := finishProbeNetwork(runtime.networkDone)
	defer finishNetwork()
	defer func() {
		runtime.cancel()
		runtime.closeNetwork()
		_ = s.historyStore.DB().FinishAutomaticMeasurementRound(context.Background(), runtime.requestID)
		s.endWorkbenchDownload(runtime)
	}()
	finishActivity := s.probeActivity.begin("download")
	var result speedtester.DownloadStreamResult
	started := time.Now().UTC()
	var preparationErr error
	if err := runtime.ctx.Err(); err != nil {
		preparationErr = err
	} else if runtime.preparePhysical != nil {
		original := runtime.proxy
		prepared, egress, err := runtime.preparePhysical(runtime.ctx, original)
		runtime.physicalEgress = egress
		if prepared != nil {
			runtime.proxy = prepared
		}
		if err == nil && original != nil && runtime.proxy != original {
			closeWorkbenchDownloadProxy(original)
		}
		preparationErr = err
		if preparationErr == nil {
			preparationErr = runtime.ctx.Err()
		}
	}
	if preparationErr != nil {
		result = workbenchDownloadPreflightFailure(runtime, started, time.Now().UTC(), preparationErr)
	} else {
		client, err := s.workbenchDownloadHTTPClient(runtime.engine, runtime.proxy, time.Duration(runtime.rule.MaximumDurationNS))
		if err != nil {
			now := time.Now().UTC()
			result = speedtester.DownloadStreamResult{Outcome: "connection_failed", TargetURL: runtime.rule.TargetURL, StartedAt: now, FinishedAt: now, Phase: "proxy_setup", ErrorClass: "proxy_setup", EndReason: "connection_failed", FailurePhase: "proxy_setup", ErrorMessage: "无法建立节点隔离下载连接", Samples: []speedtester.DownloadStreamSample{}}
		} else {
			options := speedtester.DownloadStreamOptions{
				MaximumBytes: runtime.rule.MaximumBytes, MaximumDuration: time.Duration(runtime.rule.MaximumDurationNS),
				SampleEveryBytes: runtime.rule.SampleEveryBytes, SampleEveryDuration: time.Duration(runtime.rule.SampleEveryNS),
			}
			result = runtime.engine.MeasureDownloadStream(runtime.ctx, client, options, func(sample speedtester.DownloadStreamSample, bytesRead int64) {
				if s.emitter != nil {
					s.emitter.Emit(Event{Type: "workbench_download_progress", Payload: map[string]any{
						"attempt_id": runtime.attemptID, "bytes_read": bytesRead, "sample": sample,
					}})
				}
			})
		}
	}
	runtime.closeNetwork()
	finishNetwork()
	path := downloadNetworkPath(runtime, preparationErr)
	normalizeUnverifiedPathFailure(&result, path)
	runtime.mu.Lock()
	shutdown := runtime.shutdown
	runtime.mu.Unlock()
	interrupted := shutdown && result.Outcome == "user_cancelled"
	if interrupted {
		result.Outcome = "interrupted"
		result.FailurePhase = "application_exit"
		result.ErrorMessage = "应用退出时下载被中断；已读取数据保留"
	}
	executionState := result.Outcome
	if executionState == "completed" || executionState == "byte_limit" || executionState == "time_limit" {
		executionState = "completed"
	}
	if interrupted {
		executionState = "interrupted"
	}
	measurement := workbenchDownloadMeasurement(result)
	measurement.PhaseTimingsNS = result.PhaseTimingsNS
	if runtime.timingNS == nil {
		runtime.timingNS = make(map[string]int64)
	}
	measurement.TimingNS = runtime.timingNS
	measurement.TimingNS["network_ns"] = result.DurationNS
	measurement.NetworkPath = &path
	measurement.CompetingProbes = finishActivity()
	if err := s.stageWorkbenchDownloadResult(runtime.attemptID, executionState, measurement); err != nil {
		_ = s.historyStore.MarkWorkbenchDownloadStageFailed(context.Background(), runtime.attemptID, executionState, measurement.FinishedAt,
			"测量已结束，但结果未能暂存；该结果无法重试保存，请重新执行检测")
		s.emitWorkbenchDownloadUpdate(runtime.requestID)
		return
	}
	if err := s.saveWorkbenchDownloadResult(runtime.attemptID); err != nil {
		_ = s.historyStore.MarkWorkbenchDownloadSaveFailed(context.Background(), runtime.attemptID, "结果仍已暂存；保存失败，可沿用原 attempt 重试")
	}
	s.emitWorkbenchDownloadUpdate(runtime.requestID)
}

func (runtime *workbenchDownloadRuntime) closeNetwork() {
	if runtime == nil {
		return
	}
	runtime.cleanupOnce.Do(func() {
		closeWorkbenchDownloadProxy(runtime.proxy)
	})
}

func closeWorkbenchDownloadProxy(proxy *speedtester.CProxy) {
	if proxy == nil || proxy.Proxy == nil {
		return
	}
	_ = proxy.Close()
}

func workbenchDownloadPreflightFailure(runtime *workbenchDownloadRuntime, started, finished time.Time, cause error) speedtester.DownloadStreamResult {
	reason := preflightFailureReason(runtime.ctx, cause)
	return speedtester.DownloadStreamResult{
		Outcome: "not_executed", TargetURL: runtime.rule.TargetURL, StartedAt: started.UTC(), FinishedAt: finished.UTC(),
		DurationNS: finished.Sub(started).Nanoseconds(), Phase: "physical_preflight", ErrorClass: "physical_path",
		EndReason: reason, FailurePhase: "physical_preflight", ErrorMessage: "物理出口预检未完成；下载未执行",
		Samples: []speedtester.DownloadStreamSample{},
	}
}

func preflightFailureReason(ctx context.Context, cause error) string {
	if errors.Is(cause, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return "user_cancelled"
	}
	if errors.Is(cause, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "deadline_exceeded"
	}
	return "physical_path_unverified"
}

func downloadNetworkPath(runtime *workbenchDownloadRuntime, preparationErr error) speedtester.DownloadNetworkPath {
	if runtime.physicalEgress != nil {
		path := runtime.physicalEgress.Snapshot()
		if preparationErr != nil && path.FailureReason == "" {
			path.FailureReason = preflightFailureReason(runtime.ctx, preparationErr)
		}
		return path
	}
	path := speedtester.DownloadNetworkPath{
		Method: "unknown", AddressFamily: "unknown", AddressSource: "unobserved",
		TUNEvidence: "packet_route_not_observed", FailureReason: "physical_path_unobserved",
	}
	if preparationErr != nil {
		path.FailureReason = preflightFailureReason(runtime.ctx, preparationErr)
	}
	return path
}

func normalizeUnverifiedPathFailure(result *speedtester.DownloadStreamResult, path speedtester.DownloadNetworkPath) {
	if result == nil || path.Method != "legacy_default_binding_unverified" {
		return
	}
	if result.Outcome != "connection_failed" && result.Outcome != "transfer_interrupted" {
		return
	}
	result.Outcome = "local_path_unverified"
	result.EndReason = "local_path_unverified"
	if result.FailurePhase == "" {
		result.FailurePhase = "local_egress"
	}
	result.ErrorMessage = "本机物理出口未能确认；不作为节点故障或有效速度读数"
}

func (s *AppService) workbenchDownloadHTTPClient(engine *speedtester.SpeedTester, proxy *speedtester.CProxy, timeout time.Duration) (*http.Client, error) {
	if s.workbenchDownloadClientFactory != nil {
		return s.workbenchDownloadClientFactory(engine, proxy, timeout)
	}
	return engine.NewNodeHTTPClient(proxy, timeout)
}

func workbenchDownloadMeasurement(result speedtester.DownloadStreamResult) history.WorkbenchDownloadMeasurement {
	samples := make([]history.WorkbenchDownloadSample, 0, len(result.Samples))
	for _, sample := range result.Samples {
		samples = append(samples, history.WorkbenchDownloadSample{
			ElapsedNS: sample.ElapsedNS, IntervalNS: sample.IntervalNS, DeltaBytes: sample.DeltaBytes,
			CumulativeBytes: sample.CumulativeBytes, SpeedMbps: sample.SpeedMbps,
		})
	}
	return history.WorkbenchDownloadMeasurement{
		Phase: result.Phase, ErrorClass: result.ErrorClass, EndReason: result.EndReason, PartialMeasurement: result.PartialMeasurement, RetryAfterSeconds: result.RetryAfterSeconds,
		Outcome: result.Outcome, HTTPStatus: result.HTTPStatus, BytesRead: result.BytesRead,
		StartedAt: result.StartedAt.UTC(), FinishedAt: result.FinishedAt.UTC(), DurationNS: result.DurationNS,
		FailurePhase: result.FailurePhase, ErrorMessage: safeDownloadError(result.ErrorMessage), Samples: samples,
	}
}

func safeDownloadError(message string) string {
	const maxErrorLength = 240
	message = strings.TrimSpace(message)
	if len(message) > maxErrorLength {
		message = message[:maxErrorLength]
	}
	return message
}

func (s *AppService) stageWorkbenchDownloadResult(attemptID, state string, result history.WorkbenchDownloadMeasurement) error {
	if s.workbenchDownloadStageHook != nil {
		return s.workbenchDownloadStageHook(context.Background(), attemptID, state, result)
	}
	return s.historyStore.StageWorkbenchDownloadResult(context.Background(), attemptID, state, result)
}

func (s *AppService) saveWorkbenchDownloadResult(attemptID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), workbenchLatencyPersistenceTimeout)
	defer cancel()
	if s.workbenchDownloadSaveHook != nil {
		if err := s.workbenchDownloadSaveHook(ctx, attemptID); err != nil {
			return err
		}
	}
	return s.historyStore.CommitWorkbenchDownloadResult(ctx, attemptID)
}

func (s *AppService) emitWorkbenchDownloadUpdate(requestID string) {
	if s.emitter == nil {
		return
	}
	if attempt, err := s.historyStore.GetWorkbenchDownloadAttemptByRequestID(context.Background(), requestID); err == nil {
		s.emitter.Emit(Event{Type: "workbench_download_attempt_updated", Payload: attempt})
	}
}

func (s *AppService) CancelWorkbenchDownloadTest(ctx context.Context, attemptID string, query WorkbenchDownloadHistoryQuery) (*history.WorkbenchDownloadAttempt, error) {
	if s.historyStore == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	filter, err := workbenchDownloadHistoryFilter(query)
	if err != nil {
		return nil, err
	}
	if _, err := s.historyStore.GetWorkbenchDownloadAttempt(ctx, strings.TrimSpace(attemptID), filter); err != nil {
		return nil, err
	}
	s.workbenchMu.Lock()
	runtime := s.workbenchActiveDownload
	if runtime == nil || runtime.attemptID != attemptID {
		s.workbenchMu.Unlock()
		return nil, fmt.Errorf("指定下载检测当前没有可取消的执行")
	}
	changed, err := s.historyStore.RequestWorkbenchDownloadCancellation(context.Background(), attemptID)
	if changed {
		runtime.cancel()
	}
	s.workbenchMu.Unlock()
	if err != nil {
		return nil, err
	}
	return s.GetWorkbenchDownloadAttempt(ctx, attemptID, query)
}

func (s *AppService) RetrySaveWorkbenchDownloadTest(ctx context.Context, attemptID string, query WorkbenchDownloadHistoryQuery) (*history.WorkbenchDownloadAttempt, error) {
	if s.historyStore == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	filter, err := workbenchDownloadHistoryFilter(query)
	if err != nil {
		return nil, err
	}
	current, err := s.historyStore.GetWorkbenchDownloadAttempt(ctx, strings.TrimSpace(attemptID), filter)
	if err != nil {
		return nil, err
	}
	if current.PersistenceState == "saved" {
		return current, nil
	}
	if current.PersistenceState != "failed" || current.Result == nil {
		return nil, monitor.NewValidationError("没有可重试保存的暂存结果")
	}
	s.workbenchMu.Lock()
	active := s.workbenchActiveDownload != nil && s.workbenchActiveDownload.attemptID == attemptID
	closed := s.workbenchClosed
	s.workbenchMu.Unlock()
	if active || closed {
		return nil, fmt.Errorf("下载检测仍在进行或应用正在关闭")
	}
	s.workbenchMu.Lock()
	if s.workbenchClosed {
		s.workbenchMu.Unlock()
		return nil, fmt.Errorf("应用正在关闭")
	}
	s.workbenchWG.Add(1)
	s.workbenchMu.Unlock()
	defer s.workbenchWG.Done()
	if err := s.saveWorkbenchDownloadResult(attemptID); err != nil {
		_ = s.historyStore.MarkWorkbenchDownloadSaveFailed(context.Background(), attemptID, "结果仍已暂存；保存失败，可再次沿用原 attempt 重试")
		return nil, err
	}
	return s.historyStore.GetWorkbenchDownloadAttempt(ctx, attemptID, filter)
}

func (s *AppService) GetWorkbenchDownloadAttempt(ctx context.Context, attemptID string, query WorkbenchDownloadHistoryQuery) (*history.WorkbenchDownloadAttempt, error) {
	if s.historyStore == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	filter, err := workbenchDownloadHistoryFilter(query)
	if err != nil {
		return nil, err
	}
	return s.historyStore.GetWorkbenchDownloadAttempt(ctx, strings.TrimSpace(attemptID), filter)
}

func (s *AppService) ListWorkbenchDownloadTests(ctx context.Context, query WorkbenchDownloadHistoryQuery) (WorkbenchDownloadHistoryResult, error) {
	if s.historyStore == nil {
		return WorkbenchDownloadHistoryResult{}, fmt.Errorf("history store is not initialized")
	}
	filter, err := workbenchDownloadHistoryFilter(query)
	if err != nil {
		return WorkbenchDownloadHistoryResult{}, err
	}
	since, until, err := normalizeWorkbenchLatencyWindow(query.Since, query.Until)
	if err != nil {
		return WorkbenchDownloadHistoryResult{}, monitor.NewValidationError(err.Error())
	}
	page, err := s.historyStore.QueryWorkbenchDownloadAttempts(ctx, filter)
	if err != nil {
		return WorkbenchDownloadHistoryResult{}, err
	}
	result := WorkbenchDownloadHistoryResult{Attempts: page.Attempts, HasMore: page.HasMore, Complete: !page.HasMore}
	if since != nil {
		result.Since, result.Until = *since, *until
	}
	return result, nil
}

func applyWorkbenchDownloadRulePath(rule *history.WorkbenchDownloadRuleSnapshot, path speedtester.DownloadNetworkPath) {
	if rule == nil {
		return
	}
	rule.NetworkPathMethod = path.Method
	rule.PhysicalInterface = path.Interface
	rule.DNSMode = path.DNSMode
}

func (s *AppService) reconcileWorkbenchDownloadAttempts() error {
	if s.historyStore == nil {
		return nil
	}
	return s.historyStore.ReconcileWorkbenchDownloadAttempts(context.Background())
}

func workbenchDownloadHistoryFilter(query WorkbenchDownloadHistoryQuery) (history.WorkbenchDownloadFilter, error) {
	filter := history.WorkbenchDownloadFilter{
		ProfileID: strings.TrimSpace(query.ProfileID), NodeKey: strings.TrimSpace(query.NodeKey),
		NodeIdentityKey: strings.TrimSpace(query.NodeIdentityKey), ConfigRevisionKey: strings.TrimSpace(query.ConfigRevisionKey),
		Since: query.Since, Until: query.Until, Limit: query.Limit, BeforeAt: query.BeforeAt, BeforeAttemptID: strings.TrimSpace(query.BeforeAttemptID),
	}
	if filter.ProfileID == "" || filter.NodeKey == "" || filter.NodeIdentityKey == "" || filter.ConfigRevisionKey == "" {
		return history.WorkbenchDownloadFilter{}, monitor.NewValidationError("下载历史查询必须包含 profile、node_key、稳定身份和配置 revision")
	}
	if filter.Limit < 0 || filter.Limit > 100 {
		return history.WorkbenchDownloadFilter{}, monitor.NewValidationError("下载历史分页 limit 必须在 0 到 100 之间")
	}
	if filter.BeforeAttemptID != "" && filter.BeforeAt == nil {
		return history.WorkbenchDownloadFilter{}, monitor.NewValidationError("下载历史分页游标不完整")
	}
	return filter, nil
}

func (s *AppService) reserveWorkbenchPublicServiceStart() error {
	s.workbenchMu.Lock()
	defer s.workbenchMu.Unlock()
	if s.workbenchClosed {
		return fmt.Errorf("应用正在关闭，不能开始公共服务检测")
	}
	if s.workbenchActiveDownload != nil && probeNetworkRunning(s.workbenchActiveDownload.networkDone) {
		return fmt.Errorf("下载测量正在运行，其他 Workbench 主动测试暂不可启动: %w", errWorkbenchAdmissionBusy)
	}
	s.workbenchPublicServiceStarting++
	return nil
}

func (s *AppService) releaseWorkbenchPublicServiceStart() {
	s.workbenchMu.Lock()
	if s.workbenchPublicServiceStarting > 0 {
		s.workbenchPublicServiceStarting--
	}
	s.workbenchMu.Unlock()
}
