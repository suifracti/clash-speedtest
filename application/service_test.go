package application

import (
	"bytes"
	"context"
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

	// Trigger immediate monitor run
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

