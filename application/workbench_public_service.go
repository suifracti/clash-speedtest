package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/publicservice"
)

const workbenchPublicServiceSource = "workbench_public_service"

type publicServiceRuntime struct {
	attemptID string
	requestID string
	cancel    context.CancelFunc
	node      monitor.MonitoredNode
	rule      publicservice.Rule
	timeout   time.Duration
}

func (s *AppService) ListWorkbenchPublicServiceCatalog() []publicservice.Rule {
	return publicservice.Catalog()
}

// StartWorkbenchPublicServiceTest first commits the immutable attempt identity
// and rule snapshot, then schedules one request in the selected node's isolated
// proxy path. Repeating RequestID returns the same attempt without remeasuring.
func (s *AppService) StartWorkbenchPublicServiceTest(_ context.Context, req WorkbenchPublicServiceTestRequest) (*history.PublicServiceAttempt, error) {
	if s.historyStore == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	requestID := strings.TrimSpace(req.RequestID)
	profileID := strings.TrimSpace(req.ProfileID)
	nodeKey := strings.TrimSpace(req.NodeKey)
	identityKey := strings.TrimSpace(req.NodeIdentityKey)
	revisionKey := strings.TrimSpace(req.ConfigRevisionKey)
	serviceID := strings.TrimSpace(req.ServiceID)
	if requestID == "" || len(requestID) > 128 {
		return nil, monitor.NewValidationError("request_id 必须为 1 到 128 个字符")
	}
	if profileID == "" || nodeKey == "" || identityKey == "" || revisionKey == "" {
		return nil, monitor.NewValidationError("检测请求必须包含 profile、node_key、稳定身份和配置 revision")
	}
	rule, ok := publicservice.RuleFor(serviceID)
	if !ok {
		return nil, monitor.NewValidationError("所选公共服务不在固定目录中")
	}
	timeoutSeconds := req.TimeoutSeconds
	if timeoutSeconds == 0 {
		timeoutSeconds = int64(publicservice.DefaultTimeout.Seconds())
	}
	if timeoutSeconds < 1 || timeoutSeconds > int64(publicservice.MaximumTimeout.Seconds()) {
		return nil, monitor.NewValidationError("公共服务检测超时必须在 1 到 30 秒之间")
	}
	rule.TimeoutSeconds = int(timeoutSeconds)
	ruleSnapshot := publicServiceRuleSnapshot(rule, timeoutSeconds)

	s.publicServiceMu.Lock()
	if s.publicServiceClosed {
		s.publicServiceMu.Unlock()
		return nil, fmt.Errorf("应用正在关闭，不能开始公共服务检测")
	}
	if s.publicServiceTransition {
		s.publicServiceMu.Unlock()
		return nil, fmt.Errorf("应用数据正在迁移，不能开始公共服务检测")
	}
	prior, priorErr := s.historyStore.GetPublicServiceAttemptByRequestID(context.Background(), requestID)
	if priorErr == nil {
		s.publicServiceMu.Unlock()
		if prior.ProfileID != profileID || prior.NodeKey != nodeKey || prior.NodeIdentityKey != identityKey ||
			prior.ConfigRevisionKey != revisionKey || prior.ServiceID != serviceID || prior.Rule.TimeoutSeconds != timeoutSeconds {
			return nil, monitor.NewValidationError("request_id 已用于另一组检测参数")
		}
		return prior, nil
	}
	if priorErr != history.ErrPublicServiceAttemptNotFound {
		s.publicServiceMu.Unlock()
		return nil, fmt.Errorf("检查公共服务请求身份失败: %w", priorErr)
	}
	if s.publicServiceActive == nil {
		s.publicServiceActive = make(map[string]*publicServiceRuntime)
	}
	if len(s.publicServiceActive) > 0 {
		s.publicServiceMu.Unlock()
		return nil, fmt.Errorf("已有公共服务检测正在运行，请等待完成或取消")
	}

	node, err := s.resolveWorkbenchPublicServiceNode(profileID, nodeKey)
	if err != nil {
		s.publicServiceMu.Unlock()
		return nil, monitor.NewValidationError("无法安全解析当前节点配置；未发出服务请求")
	}
	if node.NodeKey != nodeKey || node.NodeIdentityKey != identityKey || node.ConfigRevisionKey != revisionKey {
		s.publicServiceMu.Unlock()
		return nil, monitor.NewValidationError("所选节点身份或配置 revision 已变化；未发出服务请求")
	}
	if len(node.RawConfig) == 0 && s.publicServiceResolveHook == nil {
		s.publicServiceMu.Unlock()
		return nil, monitor.NewValidationError("当前节点缓存配置不可用；未发出服务请求")
	}
	now := time.Now().UTC()
	attempt := &history.PublicServiceAttempt{
		AttemptID:         newWorkbenchLatencyID("public_service_attempt_"),
		RequestID:         requestID,
		ProfileID:         profileID,
		NodeKey:           nodeKey,
		NodeIdentityKey:   identityKey,
		ConfigRevisionKey: revisionKey,
		DisplayName:       node.DisplayName,
		NodeType:          node.Type,
		Source:            workbenchPublicServiceSource,
		ServiceID:         serviceID,
		Rule:              ruleSnapshot,
		RequestedAt:       now,
		ExecutionState:    "queued",
		PersistenceState:  "not_started",
	}
	var createErr error
	if s.publicServiceAttemptCreateHook != nil {
		createErr = s.publicServiceAttemptCreateHook(context.Background(), attempt)
	} else {
		createErr = s.historyStore.CreatePublicServiceAttempt(context.Background(), attempt)
	}
	if createErr != nil {
		s.publicServiceMu.Unlock()
		return nil, fmt.Errorf("无法保存检测身份；未发出服务请求: %w", createErr)
	}
	startedAt := time.Now().UTC()
	if err := s.historyStore.BeginPublicServiceAttempt(context.Background(), attempt.AttemptID, startedAt); err != nil {
		s.publicServiceMu.Unlock()
		return nil, fmt.Errorf("无法登记检测启动；未发出服务请求: %w", err)
	}
	attempt.StartedAt = &startedAt
	attempt.ExecutionState = "running"
	ctx, cancel := context.WithCancel(context.Background())
	runtime := &publicServiceRuntime{
		attemptID: attempt.AttemptID,
		requestID: attempt.RequestID,
		cancel:    cancel,
		node:      node,
		rule:      rule,
		timeout:   time.Duration(timeoutSeconds) * time.Second,
	}
	s.publicServiceActive[attempt.AttemptID] = runtime
	s.publicServiceWG.Add(1)
	s.publicServiceMu.Unlock()

	go s.executeWorkbenchPublicServiceTest(ctx, runtime)
	return attempt, nil
}

func (s *AppService) resolveWorkbenchPublicServiceNode(profileID, nodeKey string) (monitor.MonitoredNode, error) {
	if s.publicServiceResolveHook != nil {
		return s.publicServiceResolveHook(profileID, nodeKey)
	}
	selected, _, _, err := s.resolveWorkbenchLatencyProxy(profileID, nodeKey)
	return selected, err
}

func (s *AppService) executeWorkbenchPublicServiceTest(ctx context.Context, runtime *publicServiceRuntime) {
	defer s.publicServiceWG.Done()
	defer func() {
		runtime.cancel()
		s.publicServiceMu.Lock()
		delete(s.publicServiceActive, runtime.attemptID)
		s.publicServiceMu.Unlock()
	}()
	result := s.publicServiceChecker.Check(ctx, runtime.node, runtime.rule, runtime.timeout)
	executionState := "failed"
	switch result.Outcome {
	case "matched":
		executionState = "completed"
	case "cancelled":
		executionState = "cancelled"
	}
	measurement := history.PublicServiceMeasurement{
		Outcome:      result.Outcome,
		HTTPStatus:   result.HTTPStatus,
		BytesRead:    result.BytesRead,
		StartedAt:    result.StartedAt,
		FinishedAt:   result.FinishedAt,
		DurationMs:   result.DurationMs,
		FailurePhase: result.FailurePhase,
		ErrorMessage: result.ErrorMessage,
	}
	if err := s.stageWorkbenchPublicServiceResult(runtime.attemptID, executionState, measurement); err != nil {
		_ = s.historyStore.MarkPublicServiceStageFailed(context.Background(), runtime.attemptID, executionState, measurement.FinishedAt,
			"测量已结束，但结果未能暂存；该结果无法重试保存，请重新执行检测")
		s.emitWorkbenchPublicServiceUpdate(runtime.requestID)
		return
	}
	if err := s.saveWorkbenchPublicServiceResult(runtime.attemptID); err != nil {
		_ = s.historyStore.MarkPublicServiceSaveFailed(context.Background(), runtime.attemptID,
			"结果仍已暂存；保存失败，可沿用原 attempt 重试")
	}
	s.emitWorkbenchPublicServiceUpdate(runtime.requestID)
}

func (s *AppService) stageWorkbenchPublicServiceResult(attemptID, executionState string, measurement history.PublicServiceMeasurement) error {
	if s.publicServiceStageHook != nil {
		return s.publicServiceStageHook(context.Background(), attemptID, executionState, measurement)
	}
	return s.historyStore.StagePublicServiceResult(context.Background(), attemptID, executionState, measurement)
}

func (s *AppService) emitWorkbenchPublicServiceUpdate(requestID string) {
	if s.emitter == nil {
		return
	}
	if attempt, err := s.historyStore.GetPublicServiceAttemptByRequestID(context.Background(), requestID); err == nil {
		s.emitter.Emit(Event{Type: "workbench_public_service_attempt_updated", Payload: attempt})
	}
}

func (s *AppService) saveWorkbenchPublicServiceResult(attemptID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), workbenchLatencyPersistenceTimeout)
	defer cancel()
	if s.publicServiceSaveHook != nil {
		if err := s.publicServiceSaveHook(ctx, attemptID); err != nil {
			return err
		}
	}
	return s.historyStore.CommitPublicServiceResult(ctx, attemptID)
}

func (s *AppService) CancelWorkbenchPublicServiceTest(ctx context.Context, attemptID string, query WorkbenchPublicServiceHistoryQuery) (*history.PublicServiceAttempt, error) {
	if s.historyStore == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	filter, err := publicServiceHistoryFilter(query)
	if err != nil {
		return nil, err
	}
	if _, err := s.historyStore.GetPublicServiceAttempt(ctx, strings.TrimSpace(attemptID), filter); err != nil {
		return nil, err
	}
	s.publicServiceMu.Lock()
	runtime := s.publicServiceActive[attemptID]
	if runtime != nil {
		changed, updateErr := s.historyStore.RequestPublicServiceCancellation(context.Background(), attemptID)
		if updateErr != nil {
			s.publicServiceMu.Unlock()
			return nil, updateErr
		}
		if changed {
			runtime.cancel()
		}
	}
	s.publicServiceMu.Unlock()
	return s.GetWorkbenchPublicServiceAttempt(ctx, attemptID, query)
}

func (s *AppService) RetrySaveWorkbenchPublicServiceTest(ctx context.Context, attemptID string, query WorkbenchPublicServiceHistoryQuery) (*history.PublicServiceAttempt, error) {
	if s.historyStore == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	filter, err := publicServiceHistoryFilter(query)
	if err != nil {
		return nil, err
	}
	attemptID = strings.TrimSpace(attemptID)
	if _, err := s.historyStore.GetPublicServiceAttempt(ctx, attemptID, filter); err != nil {
		return nil, err
	}
	s.publicServiceMu.Lock()
	if s.publicServiceActive[attemptID] != nil {
		s.publicServiceMu.Unlock()
		return nil, fmt.Errorf("检测或保存仍在进行")
	}
	s.publicServiceMu.Unlock()
	current, err := s.historyStore.GetPublicServiceAttempt(ctx, attemptID, filter)
	if err != nil {
		return nil, err
	}
	if current.PersistenceState == "saved" {
		return current, nil
	}
	if current.PersistenceState != "failed" || current.Result == nil {
		return nil, monitor.NewValidationError("没有可重试保存的暂存结果")
	}
	if err := s.saveWorkbenchPublicServiceResult(attemptID); err != nil {
		_ = s.historyStore.MarkPublicServiceSaveFailed(context.Background(), attemptID,
			"结果仍已暂存；保存失败，可沿用原 attempt 重试")
		return nil, fmt.Errorf("公共服务结果保存失败；可沿用原 attempt 重试")
	}
	return s.historyStore.GetPublicServiceAttempt(ctx, attemptID, filter)
}

func (s *AppService) GetWorkbenchPublicServiceAttempt(ctx context.Context, attemptID string, query WorkbenchPublicServiceHistoryQuery) (*history.PublicServiceAttempt, error) {
	if s.historyStore == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	filter, err := publicServiceHistoryFilter(query)
	if err != nil {
		return nil, err
	}
	return s.historyStore.GetPublicServiceAttempt(ctx, strings.TrimSpace(attemptID), filter)
}

func (s *AppService) ListWorkbenchPublicServiceTests(ctx context.Context, query WorkbenchPublicServiceHistoryQuery) (WorkbenchPublicServiceHistoryResult, error) {
	if s.historyStore == nil {
		return WorkbenchPublicServiceHistoryResult{}, fmt.Errorf("history store is not initialized")
	}
	filter, err := publicServiceHistoryFilter(query)
	if err != nil {
		return WorkbenchPublicServiceHistoryResult{}, err
	}
	since, until, err := normalizeWorkbenchLatencyWindow(query.Since, query.Until)
	if err != nil {
		return WorkbenchPublicServiceHistoryResult{}, monitor.NewValidationError(err.Error())
	}
	page, err := s.historyStore.QueryPublicServiceAttempts(ctx, filter)
	if err != nil {
		return WorkbenchPublicServiceHistoryResult{}, err
	}
	result := WorkbenchPublicServiceHistoryResult{Attempts: page.Attempts, HasMore: page.HasMore, Complete: !page.HasMore}
	if since != nil {
		result.Since = *since
		result.Until = *until
	}
	return result, nil
}

func (s *AppService) reconcileWorkbenchPublicServiceAttempts() error {
	if s.historyStore == nil {
		return nil
	}
	return s.historyStore.ReconcilePublicServiceAttempts(context.Background())
}

func (s *AppService) beginPublicServiceStorageTransition() error {
	s.publicServiceMu.Lock()
	defer s.publicServiceMu.Unlock()
	if s.publicServiceClosed || s.publicServiceTransition {
		return fmt.Errorf("公共服务检测正在关闭或切换存储")
	}
	if len(s.publicServiceActive) > 0 {
		return fmt.Errorf("请先完成或取消正在运行的公共服务检测")
	}
	s.publicServiceTransition = true
	return nil
}

func (s *AppService) endPublicServiceStorageTransition() {
	s.publicServiceMu.Lock()
	s.publicServiceTransition = false
	s.publicServiceMu.Unlock()
}

func publicServiceRuleSnapshot(rule publicservice.Rule, timeoutSeconds int64) history.PublicServiceRuleSnapshot {
	return history.PublicServiceRuleSnapshot{
		ServiceID:        rule.ServiceID,
		Name:             rule.Name,
		RuleVersion:      rule.RuleVersion,
		TargetURL:        rule.TargetURL,
		Method:           rule.Method,
		SuccessCriterion: rule.SuccessCriterion,
		RedirectPolicy:   rule.RedirectPolicy,
		TimeoutSeconds:   timeoutSeconds,
		MaximumBodyBytes: int64(rule.MaximumBodyBytes),
		Accept:           rule.Accept,
		APIVersionHeader: rule.APIVersionHeader,
	}
}

func publicServiceHistoryFilter(query WorkbenchPublicServiceHistoryQuery) (history.PublicServiceFilter, error) {
	filter := history.PublicServiceFilter{
		ProfileID: strings.TrimSpace(query.ProfileID), NodeKey: strings.TrimSpace(query.NodeKey),
		NodeIdentityKey: strings.TrimSpace(query.NodeIdentityKey), ConfigRevisionKey: strings.TrimSpace(query.ConfigRevisionKey),
		ServiceID: strings.TrimSpace(query.ServiceID), Since: query.Since, Until: query.Until,
		Limit: query.Limit, BeforeAt: query.BeforeAt, BeforeAttemptID: strings.TrimSpace(query.BeforeAttemptID),
	}
	if filter.ProfileID == "" || filter.NodeKey == "" || filter.NodeIdentityKey == "" || filter.ConfigRevisionKey == "" {
		return history.PublicServiceFilter{}, monitor.NewValidationError("服务历史查询必须包含 profile、node_key、稳定身份和配置 revision")
	}
	if filter.ServiceID != "" {
		if _, ok := publicservice.RuleFor(filter.ServiceID); !ok {
			return history.PublicServiceFilter{}, monitor.NewValidationError("service_id 不在固定目录中")
		}
	}
	return filter, nil
}
