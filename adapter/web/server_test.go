package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/faceair/clash-speedtest/core/profiles"
)

func TestWebServerEndpoints(t *testing.T) {
	tmpDir := t.TempDir()
	profileDir := filepath.Join(tmpDir, "profiles")
	_ = os.MkdirAll(profileDir, 0o755)

	server, err := NewServer(ServerConfig{
		Port:         0,
		ProfilePaths: profiles.Paths{Dir: profileDir},
		HistoryDir:   filepath.Join(tmpDir, "history"),
	})
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	// 1. Test status endpoint
	req := httptest.NewRequest(http.MethodGet, "/api/test/status", nil)
	rec := httptest.NewRecorder()
	server.handleTestStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}

	var statusMap map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&statusMap); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if isRunning, ok := statusMap["is_running"].(bool); !ok || isRunning {
		t.Errorf("expected is_running false, got %v", statusMap["is_running"])
	}

	// 2. Test airports list endpoint (empty initially)
	reqAirports := httptest.NewRequest(http.MethodGet, "/api/airports", nil)
	recAirports := httptest.NewRecorder()
	server.handleGetAirports(recAirports, reqAirports)
	if recAirports.Code != http.StatusOK {
		t.Errorf("expected 200 for airports, got %d", recAirports.Code)
	}

	// 3. Test Antigravity status endpoint
	reqToken := httptest.NewRequest(http.MethodGet, "/api/antigravity/status", nil)
	recToken := httptest.NewRecorder()
	server.handleAntigravityStatus(recToken, reqToken)
	if recToken.Code != http.StatusOK {
		t.Errorf("expected 200 for token status, got %d", recToken.Code)
	}
}
