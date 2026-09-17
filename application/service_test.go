package application

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/faceair/clash-speedtest/core/history"
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
