package application

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/controller"
	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/policy"
	"github.com/faceair/clash-speedtest/core/profiles"
)

func TestAppServiceInitialization(t *testing.T) {
	tmpDir := t.TempDir()
	hStore, err := history.NewStore(filepath.Join(tmpDir, "history"))
	if err != nil {
		t.Fatalf("create history store: %v", err)
	}

	paths := profiles.Paths{
		Dir: filepath.Join(tmpDir, "profiles"),
	}
	_ = os.MkdirAll(paths.Dir, 0o755)

	emitter := NewMemoryEventEmitter()
	var received []Event
	emitter.Subscribe(func(e Event) {
		received = append(received, e)
	})

	svc := NewAppService(hStore, paths, emitter)
	defer svc.Close()

	// Verify initial status
	st := svc.Status()
	if st.IsRunning {
		t.Errorf("expected IsRunning false on init")
	}

	// Test Token set and event emission
	svc.SetAntigravityToken("test-token-12345", "test_source")
	if svc.GetAntigravityToken() != "test-token-12345" {
		t.Errorf("expected token test-token-12345, got %s", svc.GetAntigravityToken())
	}
	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	if received[0].Type != "antigravity_token_updated" {
		t.Errorf("expected event type antigravity_token_updated, got %s", received[0].Type)
	}

	// Test Token status preview
	statusDTO := svc.GetTokenStatus()
	if !statusDTO.HasToken {
		t.Errorf("expected HasToken true")
	}
	if statusDTO.Source != "test_source" {
		t.Errorf("expected source test_source, got %s", statusDTO.Source)
	}

	// Test Stop when not running
	svc.Stop()
	if len(received) != 2 {
		t.Fatalf("expected 2 events after Stop, got %d", len(received))
	}
	if received[1].Type != "test_stopped" {
		t.Errorf("expected test_stopped event, got %s", received[1].Type)
	}
}

func TestParseMetricSlice(t *testing.T) {
	metrics := []string{"Latency", "Antigravity"}
	set := ParseMetricSlice(metrics)
	if !set.Latency || !set.Antigravity || set.Download || set.Upload {
		t.Errorf("ParseMetricSlice produced unexpected set: %+v", set)
	}

	defaultSet := ParseMetricSlice(nil)
	if !defaultSet.Latency || !defaultSet.Download {
		t.Errorf("ParseMetricSlice default produced unexpected set: %+v", defaultSet)
	}
}

func TestExportClashYAML(t *testing.T) {
	proxies := []map[string]any{
		{"name": "Node-1", "type": "ss", "server": "1.1.1.1", "port": 8388},
		{"name": "Node-2", "type": "vmess", "server": "2.2.2.2", "port": 443},
	}

	yamlBytes, err := ExportClashYAML(proxies)
	if err != nil {
		t.Fatalf("ExportClashYAML error: %v", err)
	}

	yamlStr := string(yamlBytes)
	if len(yamlStr) == 0 {
		t.Fatalf("expected non-empty yaml")
	}
	if !containsStr(yamlStr, "Node-1") || !containsStr(yamlStr, "Node-2") {
		t.Errorf("expected proxy names in yaml, got:\n%s", yamlStr)
	}
	if !containsStr(yamlStr, "proxies:") {
		t.Errorf("expected proxies list in yaml, got:\n%s", yamlStr)
	}
}

func containsStr(s, substr string) bool {
	return filepath.Clean(s) != "" && len(s) >= len(substr) && (s == substr || len(s) > 0 && searchStr(s, substr))
}

func searchStr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

type testMockController struct {
	selectedNode string
	selectCalls  int
}

func (m *testMockController) GetVersion(ctx context.Context) (controller.VersionInfo, error) {
	return controller.VersionInfo{Version: "v1.18.5", CoreType: "mihomo"}, nil
}

func (m *testMockController) GetCapabilities(ctx context.Context) (controller.Capabilities, error) {
	return controller.Capabilities{CanSelectNode: true, CanTestDelay: true}, nil
}

func (m *testMockController) ListGroups(ctx context.Context) ([]controller.Group, error) {
	return []controller.Group{
		{Name: "PROXY", Type: "Selector", Now: m.selectedNode, All: []string{"HK-01", "SG-01"}},
	}, nil
}

func (m *testMockController) ListNodes(ctx context.Context, group string) ([]controller.Node, error) {
	return []controller.Node{
		{Name: "HK-01", Type: "ss"},
		{Name: "SG-01", Type: "vmess"},
	}, nil
}

func (m *testMockController) GetCurrentSelection(ctx context.Context, group string) (string, error) {
	return m.selectedNode, nil
}

func (m *testMockController) SelectNode(ctx context.Context, group string, nodeName string) error {
	m.selectCalls++
	m.selectedNode = nodeName
	return nil
}

func (m *testMockController) TestDelay(ctx context.Context, proxyName string, url string, timeout time.Duration) (time.Duration, error) {
	return 35 * time.Millisecond, nil
}

func TestAppService_ControllerAndPolicy(t *testing.T) {
	tmpDir := t.TempDir()
	hStore, _ := history.NewStore(filepath.Join(tmpDir, "history"))
	paths := profiles.Paths{Dir: filepath.Join(tmpDir, "profiles")}

	emitter := NewMemoryEventEmitter()
	var events []Event
	emitter.Subscribe(func(e Event) {
		events = append(events, e)
	})

	svc := NewAppService(hStore, paths, emitter)
	defer svc.Close()

	mockCtrl := &testMockController{selectedNode: "HK-01"}
	svc.SetController(mockCtrl, ControllerConfigDTO{Endpoint: "http://127.0.0.1:9090", Mode: "external"})

	ctx := context.Background()

	// 1. GetControllerStatus
	st, err := svc.GetControllerStatus(ctx)
	if err != nil {
		t.Fatalf("GetControllerStatus failed: %v", err)
	}
	if !st.Connected || st.CoreVersion != "v1.18.5" || st.CurrentNode != "HK-01" {
		t.Fatalf("unexpected controller status: %+v", st)
	}

	// 2. ListControllerGroups
	groups, err := svc.ListControllerGroups(ctx)
	if err != nil || len(groups) != 1 {
		t.Fatalf("ListControllerGroups failed: %v", err)
	}

	// 3. SelectControllerNode
	if err := svc.SelectControllerNode(ctx, "PROXY", "SG-01"); err != nil {
		t.Fatalf("SelectControllerNode failed: %v", err)
	}
	if mockCtrl.selectedNode != "SG-01" {
		t.Fatalf("expected selectedNode SG-01, got %s", mockCtrl.selectedNode)
	}

	// 4. Policy manipulation
	pol, err := svc.GetSwitchPolicy(ctx)
	if err != nil {
		t.Fatalf("GetSwitchPolicy failed: %v", err)
	}
	pol.Mode = policy.ModeAuto
	pol.TargetGroup = "PROXY"
	pol.MinImprovementRTT = 10 * time.Millisecond
	pol.MinImprovementRatio = 0.10
	pol.CooldownDuration = 0 // Disable cooldown for test
	pol.MaxSampleAge = 0     // Disable freshness check for test
	pol.MinSampleCount = 0
	pol.MinObservationWindow = 0
	pol.RollbackOnFailure = false // Test switch only in this unit test

	if err := svc.UpdateSwitchPolicy(ctx, pol); err != nil {
		t.Fatalf("UpdateSwitchPolicy failed: %v", err)
	}

	// 5. EvaluateAndAutoSwitch
	evals := []policy.NodeEvaluation{
		{Name: "SG-01", Available: true, RTT: 100 * time.Millisecond},
		{Name: "HK-01", Available: true, RTT: 40 * time.Millisecond}, // Significant improvement
	}

	res, err := svc.EvaluateAndAutoSwitch(ctx, evals)
	if err != nil {
		t.Fatalf("EvaluateAndAutoSwitch error: %v", err)
	}
	if !res.ShouldSwitch || res.TargetNode != "HK-01" {
		t.Fatalf("expected switch to HK-01, got %+v", res)
	}
	if mockCtrl.selectedNode != "HK-01" {
		t.Fatalf("expected controller node switched to HK-01, got %s", mockCtrl.selectedNode)
	}

	// 6. Audit Trail
	audit, err := svc.GetSwitchAuditTrail(ctx)
	if err != nil || len(audit) < 2 {
		t.Fatalf("expected at least 2 switch events in audit trail, got %d, err: %v", len(audit), err)
	}
}

func TestAppService_OrchestratorModeHardGuards(t *testing.T) {
	tmpDir := t.TempDir()
	hStore, _ := history.NewStore(filepath.Join(tmpDir, "history"))
	paths := profiles.Paths{Dir: filepath.Join(tmpDir, "profiles")}
	emitter := NewMemoryEventEmitter()

	svc := NewAppService(hStore, paths, emitter)
	defer svc.Close()
	mockCtrl := &testMockController{selectedNode: "HK-01"}
	svc.SetController(mockCtrl, ControllerConfigDTO{Endpoint: "http://127.0.0.1:9090", Mode: "external"})

	ctx := context.Background()
	evalsFailure := []policy.NodeEvaluation{
		{Name: "HK-01", Available: false, TriageStatus: "failed"},
		{Name: "HK-FAST", Available: true, RTT: 25 * time.Millisecond, SampleCount: 5, LastSampleTime: time.Now(), ObservationWindow: 2 * time.Minute},
	}

	// 1. Monitor Only: MUST NOT call SelectNode
	polMon := policy.DefaultSwitchPolicy()
	polMon.Mode = policy.ModeMonitorOnly
	polMon.TargetGroup = "PROXY"
	polMon.LockedNode = "HK-LOCKED" // Even with locked node!
	_ = svc.UpdateSwitchPolicy(ctx, polMon)

	resMon, err := svc.EvaluateAndAutoSwitch(ctx, evalsFailure)
	if err != nil {
		t.Fatalf("EvaluateAndAutoSwitch monitor error: %v", err)
	}
	if resMon.ShouldSwitch {
		t.Errorf("expected ShouldSwitch=false in Monitor Only mode")
	}
	if mockCtrl.selectCalls != 0 {
		t.Errorf("Monitor Only mode MUST NEVER call SelectNode (got %d calls)", mockCtrl.selectCalls)
	}

	// 2. Recommend: MUST NOT call SelectNode, but produces recommendation
	polRec := policy.DefaultSwitchPolicy()
	polRec.Mode = policy.ModeRecommend
	polRec.TargetGroup = "PROXY"
	polRec.CooldownDuration = 0
	_ = svc.UpdateSwitchPolicy(ctx, polRec)

	resRec, err := svc.EvaluateAndAutoSwitch(ctx, evalsFailure)
	if err != nil {
		t.Fatalf("EvaluateAndAutoSwitch recommend error: %v", err)
	}
	if resRec.ShouldSwitch {
		t.Errorf("expected ShouldSwitch=false in Recommend mode")
	}
	if resRec.Recommendation == nil || resRec.Recommendation.TargetNode != "HK-FAST" {
		t.Errorf("expected recommendation for HK-FAST, got %+v", resRec.Recommendation)
	}
	if mockCtrl.selectCalls != 0 {
		t.Errorf("Recommend mode MUST NEVER call SelectNode without user confirmation (got %d calls)", mockCtrl.selectCalls)
	}
}

func TestAppService_StateSyncAndManualOverride(t *testing.T) {
	tmpDir := t.TempDir()
	hStore, _ := history.NewStore(filepath.Join(tmpDir, "history"))
	paths := profiles.Paths{Dir: filepath.Join(tmpDir, "profiles")}
	emitter := NewMemoryEventEmitter()

	svc := NewAppService(hStore, paths, emitter)
	defer svc.Close()
	mockCtrl := &testMockController{selectedNode: "HK-01"}
	svc.SetController(mockCtrl, ControllerConfigDTO{Endpoint: "http://127.0.0.1:9090", Mode: "external"})

	ctx := context.Background()

	// Scenario A: Startup without calling GetControllerStatus
	// Verify that EvaluateAndAutoSwitch actively syncs CurrentNode from controller.
	evalsA := []policy.NodeEvaluation{
		{Name: "HK-01", Available: true, RTT: 100 * time.Millisecond},
		{Name: "HK-FAST", Available: true, RTT: 30 * time.Millisecond},
	}

	pol := policy.DefaultSwitchPolicy()
	pol.Mode = policy.ModeAuto
	pol.TargetGroup = "PROXY"
	pol.CooldownDuration = 0
	pol.MinImprovementRTT = 10 * time.Millisecond
	pol.MinImprovementRatio = 0.10
	pol.MaxSampleAge = 0
	pol.MinSampleCount = 0
	pol.MinObservationWindow = 0
	pol.RollbackOnFailure = false
	_ = svc.UpdateSwitchPolicy(ctx, pol)

	resA, err := svc.EvaluateAndAutoSwitch(ctx, evalsA)
	if err != nil {
		t.Fatalf("EvaluateAndAutoSwitch startup sync error: %v", err)
	}
	if !resA.ShouldSwitch || resA.TargetNode != "HK-FAST" {
		t.Fatalf("expected switch to HK-FAST on startup evaluation, got %+v", resA)
	}
	if mockCtrl.selectedNode != "HK-FAST" {
		t.Fatalf("expected controller node to be HK-FAST, got %s", mockCtrl.selectedNode)
	}

	// Scenario B: External manual switch by user in Clash Verge / external core
	// User manually switches active node to "SG-MANUAL" outside our app
	mockCtrl.selectedNode = "SG-MANUAL"
	callsBefore := mockCtrl.selectCalls

	evalsB := []policy.NodeEvaluation{
		{Name: "SG-MANUAL", Available: true, RTT: 120 * time.Millisecond},
		{Name: "HK-FAST", Available: true, RTT: 30 * time.Millisecond},
	}

	// Run EvaluateAndAutoSwitch: it MUST detect external manual switch and SUPPRESS auto-switch this round!
	resB, err := svc.EvaluateAndAutoSwitch(ctx, evalsB)
	if err != nil {
		t.Fatalf("EvaluateAndAutoSwitch manual override error: %v", err)
	}
	if resB.ShouldSwitch {
		t.Errorf("must suppress auto reverse switch when external manual change is detected")
	}
	if mockCtrl.selectCalls != callsBefore {
		t.Errorf("must not call SelectNode when external manual switch is detected")
	}
	if mockCtrl.selectedNode != "SG-MANUAL" {
		t.Errorf("expected selected node to stay SG-MANUAL, got %s", mockCtrl.selectedNode)
	}
}

type serviceTestMockDialer struct{}

func (d *serviceTestMockDialer) CreateClient(node monitor.MonitoredNode, timeout time.Duration) (*http.Client, error) {
	return &http.Client{
		Timeout: timeout,
		Transport: &serviceTestRoundTripper{},
	}, nil
}

type serviceTestRoundTripper struct{}

func (rt *serviceTestRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: 204,
		Body:       io.NopCloser(bytes.NewReader(nil)),
		Header:     make(http.Header),
	}, nil
}

func TestAppService_MonitorJobLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	hStore, err := history.NewStore(filepath.Join(tmpDir, "history"))
	if err != nil {
		t.Fatalf("create history store: %v", err)
	}

	paths := profiles.Paths{Dir: filepath.Join(tmpDir, "profiles")}
	emitter := NewMemoryEventEmitter()

	svc := NewAppService(hStore, paths, emitter)
	defer svc.Close()

	// Inject custom runner with mock dialer
	runner := monitor.NewRunner(monitor.RunnerConfig{
		Store:  hStore,
		Dialer: &serviceTestMockDialer{},
	})
	svc.SetMonitorRunner(runner)

	jobReq := monitor.MonitorJob{
		ID:       "test_job_1",
		Name:     "Test Job",
		ProbeSet: monitor.ProbeSetLight,
		Interval: 10 * time.Minute,
		Nodes: []monitor.MonitoredNode{
			{
				DisplayName: "HK Node 1",
				Type:        "ss",
				Server:      "1.1.1.1",
				Port:        8388,
				RawConfig: map[string]any{
					"type":     "ss",
					"server":   "1.1.1.1",
					"port":     8388,
					"password": "secret_password",
				},
			},
		},
	}

	// 1. CreateMonitorJob
	created, err := svc.CreateMonitorJob(jobReq)
	if err != nil {
		t.Fatalf("CreateMonitorJob failed: %v", err)
	}
	if created.ID != "test_job_1" {
		t.Fatalf("expected job ID test_job_1, got %s", created.ID)
	}
	if len(created.Nodes) != 1 || created.Nodes[0].NodeKey == "" {
		t.Fatalf("expected node key to be generated, got %+v", created.Nodes)
	}
	// Verify plaintext password is not in NodeKey
	if strings.Contains(created.Nodes[0].NodeKey, "secret_password") {
		t.Fatalf("NodeKey contains sensitive password: %s", created.Nodes[0].NodeKey)
	}

	// 2. StartMonitorJob
	if err := svc.StartMonitorJob("test_job_1"); err != nil {
		t.Fatalf("StartMonitorJob failed: %v", err)
	}
	job, err := svc.GetMonitorJob("test_job_1")
	if err != nil || job.State != monitor.JobStateRunning {
		t.Fatalf("expected running state, got err: %v, job: %+v", err, job)
	}

	// 3. PauseMonitorJob
	if err := svc.PauseMonitorJob("test_job_1"); err != nil {
		t.Fatalf("PauseMonitorJob failed: %v", err)
	}
	job, err = svc.GetMonitorJob("test_job_1")
	if err != nil || job.State != monitor.JobStatePaused {
		t.Fatalf("expected paused state, got err: %v, job: %+v", err, job)
	}

	// 4. ResumeMonitorJob
	if err := svc.ResumeMonitorJob("test_job_1"); err != nil {
		t.Fatalf("ResumeMonitorJob failed: %v", err)
	}
	job, err = svc.GetMonitorJob("test_job_1")
	if err != nil || job.State != monitor.JobStateRunning {
		t.Fatalf("expected running state, got err: %v, job: %+v", err, job)
	}

	// 5. Trigger Immediate Run
	run, err := svc.TriggerMonitorJob("test_job_1")
	if err != nil {
		t.Fatalf("TriggerMonitorJob failed: %v", err)
	}
	if run == nil || run.Status != monitor.RunStatusCompleted {
		t.Fatalf("expected completed run, got %+v", run)
	}

	// 6. StopMonitorJob
	if err := svc.StopMonitorJob("test_job_1"); err != nil {
		t.Fatalf("StopMonitorJob failed: %v", err)
	}
	job, err = svc.GetMonitorJob("test_job_1")
	if err != nil || job.State != monitor.JobStateStopped {
		t.Fatalf("expected stopped state, got err: %v, job: %+v", err, job)
	}

	// 7. Verify SQLite Persistence
	ctx := context.Background()
	runs, err := svc.QueryMonitorRuns(ctx, "test_job_1", 10)
	if err != nil {
		t.Fatalf("QueryMonitorRuns failed: %v", err)
	}
	if len(runs) < 1 {
		t.Fatalf("expected at least 1 run in SQLite, got %d", len(runs))
	}

	samples, err := svc.QueryMonitorSamples(ctx, monitor.SampleFilter{NodeKey: created.Nodes[0].NodeKey})
	if err != nil {
		t.Fatalf("QueryMonitorSamples failed: %v", err)
	}
	if len(samples) < 1 {
		t.Fatalf("expected at least 1 sample in SQLite, got %d", len(samples))
	}
	if !samples[0].Success {
		t.Fatalf("expected successful sample, got %+v", samples[0])
	}
}

func TestAppService_MonitorZeroSelectNode(t *testing.T) {
	tmpDir := t.TempDir()
	hStore, _ := history.NewStore(filepath.Join(tmpDir, "history"))
	paths := profiles.Paths{Dir: filepath.Join(tmpDir, "profiles")}
	emitter := NewMemoryEventEmitter()

	svc := NewAppService(hStore, paths, emitter)
	defer svc.Close()

	mockCtrl := &testMockController{selectedNode: "HK-01"}
	svc.SetController(mockCtrl, ControllerConfigDTO{Endpoint: "http://127.0.0.1:9090", Mode: "external"})

	runner := monitor.NewRunner(monitor.RunnerConfig{
		Store:  hStore,
		Dialer: &serviceTestMockDialer{},
	})
	svc.SetMonitorRunner(runner)

	jobReq := monitor.MonitorJob{
		ID:       "zero_select_job",
		Name:     "Zero Select Job",
		ProbeSet: monitor.ProbeSetLight,
		Interval: 1 * time.Hour,
		Nodes: []monitor.MonitoredNode{
			{
				DisplayName: "Target Node",
				Type:        "ss",
				Server:      "1.1.1.1",
				Port:        8388,
				RawConfig: map[string]any{
					"type":   "ss",
					"server": "1.1.1.1",
					"port":   8388,
				},
			},
		},
	}

	_, err := svc.CreateMonitorJob(jobReq)
	if err != nil {
		t.Fatalf("CreateMonitorJob failed: %v", err)
	}

	// 1. Triggering a stopped job must return an error
	_, err = svc.TriggerMonitorJob("zero_select_job")
	if err == nil {
		t.Fatalf("Expected TriggerMonitorJob to fail on stopped job")
	}

	// 2. Start job
	if err := svc.StartMonitorJob("zero_select_job"); err != nil {
		t.Fatalf("StartMonitorJob failed: %v", err)
	}

	// 3. Trigger immediate monitor run
	_, err = svc.TriggerMonitorJob("zero_select_job")
	if err != nil {
		t.Fatalf("TriggerMonitorJob failed: %v", err)
	}

	// PROOF: Zero SelectNode calls!
	if mockCtrl.selectCalls != 0 {
		t.Fatalf("CRITICAL ARCHITECTURE VIOLATION: Monitor must NEVER call Controller.SelectNode! Found %d calls", mockCtrl.selectCalls)
	}
	if mockCtrl.selectedNode != "HK-01" {
		t.Fatalf("Selected node changed unexpectedly: %s", mockCtrl.selectedNode)
	}
}

func TestAppService_CreateDuplicateRunningJobFails(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "app_service_dup_test_*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	hStore, err := history.NewStore(filepath.Join(tmpDir, "history"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer hStore.Close()

	paths := profiles.Paths{Dir: filepath.Join(tmpDir, "profiles")}
	emitter := NewMemoryEventEmitter()

	svc := NewAppService(hStore, paths, emitter)
	defer svc.Close()

	runner := monitor.NewRunner(monitor.RunnerConfig{
		Store:  hStore,
		Dialer: &serviceTestMockDialer{},
	})
	svc.SetMonitorRunner(runner)

	jobReq := monitor.MonitorJob{
		ID:       "dup_test_job",
		Name:     "Duplicate Test Job",
		ProbeSet: monitor.ProbeSetLight,
		Interval: 1 * time.Hour,
		Nodes: []monitor.MonitoredNode{
			{
				DisplayName: "Node A",
				Type:        "ss",
				Server:      "1.1.1.1",
				Port:        8388,
			},
		},
	}

	// 1. Initial creation succeeds
	_, err = svc.CreateMonitorJob(jobReq)
	if err != nil {
		t.Fatalf("Initial CreateMonitorJob failed: %v", err)
	}

	// 2. Creating with same ID while initial job is STOPPED -> must fail
	_, err = svc.CreateMonitorJob(jobReq)
	if err == nil {
		t.Fatalf("Expected duplicate CreateMonitorJob to fail when job is stopped")
	}

	// 3. Start the job
	if err := svc.StartMonitorJob("dup_test_job"); err != nil {
		t.Fatalf("StartMonitorJob failed: %v", err)
	}

	// 4. Creating with same ID while job is RUNNING -> must fail
	_, err = svc.CreateMonitorJob(jobReq)
	if err == nil {
		t.Fatalf("Expected duplicate CreateMonitorJob to fail when job is running")
	}

	// 5. Pause the job
	if err := svc.PauseMonitorJob("dup_test_job"); err != nil {
		t.Fatalf("PauseMonitorJob failed: %v", err)
	}

	// 6. Creating with same ID while job is PAUSED -> must fail
	_, err = svc.CreateMonitorJob(jobReq)
	if err == nil {
		t.Fatalf("Expected duplicate CreateMonitorJob to fail when job is paused")
	}

	// 7. Stop the job
	if err := svc.StopMonitorJob("dup_test_job"); err != nil {
		t.Fatalf("StopMonitorJob failed: %v", err)
	}

	// 8. Creating with same ID while job is back to STOPPED -> must STILL fail
	_, err = svc.CreateMonitorJob(jobReq)
	if err == nil {
		t.Fatalf("Expected duplicate CreateMonitorJob to fail when job is stopped again")
	}
}

func TestAppService_MonitorHistoryAPIs(t *testing.T) {
	tmpDir := t.TempDir()
	hStore, err := history.NewStore(filepath.Join(tmpDir, "history"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer hStore.Close()

	paths := profiles.Paths{Dir: filepath.Join(tmpDir, "profiles")}
	emitter := NewMemoryEventEmitter()

	var retentionEvents []Event
	emitter.Subscribe(func(e Event) {
		if e.Type == "monitor_retention_applied" {
			retentionEvents = append(retentionEvents, e)
		}
	})

	svc := NewAppService(hStore, paths, emitter)
	defer svc.Close()

	mockCtrl := &testMockController{selectedNode: "DEFAULT_NODE"}
	svc.SetController(mockCtrl, ControllerConfigDTO{Endpoint: "http://127.0.0.1:9090", Mode: "external"})

	runner := monitor.NewRunner(monitor.RunnerConfig{
		Store:  hStore,
		Dialer: &serviceTestMockDialer{},
	})
	svc.SetMonitorRunner(runner)

	jobReq := monitor.MonitorJob{
		ID:       "history_test_job",
		Name:     "History Test Job",
		ProbeSet: monitor.ProbeSetLight,
		Interval: 1 * time.Hour,
		Nodes: []monitor.MonitoredNode{
			{
				DisplayName: "Node Alpha",
				Type:        "ss",
				Server:      "2.2.2.2",
				Port:        8388,
				RawConfig: map[string]any{
					"type":     "ss",
					"server":   "2.2.2.2",
					"port":     8388,
					"password": "secret_pwd",
				},
			},
		},
	}

	created, err := svc.CreateMonitorJob(jobReq)
	if err != nil {
		t.Fatalf("CreateMonitorJob failed: %v", err)
	}

	// Verify identity keys were automatically populated
	if created.Nodes[0].NodeIdentityKey == "" {
		t.Errorf("expected NodeIdentityKey to be populated on MonitoredNode")
	}
	if created.Nodes[0].ConfigRevisionKey == "" {
		t.Errorf("expected ConfigRevisionKey to be populated on MonitoredNode")
	}

	// Start & trigger one run to produce a raw sample
	if err := svc.StartMonitorJob("history_test_job"); err != nil {
		t.Fatalf("StartMonitorJob failed: %v", err)
	}
	run, err := svc.TriggerMonitorJob("history_test_job")
	if err != nil || run.Status != monitor.RunStatusCompleted {
		t.Fatalf("TriggerMonitorJob failed: run=%+v, err=%v", run, err)
	}

	ctx := context.Background()

	// 1. Test QueryMonitorSamplesCursor
	page, err := svc.QueryMonitorSamplesCursor(ctx, monitor.CursorFilter{
		NodeIdentityKey: created.Nodes[0].NodeIdentityKey,
		Limit:           10,
	})
	if err != nil {
		t.Fatalf("QueryMonitorSamplesCursor failed: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected 1 item in cursor page, got %d", len(page.Items))
	}
	if page.Items[0].NodeIdentityKey != created.Nodes[0].NodeIdentityKey {
		t.Errorf("mismatched NodeIdentityKey in sample: %s", page.Items[0].NodeIdentityKey)
	}

	// 2. Test GetMonitorStats
	stats, err := svc.GetMonitorStats(ctx, monitor.StatsQuery{
		NodeIdentityKey: created.Nodes[0].NodeIdentityKey,
	})
	if err != nil {
		t.Fatalf("GetMonitorStats failed: %v", err)
	}
	if stats.SampleCount != 1 || stats.SuccessCount != 1 || stats.SuccessRate != 1.0 {
		t.Errorf("unexpected stats: %+v", stats)
	}

	// 3. Test ApplyRetention (KeepAll by default)
	retRes, err := svc.ApplyRetention(ctx, monitor.RetentionRequest{
		Policy: monitor.RetentionKeepAll,
	})
	if err != nil {
		t.Fatalf("ApplyRetention failed: %v", err)
	}
	if retRes.SamplesDeleted != 0 {
		t.Errorf("expected 0 samples deleted with KeepAll, got %d", retRes.SamplesDeleted)
	}
	if len(retentionEvents) != 1 {
		t.Errorf("expected 1 monitor_retention_applied event emitted, got %d", len(retentionEvents))
	}

	// 4. Hard Line of Defense: Zero calls to SelectNode throughout
	if mockCtrl.selectCalls != 0 {
		t.Fatalf("CRITICAL SECURITY VIOLATION: SelectNode called %d times during monitor history operations", mockCtrl.selectCalls)
	}
}

func TestAppService_Retention_PartialFailureAudit(t *testing.T) {
	// NEW B-05: Verify that partial retention failure emits monitor_retention_partial_failure
	// and strictly does NOT emit monitor_retention_applied.
	tmpDir := t.TempDir()
	hStore, err := history.NewStore(filepath.Join(tmpDir, "history"))
	if err != nil {
		t.Fatalf("create history store: %v", err)
	}

	paths := profiles.Paths{
		Dir: filepath.Join(tmpDir, "profiles"),
	}
	_ = os.MkdirAll(paths.Dir, 0o755)

	emitter := NewMemoryEventEmitter()
	var partialEvents []Event
	var appliedEvents []Event
	emitter.Subscribe(func(e Event) {
		if e.Type == "monitor_retention_partial_failure" {
			partialEvents = append(partialEvents, e)
		}
		if e.Type == "monitor_retention_applied" {
			appliedEvents = append(appliedEvents, e)
		}
	})

	svc := NewAppService(hStore, paths, emitter)
	defer svc.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	cutoff := now.AddDate(0, 0, -30)

	// Seed 1200 samples (> 2 batches of 500)
	var samples []*monitor.MonitorSample
	for i := 0; i < 1200; i++ {
		samples = append(samples, &monitor.MonitorSample{
			SampleID:  fmt.Sprintf("s_app_fault_%04d", i),
			RunID:     "run_fault",
			NodeKey:   "nk_app_fault",
			Timestamp: cutoff.Add(-time.Duration(i+1) * time.Minute),
			Success:   true,
		})
	}
	if err := hStore.SaveMonitorSamples(ctx, samples); err != nil {
		t.Fatalf("save samples failed: %v", err)
	}

	// Trigger fault on batch 3
	hStore.SetTestBatchFailAt(3)

	result, err := svc.ApplyRetention(ctx, monitor.RetentionRequest{
		Policy:     monitor.Retention30d,
		CutoffTime: &cutoff,
	})

	if err == nil {
		t.Fatalf("expected error from injected batch 3 fault, got nil")
	}

	// Assert partial == true and SamplesDeleted == 1000
	if result == nil || !result.Partial {
		t.Errorf("expected result.Partial to be true, got %+v", result)
	}
	if result.SamplesDeleted != 1000 {
		t.Errorf("expected 1000 samples deleted, got %d", result.SamplesDeleted)
	}

	// Assert events
	if len(partialEvents) != 1 {
		t.Fatalf("expected exactly 1 monitor_retention_partial_failure event, got %d", len(partialEvents))
	}
	evt := partialEvents[0]
	payload, ok := evt.Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected payload to be map[string]any, got %T", evt.Payload)
	}
	if payload["partial"] != true {
		t.Errorf("expected payload partial=true, got %v", payload["partial"])
	}
	if payload["samples_deleted"] != int64(1000) {
		t.Errorf("expected payload samples_deleted=1000, got %v", payload["samples_deleted"])
	}
	if payload["error"] == "" {
		t.Errorf("expected payload error string to be populated")
	}

	// CRITICAL: Must NOT emit success event
	if len(appliedEvents) != 0 {
		t.Errorf("VIOLATION: monitor_retention_applied emitted on partial failure! %+v", appliedEvents)
	}
}

func TestAppService_MigrationContinuity_PublicAPI(t *testing.T) {
	// Verify that PR3 legacy samples + PR4 new samples are continuously visible
	// across AppService.QueryMonitorSamplesCursor and AppService.GetMonitorStats.
	tmpDir := t.TempDir()
	ctx := context.Background()
	hDir := filepath.Join(tmpDir, "history")
	_ = os.MkdirAll(hDir, 0o755)
	dbPath := filepath.Join(hDir, "history.db")

	// 1. Manually setup PR#3 schema
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	_, err = rawDB.Exec(`
	CREATE TABLE monitor_samples (
		sample_id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		node_key TEXT NOT NULL,
		profile_id TEXT NOT NULL,
		display_name_snapshot TEXT NOT NULL,
		probe_type TEXT NOT NULL,
		target TEXT NOT NULL,
		timestamp DATETIME NOT NULL,
		success INTEGER NOT NULL,
		latency_ms INTEGER NOT NULL DEFAULT 0,
		ttfb_ms INTEGER NOT NULL DEFAULT 0,
		error_class TEXT NOT NULL DEFAULT 'none',
		error_detail TEXT,
		exit_ip TEXT,
		exit_region TEXT,
		metadata_json TEXT
	);
	CREATE TABLE monitor_runs (
		run_id TEXT PRIMARY KEY,
		job_id TEXT NOT NULL,
		scheduled_at DATETIME NOT NULL,
		started_at DATETIME NOT NULL,
		finished_at DATETIME,
		status TEXT NOT NULL,
		total_nodes INTEGER NOT NULL DEFAULT 0,
		success_nodes INTEGER NOT NULL DEFAULT 0,
		failed_nodes INTEGER NOT NULL DEFAULT 0,
		error_message TEXT
	);
	`)
	if err != nil {
		t.Fatalf("exec pr3 ddl: %v", err)
	}

	// Insert PR#3 legacy sample
	tLegacy := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	_, err = rawDB.Exec(`
		INSERT INTO monitor_samples (
			sample_id, run_id, node_key, profile_id, display_name_snapshot,
			probe_type, target, timestamp, success, latency_ms, ttfb_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "s_app_legacy", "run_pr3", "nk_app_legacy_node", "prof_1", "LegacyNode", "rtt", "https://test.com", tLegacy, 1, 60, 50)
	if err != nil {
		t.Fatalf("insert legacy sample: %v", err)
	}
	_ = rawDB.Close()

	// 2. Open AppService which triggers schema migration
	hStore, err := history.NewStore(hDir)
	if err != nil {
		t.Fatalf("create migrated store: %v", err)
	}
	paths := profiles.Paths{
		Dir: filepath.Join(tmpDir, "profiles"),
	}
	_ = os.MkdirAll(paths.Dir, 0o755)
	emitter := NewMemoryEventEmitter()
	svc := NewAppService(hStore, paths, emitter)
	defer svc.Close()

	// 3. Insert PR#4 new sample
	tPR4 := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	newIdentity := "nid_ss_app_node_8388_abc123"
	err = hStore.SaveMonitorSamples(ctx, []*monitor.MonitorSample{
		{
			SampleID:            "s_app_new",
			RunID:               "run_pr4",
			NodeKey:             "nk_app_legacy_node",
			NodeIdentityKey:     newIdentity,
			ConfigRevisionKey:   "rev_abc123",
			ProfileID:           "prof_1",
			DisplayNameSnapshot: "LegacyNode",
			ProbeType:           "rtt",
			Target:              "https://test.com",
			Timestamp:           tPR4,
			Success:             true,
			Latency:             40 * time.Millisecond,
			TTFB:                30 * time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("save pr4 sample: %v", err)
	}

	// 4. Query via AppService.QueryMonitorSamplesCursor with (NodeIdentityKey + LegacyNodeKey) bridge
	page, err := svc.QueryMonitorSamplesCursor(ctx, monitor.CursorFilter{
		NodeIdentityKey: newIdentity,
		LegacyNodeKey:   "nk_app_legacy_node",
		OrderDesc:       true,
		Limit:           10,
	})
	if err != nil {
		t.Fatalf("QueryMonitorSamplesCursor failed: %v", err)
	}

	if len(page.Items) != 2 {
		t.Fatalf("expected 2 items bridging PR3 and PR4, got %d", len(page.Items))
	}
	if page.Items[0].SampleID != "s_app_new" || page.Items[1].SampleID != "s_app_legacy" {
		t.Errorf("unexpected continuous item sequence: %s, %s", page.Items[0].SampleID, page.Items[1].SampleID)
	}

	// 5. Query via AppService.GetMonitorStats with bridge
	stats, err := svc.GetMonitorStats(ctx, monitor.StatsQuery{
		NodeIdentityKey: newIdentity,
		LegacyNodeKey:   "nk_app_legacy_node",
	})
	if err != nil {
		t.Fatalf("GetMonitorStats failed: %v", err)
	}
	if stats.SampleCount != 2 || stats.SuccessCount != 2 {
		t.Errorf("expected 2 samples in derived stats, got %+v", stats)
	}
}


