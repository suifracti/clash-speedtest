package application

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/controller"
	"github.com/faceair/clash-speedtest/core/history"
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

