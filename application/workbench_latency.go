package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/profiles"
	"github.com/faceair/clash-speedtest/core/speedtester"
)

const (
	WorkbenchLatencyProject            = "latency_stability"
	minWorkbenchLatencyTimeoutSeconds  = int64(1)
	maxWorkbenchLatencyTimeoutSeconds  = int64(120)
	workbenchLatencyPersistenceTimeout = 30 * time.Second
)

// RunWorkbenchLatencyTest executes a single latency test using a stable node
// identity. The measured result is returned and emitted first with
// persistence_state="saving"; the history write completes asynchronously and
// emits a second update for the same attempt_id. This keeps a slow history
// transaction from blocking the user's measured result.
func (s *AppService) RunWorkbenchLatencyTest(ctx context.Context, req WorkbenchLatencyTestRequest) (*WorkbenchLatencyTestDTO, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	profileID := strings.TrimSpace(req.ProfileID)
	nodeKey := strings.TrimSpace(req.NodeKey)
	if profileID == "" {
		return nil, monitor.NewValidationError("profile_id 不能为空")
	}
	if nodeKey == "" {
		return nil, monitor.NewValidationError("node_key 不能为空")
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

	selected, executionName, proxy, err := s.resolveWorkbenchLatencyProxy(profileID, nodeKey)
	if err != nil {
		return nil, err
	}
	if s.historyStore == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}

	requestedAt := time.Now().UTC()
	attemptID := newWorkbenchLatencyAttemptID()
	s.emitter.Emit(Event{
		Type: "workbench_latency_test_started",
		Payload: map[string]any{
			"attempt_id":   attemptID,
			"profile_id":   profileID,
			"node_key":     selected.NodeKey,
			"display_name": selected.DisplayName,
			"test_project": WorkbenchLatencyProject,
		},
	})

	startedAt := time.Now().UTC()
	st, err := speedtester.New(&speedtester.Config{
		ConfigPaths: s.profilePaths.CacheFile(profileID),
		Mode:        speedtester.SpeedModeFast,
		Metrics:     speedtester.MetricSet{Latency: true},
		Concurrent:  1,
		Timeout:     time.Duration(timeoutSeconds) * time.Second,
		Rounds:      1,
	})
	if err != nil {
		return nil, fmt.Errorf("初始化延迟测试引擎失败: %w", err)
	}

	var lastResult *speedtester.Result
	lastResult = st.TestSingle(executionName, proxy, func(result *speedtester.Result) bool {
		lastResult = result
		return ctx.Err() == nil
	})
	finishedAt := time.Now().UTC()
	if lastResult == nil {
		return nil, fmt.Errorf("延迟测试未产生结果")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("延迟测试已取消: %w", err)
	}

	record := buildWorkbenchLatencyRecord(attemptID, profileID, selected, req.TestProject, requestedAt, startedAt, finishedAt, lastResult)
	dto := workbenchLatencyDTO(*record, "saving", "")

	s.emitter.Emit(Event{
		Type:    "workbench_latency_test_completed",
		Payload: dto,
	})
	s.persistWorkbenchLatencyTest(record)
	return &dto, nil
}

func (s *AppService) persistWorkbenchLatencyTest(record *history.LatencyTest) {
	s.latencyPersistenceWG.Add(1)
	go func() {
		defer s.latencyPersistenceWG.Done()

		ctx, cancel := context.WithTimeout(context.Background(), workbenchLatencyPersistenceTimeout)
		defer cancel()

		err := s.saveWorkbenchLatencyTest(ctx, record)
		state := "saved"
		errorMessage := ""
		if err != nil {
			state = "failed"
			errorMessage = err.Error()
		}
		if s.emitter != nil {
			s.emitter.Emit(Event{
				Type:    "workbench_latency_test_persistence_updated",
				Payload: workbenchLatencyDTO(*record, state, errorMessage),
			})
		}
	}()
}

func (s *AppService) saveWorkbenchLatencyTest(ctx context.Context, record *history.LatencyTest) error {
	if s.latencySaveHook != nil {
		return s.latencySaveHook(ctx, record)
	}
	if s.historyStore == nil {
		return fmt.Errorf("history store is not initialized")
	}
	return s.historyStore.SaveLatencyTest(ctx, record)
}

// ListWorkbenchLatencyTests returns one bounded, frozen observation window for
// one profile/node pair. The core query filters raw samples before applying its
// attempt limit, so HasMore is an honest completeness signal.
func (s *AppService) ListWorkbenchLatencyTests(ctx context.Context, query WorkbenchLatencyHistoryQuery) (WorkbenchLatencyHistoryResult, error) {
	profileID := strings.TrimSpace(query.ProfileID)
	nodeKey := strings.TrimSpace(query.NodeKey)
	if profileID == "" || nodeKey == "" {
		return WorkbenchLatencyHistoryResult{}, monitor.NewValidationError("历史查询必须同时提供 profile_id 和 node_key")
	}
	if s.historyStore == nil {
		return WorkbenchLatencyHistoryResult{}, fmt.Errorf("history store is not initialized")
	}
	since, until, err := normalizeWorkbenchLatencyWindow(query.Since, query.Until)
	if err != nil {
		return WorkbenchLatencyHistoryResult{}, monitor.NewValidationError(err.Error())
	}
	page, err := s.historyStore.QueryLatencyTests(ctx, history.LatencyTestFilter{
		ProfileID: profileID,
		NodeKey:   nodeKey,
		Since:     since,
		Until:     until,
		Limit:     query.Limit,
	})
	if err != nil {
		return WorkbenchLatencyHistoryResult{}, err
	}
	result := WorkbenchLatencyHistoryResult{
		Tests:    make([]WorkbenchLatencyTestDTO, 0, len(page.Tests)),
		HasMore:  page.HasMore,
		Complete: !page.HasMore,
	}
	if since != nil {
		result.Since = *since
		result.Until = *until
		result.AsOf = *until
	}
	for _, test := range page.Tests {
		if since != nil {
			projected := projectLatencyTestForWindow(*test)
			result.Tests = append(result.Tests, workbenchLatencyDTO(projected, "saved", ""))
			continue
		}
		result.Tests = append(result.Tests, workbenchLatencyDTO(*test, "saved", ""))
	}
	return result, nil
}

// GetWorkbenchLatencyTest returns one persisted record and all raw samples
// only when its immutable attempt ID belongs to the requested profile/node.
func (s *AppService) GetWorkbenchLatencyTest(ctx context.Context, query WorkbenchLatencyHistoryDetailQuery) (*WorkbenchLatencyTestDTO, error) {
	if s.historyStore == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	profileID := strings.TrimSpace(query.ProfileID)
	nodeKey := strings.TrimSpace(query.NodeKey)
	attemptID := strings.TrimSpace(query.AttemptID)
	if profileID == "" || nodeKey == "" || attemptID == "" {
		return nil, monitor.NewValidationError("历史详情必须同时提供 profile_id、node_key 和 attempt_id")
	}
	since, until, err := normalizeWorkbenchLatencyWindow(query.Since, query.Until)
	if err != nil {
		return nil, monitor.NewValidationError(err.Error())
	}
	var test *history.LatencyTest
	if since != nil {
		test, err = s.historyStore.GetLatencyTestInWindow(ctx, attemptID, since, until)
	} else {
		test, err = s.historyStore.GetLatencyTest(ctx, attemptID)
	}
	if err != nil {
		return nil, err
	}
	if test.ProfileID != profileID || test.NodeKey != nodeKey {
		return nil, monitor.NewValidationError("历史详情不属于当前订阅节点")
	}
	if since != nil {
		projected := projectLatencyTestForWindow(*test)
		dto := workbenchLatencyDTO(projected, "saved", "")
		return &dto, nil
	}
	dto := workbenchLatencyDTO(*test, "saved", "")
	return &dto, nil
}

func normalizeWorkbenchLatencyWindow(since, until *time.Time) (*time.Time, *time.Time, error) {
	if (since == nil) != (until == nil) {
		return nil, nil, fmt.Errorf("历史查询必须同时提供 since 和 until")
	}
	if since == nil {
		return nil, nil, nil
	}
	normalizedSince := since.UTC()
	normalizedUntil := until.UTC()
	if !normalizedSince.Before(normalizedUntil) {
		return nil, nil, fmt.Errorf("历史查询窗口必须满足 since < until")
	}
	return &normalizedSince, &normalizedUntil, nil
}

// projectLatencyTestForWindow makes aggregate fields truthful for the raw
// samples returned by a bounded history query. It is a read projection only;
// persisted measurement fields remain unchanged in SQLite.
func projectLatencyTestForWindow(test history.LatencyTest) history.LatencyTest {
	test.Samples = append([]history.LatencyTestSample(nil), test.Samples...)
	test.TotalSamples = len(test.Samples)
	test.SuccessSamples = 0
	test.FailureSamples = 0
	test.ErrorMessage = ""
	var successful []int64
	for _, sample := range test.Samples {
		if sample.Success {
			test.SuccessSamples++
			successful = append(successful, sample.LatencyMs)
			continue
		}
		test.FailureSamples++
		if test.ErrorMessage == "" {
			test.ErrorMessage = sample.Error
		}
	}
	if test.TotalSamples == 0 || test.SuccessSamples == 0 {
		test.Status = "failed"
		test.LatencyMs = 0
		test.JitterMs = 0
	} else {
		test.Status = "completed"
		if test.FailureSamples > 0 {
			test.Status = "partial_failed"
		}
		var total int64
		for _, latency := range successful {
			total += latency
		}
		test.LatencyMs = total / int64(len(successful))
		var variance float64
		for _, latency := range successful {
			diff := float64(latency - test.LatencyMs)
			variance += diff * diff
		}
		test.JitterMs = int64(math.Sqrt(variance / float64(len(successful))))
	}
	if test.TotalSamples > 0 {
		test.PacketLoss = float64(test.FailureSamples) / float64(test.TotalSamples) * 100
	} else {
		test.PacketLoss = 0
	}
	return test
}

func (s *AppService) resolveWorkbenchLatencyProxy(profileID, nodeKey string) (monitor.MonitoredNode, string, *speedtester.CProxy, error) {
	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return monitor.MonitoredNode{}, "", nil, fmt.Errorf("加载订阅配置失败: %w", err)
	}
	if store.Get(profileID) == nil {
		return monitor.MonitoredNode{}, "", nil, monitor.NewValidationError("订阅不存在或已被移除")
	}

	available, err := s.loadMonitorNodes(profileID)
	if err != nil {
		return monitor.MonitoredNode{}, "", nil, monitor.WrapValidationError(err)
	}
	var selected monitor.MonitoredNode
	found := false
	for _, node := range available {
		if node.NodeKey == nodeKey {
			selected = node
			found = true
			break
		}
	}
	if !found || len(selected.RawConfig) == 0 {
		return monitor.MonitoredNode{}, "", nil, monitor.NewValidationError("所选节点已不存在或订阅配置已变化，请重新加载节点")
	}

	st, err := speedtester.New(&speedtester.Config{
		ConfigPaths: s.profilePaths.CacheFile(profileID),
		Mode:        speedtester.SpeedModeFast,
		Metrics:     speedtester.MetricSet{Latency: true},
		Concurrent:  1,
	})
	if err != nil {
		return monitor.MonitoredNode{}, "", nil, fmt.Errorf("初始化订阅配置失败: %w", err)
	}
	proxies, err := st.LoadProxies()
	if err != nil {
		return monitor.MonitoredNode{}, "", nil, fmt.Errorf("读取订阅配置失败: %w", err)
	}
	for name, proxy := range proxies {
		candidate := monitoredNodeFromProxy(name, proxy)
		if candidate.NodeKey != nodeKey {
			continue
		}
		if candidate.NodeIdentityKey != selected.NodeIdentityKey || candidate.ConfigRevisionKey != selected.ConfigRevisionKey {
			return monitor.MonitoredNode{}, "", nil, monitor.NewValidationError("所选节点配置已变化，请重新加载节点")
		}
		return selected, name, proxy, nil
	}

	return monitor.MonitoredNode{}, "", nil, monitor.NewValidationError("所选节点已不存在或订阅配置已变化，请重新加载节点")
}

func monitoredNodeFromProxy(name string, proxy *speedtester.CProxy) monitor.MonitoredNode {
	if proxy == nil {
		return monitor.MonitoredNode{}
	}
	node := monitor.MonitoredNode{
		DisplayName: name,
		Type:        proxy.Type().String(),
		Server:      configString(proxy.Config, "server"),
		Port:        configInt(proxy.Config, "port"),
		RawConfig:   proxy.Config,
	}
	monitor.PopulateNodeKeys(&node)
	return node
}

func buildWorkbenchLatencyRecord(attemptID, profileID string, node monitor.MonitoredNode, project string, requestedAt, startedAt, finishedAt time.Time, result *speedtester.Result) *history.LatencyTest {
	samples := make([]history.LatencyTestSample, 0, len(result.LatencySamples))
	successCount := 0
	failureCount := 0
	firstError := ""
	for _, sample := range result.LatencySamples {
		if sample.Success {
			successCount++
		} else {
			failureCount++
			if firstError == "" {
				firstError = sample.Error
			}
		}
		samples = append(samples, history.LatencyTestSample{
			Seq:       sample.Seq,
			Timestamp: sample.Timestamp,
			LatencyMs: sample.LatencyMs,
			Success:   sample.Success,
			Error:     sample.Error,
		})
	}

	status := "completed"
	if len(samples) == 0 || successCount == 0 {
		status = "failed"
	} else if failureCount > 0 {
		status = "partial_failed"
	}
	if firstError == "" && result.PacketLoss >= 100 {
		firstError = "所有延迟样本均失败"
	}

	return &history.LatencyTest{
		AttemptID:         attemptID,
		ProfileID:         profileID,
		NodeKey:           node.NodeKey,
		NodeIdentityKey:   node.NodeIdentityKey,
		ConfigRevisionKey: node.ConfigRevisionKey,
		DisplayName:       node.DisplayName,
		NodeType:          node.Type,
		TestProject:       project,
		RequestedAt:       requestedAt,
		StartedAt:         startedAt,
		FinishedAt:        finishedAt,
		Status:            status,
		LatencyMs:         result.Latency.Milliseconds(),
		JitterMs:          result.Jitter.Milliseconds(),
		PacketLoss:        result.PacketLoss,
		TotalSamples:      len(samples),
		SuccessSamples:    successCount,
		FailureSamples:    failureCount,
		ErrorMessage:      firstError,
		Samples:           samples,
	}
}

func workbenchLatencyDTO(test history.LatencyTest, persistenceState, persistenceError string) WorkbenchLatencyTestDTO {
	samples := make([]WorkbenchLatencySampleDTO, 0, len(test.Samples))
	for _, sample := range test.Samples {
		samples = append(samples, WorkbenchLatencySampleDTO{
			Seq:       sample.Seq,
			Timestamp: sample.Timestamp,
			LatencyMs: sample.LatencyMs,
			Success:   sample.Success,
			Error:     sample.Error,
		})
	}
	return WorkbenchLatencyTestDTO{
		AttemptID:         test.AttemptID,
		ProfileID:         test.ProfileID,
		NodeKey:           test.NodeKey,
		NodeIdentityKey:   test.NodeIdentityKey,
		ConfigRevisionKey: test.ConfigRevisionKey,
		DisplayName:       test.DisplayName,
		NodeType:          test.NodeType,
		TestProject:       test.TestProject,
		RequestedAt:       test.RequestedAt,
		StartedAt:         test.StartedAt,
		FinishedAt:        test.FinishedAt,
		Status:            test.Status,
		LatencyMs:         test.LatencyMs,
		JitterMs:          test.JitterMs,
		PacketLoss:        test.PacketLoss,
		TotalSamples:      test.TotalSamples,
		SuccessSamples:    test.SuccessSamples,
		FailureSamples:    test.FailureSamples,
		ErrorMessage:      test.ErrorMessage,
		Samples:           samples,
		PersistenceState:  persistenceState,
		PersistenceError:  persistenceError,
	}
}

func newWorkbenchLatencyAttemptID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return "latency_" + hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("latency_%d", time.Now().UnixNano())
}
