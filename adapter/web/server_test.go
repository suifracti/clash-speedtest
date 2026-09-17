package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
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
	defer server.Close()

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

	// 4. Test Controller status endpoint
	reqCtrl := httptest.NewRequest(http.MethodGet, "/api/controller/status", nil)
	recCtrl := httptest.NewRecorder()
	server.handleGetControllerStatus(recCtrl, reqCtrl)
	if recCtrl.Code != http.StatusOK {
		t.Errorf("expected 200 for controller status, got %d", recCtrl.Code)
	}

	// 5. Test Switch Policy endpoint
	reqPolicy := httptest.NewRequest(http.MethodGet, "/api/controller/policy", nil)
	recPolicy := httptest.NewRecorder()
	server.handleGetSwitchPolicy(recPolicy, reqPolicy)
	if recPolicy.Code != http.StatusOK {
		t.Errorf("expected 200 for switch policy, got %d", recPolicy.Code)
	}
}

func TestWebServer_LoopbackHostClassification(t *testing.T) {
	tests := []struct {
		host     string
		expected bool
	}{
		{"127.0.0.1", true},
		{"127.0.0.1:8080", true},
		{"127.0.0.2", true},
		{"127.0.0.2:9090", true},
		{"::1", true},
		{"[::1]:9090", true},
		{"localhost", true},
		{"localhost:5173", true},
		{"localhost.localdomain", false},
		{"localhost.localdomain:8080", false},
		{"127.evil.com", false},
		{"127.evil.com:8080", false},
		{"127.0.0.1.nip.io", false},
		{"127.0.0.1.nip.io:8080", false},
		{"attacker.com", false},
		{"attacker.com:80", false},
		{"", false},
	}

	for _, tc := range tests {
		got := isLoopbackHost(tc.host)
		if got != tc.expected {
			t.Errorf("isLoopbackHost(%q) = %v; expected %v", tc.host, got, tc.expected)
		}
	}
}

func TestWebServer_SecurityMiddleware(t *testing.T) {
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
	defer server.Close()
	handler := server.Handler()

	// 1. Safe GET request with loopback Host -> 200 OK
	req1 := httptest.NewRequest(http.MethodGet, "/api/test/status", nil)
	req1.Host = "127.0.0.1:8080"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Errorf("GET with loopback host: expected 200, got %d", rec1.Code)
	}

	// 2. Non-loopback Host header (DNS rebinding attempt) -> 403 Forbidden
	req2 := httptest.NewRequest(http.MethodGet, "/api/test/status", nil)
	req2.Host = "attacker.com"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusForbidden {
		t.Errorf("GET with attacker host: expected 403, got %d", rec2.Code)
	}

	// 3. Mutating request (POST /api/settings) with loopback Origin -> 200 OK & reflect Origin (never wildcard)
	body := []byte(`{}`)
	req3 := httptest.NewRequest(http.MethodPost, "/api/settings", bytes.NewReader(body))
	req3.Host = "127.0.0.1:8080"
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Origin", "http://localhost:5173")
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Errorf("POST with loopback origin: expected 200, got %d", rec3.Code)
	}
	if rec3.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Errorf("expected reflected loopback origin, got %q", rec3.Header().Get("Access-Control-Allow-Origin"))
	}

	// 4. Mutating request with untrusted external Origin -> 403 Forbidden
	req4 := httptest.NewRequest(http.MethodPost, "/api/settings", bytes.NewReader(body))
	req4.Host = "127.0.0.1:8080"
	req4.Header.Set("Content-Type", "application/json")
	req4.Header.Set("Origin", "http://evil.com")
	rec4 := httptest.NewRecorder()
	handler.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusForbidden {
		t.Errorf("POST with evil.com origin: expected 403, got %d", rec4.Code)
	}

	// 5. Mutating request with prefix bypass Origin -> 403 Forbidden
	req5 := httptest.NewRequest(http.MethodPost, "/api/settings", bytes.NewReader(body))
	req5.Host = "127.0.0.1:8080"
	req5.Header.Set("Content-Type", "application/json")
	req5.Header.Set("Origin", "http://127.evil.com")
	rec5 := httptest.NewRecorder()
	handler.ServeHTTP(rec5, req5)
	if rec5.Code != http.StatusForbidden {
		t.Errorf("POST with 127.evil.com origin: expected 403, got %d", rec5.Code)
	}

	// 6. Mutating request with nip.io Origin -> 403 Forbidden
	req6 := httptest.NewRequest(http.MethodPost, "/api/settings", bytes.NewReader(body))
	req6.Host = "127.0.0.1:8080"
	req6.Header.Set("Content-Type", "application/json")
	req6.Header.Set("Origin", "http://127.0.0.1.nip.io:8080")
	rec6 := httptest.NewRecorder()
	handler.ServeHTTP(rec6, req6)
	if rec6.Code != http.StatusForbidden {
		t.Errorf("POST with nip.io origin: expected 403, got %d", rec6.Code)
	}

	// 7. Mutating request with untrusted Referer -> 403 Forbidden
	req7 := httptest.NewRequest(http.MethodPost, "/api/settings", bytes.NewReader(body))
	req7.Host = "127.0.0.1:8080"
	req7.Header.Set("Content-Type", "application/json")
	req7.Header.Set("Referer", "http://evil.com/attack")
	rec7 := httptest.NewRecorder()
	handler.ServeHTTP(rec7, req7)
	if rec7.Code != http.StatusForbidden {
		t.Errorf("POST with evil referer: expected 403, got %d", rec7.Code)
	}

	// 8. CORS preflight OPTIONS with loopback Origin -> 200 OK
	req8 := httptest.NewRequest(http.MethodOptions, "/api/settings", nil)
	req8.Host = "127.0.0.1:8080"
	req8.Header.Set("Origin", "http://localhost:5173")
	rec8 := httptest.NewRecorder()
	handler.ServeHTTP(rec8, req8)
	if rec8.Code != http.StatusOK {
		t.Errorf("OPTIONS with loopback origin: expected 200, got %d", rec8.Code)
	}
	if rec8.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Errorf("OPTIONS: expected reflected loopback origin, got %q", rec8.Header().Get("Access-Control-Allow-Origin"))
	}

	// 9. CORS preflight OPTIONS with untrusted Origin -> 403 Forbidden
	req9 := httptest.NewRequest(http.MethodOptions, "/api/settings", nil)
	req9.Host = "127.0.0.1:8080"
	req9.Header.Set("Origin", "http://evil.com")
	rec9 := httptest.NewRecorder()
	handler.ServeHTTP(rec9, req9)
	if rec9.Code != http.StatusForbidden {
		t.Errorf("OPTIONS with evil origin: expected 403, got %d", rec9.Code)
	}

	// 10. Verify that Access-Control-Allow-Origin: * is NEVER set in any responses
	for idx, rec := range []*httptest.ResponseRecorder{rec1, rec2, rec3, rec4, rec5, rec6, rec7, rec8, rec9} {
		if rec.Header().Get("Access-Control-Allow-Origin") == "*" {
			t.Errorf("case %d: Access-Control-Allow-Origin MUST NEVER be wildcard '*'", idx+1)
		}
	}
}

func TestWebServer_SPAHandler(t *testing.T) {
	mockFS := fstest.MapFS{
		"index.html":       {Data: []byte("<html><body>Mock App</body></html>")},
		"assets/style.css": {Data: []byte("body { color: red; }")},
	}
	spa := SPAHandler(mockFS)

	// 1. Root path -> index.html
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	spa.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK || !strings.Contains(rec1.Body.String(), "Mock App") {
		t.Errorf("SPA root: expected index.html content, got %s", rec1.Body.String())
	}

	// 2. Direct static file -> assets/style.css
	req2 := httptest.NewRequest(http.MethodGet, "/assets/style.css", nil)
	rec2 := httptest.NewRecorder()
	spa.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK || !strings.Contains(rec2.Body.String(), "color: red") {
		t.Errorf("SPA static asset: expected css content, got %s", rec2.Body.String())
	}

	// 3. Unknown route -> fallback to index.html (SPA routing)
	req3 := httptest.NewRequest(http.MethodGet, "/controller/dashboard", nil)
	rec3 := httptest.NewRecorder()
	spa.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK || !strings.Contains(rec3.Body.String(), "Mock App") {
		t.Errorf("SPA fallback: expected index.html content, got %s", rec3.Body.String())
	}
}

func TestWebServer_MonitorEndpoints(t *testing.T) {
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
	defer server.Close()

	handler := server.Handler()

	// 1. Create monitor job
	createPayload := []byte(`{
		"id": "job_web_1",
		"name": "Web Test Monitor",
		"interval": 300000000000,
		"nodes": [
			{
				"display_name": "HK-Node",
				"type": "ss",
				"server": "1.2.3.4",
				"port": 8388,
				"raw_config": {"type": "ss", "server": "1.2.3.4", "port": 8388}
			}
		]
	}`)
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/monitor/jobs", bytes.NewReader(createPayload))
	reqCreate.Host = "127.0.0.1:8080"
	reqCreate.Header.Set("Content-Type", "application/json")
	recCreate := httptest.NewRecorder()
	handler.ServeHTTP(recCreate, reqCreate)
	if recCreate.Code != http.StatusCreated {
		t.Fatalf("POST /api/monitor/jobs: expected 201 Created, got %d, body: %s", recCreate.Code, recCreate.Body.String())
	}

	// 2. List monitor jobs
	reqList := httptest.NewRequest(http.MethodGet, "/api/monitor/jobs", nil)
	reqList.Host = "127.0.0.1:8080"
	recList := httptest.NewRecorder()
	handler.ServeHTTP(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Fatalf("GET /api/monitor/jobs: expected 200 OK, got %d", recList.Code)
	}

	// 3. Get specific job
	reqGet := httptest.NewRequest(http.MethodGet, "/api/monitor/jobs/job_web_1", nil)
	reqGet.Host = "127.0.0.1:8080"
	recGet := httptest.NewRecorder()
	handler.ServeHTTP(recGet, reqGet)
	if recGet.Code != http.StatusOK {
		t.Fatalf("GET /api/monitor/jobs/job_web_1: expected 200 OK, got %d", recGet.Code)
	}

	// 4. Start job
	reqStart := httptest.NewRequest(http.MethodPost, "/api/monitor/jobs/job_web_1/start", nil)
	reqStart.Host = "127.0.0.1:8080"
	recStart := httptest.NewRecorder()
	handler.ServeHTTP(recStart, reqStart)
	if recStart.Code != http.StatusOK {
		t.Fatalf("POST /api/monitor/jobs/job_web_1/start: expected 200 OK, got %d", recStart.Code)
	}

	// 5. Pause job
	reqPause := httptest.NewRequest(http.MethodPost, "/api/monitor/jobs/job_web_1/pause", nil)
	reqPause.Host = "127.0.0.1:8080"
	recPause := httptest.NewRecorder()
	handler.ServeHTTP(recPause, reqPause)
	if recPause.Code != http.StatusOK {
		t.Fatalf("POST /api/monitor/jobs/job_web_1/pause: expected 200 OK, got %d", recPause.Code)
	}

	// 6. Resume job
	reqResume := httptest.NewRequest(http.MethodPost, "/api/monitor/jobs/job_web_1/resume", nil)
	reqResume.Host = "127.0.0.1:8080"
	recResume := httptest.NewRecorder()
	handler.ServeHTTP(recResume, reqResume)
	if recResume.Code != http.StatusOK {
		t.Fatalf("POST /api/monitor/jobs/job_web_1/resume: expected 200 OK, got %d", recResume.Code)
	}

	// 7. Stop job
	reqStop := httptest.NewRequest(http.MethodPost, "/api/monitor/jobs/job_web_1/stop", nil)
	reqStop.Host = "127.0.0.1:8080"
	recStop := httptest.NewRecorder()
	handler.ServeHTTP(recStop, reqStop)
	if recStop.Code != http.StatusOK {
		t.Fatalf("POST /api/monitor/jobs/job_web_1/stop: expected 200 OK, got %d", recStop.Code)
	}

	// 8. Query runs
	reqRuns := httptest.NewRequest(http.MethodGet, "/api/monitor/runs?job_id=job_web_1", nil)
	reqRuns.Host = "127.0.0.1:8080"
	recRuns := httptest.NewRecorder()
	handler.ServeHTTP(recRuns, reqRuns)
	if recRuns.Code != http.StatusOK {
		t.Fatalf("GET /api/monitor/runs: expected 200 OK, got %d", recRuns.Code)
	}

	// 9. Query samples
	reqSamples := httptest.NewRequest(http.MethodGet, "/api/monitor/samples?probe_type=rtt", nil)
	reqSamples.Host = "127.0.0.1:8080"
	recSamples := httptest.NewRecorder()
	handler.ServeHTTP(recSamples, reqSamples)
	if recSamples.Code != http.StatusOK {
		t.Fatalf("GET /api/monitor/samples: expected 200 OK, got %d", recSamples.Code)
	}

	// 10. Query timeline
	reqTimeline := httptest.NewRequest(http.MethodGet, "/api/monitor/timeline?node_key=test_nk", nil)
	reqTimeline.Host = "127.0.0.1:8080"
	recTimeline := httptest.NewRecorder()
	handler.ServeHTTP(recTimeline, reqTimeline)
	if recTimeline.Code != http.StatusOK {
		t.Fatalf("GET /api/monitor/timeline: expected 200 OK, got %d", recTimeline.Code)
	}
}

func TestWebServer_StopCascadeAndIdempotency(t *testing.T) {
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

	// 1. Create a monitor job to ensure backend state exists
	jobReq := monitor.MonitorJob{
		ID:       "cascade_job",
		Name:     "Cascade Job",
		ProbeSet: monitor.ProbeSetLight,
		Interval: 1 * time.Minute,
	}
	_, err = server.AppService().CreateMonitorJob(jobReq)
	if err != nil {
		t.Fatalf("CreateMonitorJob failed: %v", err)
	}
	if err := server.AppService().StartMonitorJob("cascade_job"); err != nil {
		t.Fatalf("StartMonitorJob failed: %v", err)
	}

	// 2. Calling Stop should cleanly shutdown httpServer and cascade to AppService.Close()
	ctx := context.Background()
	if err := server.Stop(ctx); err != nil {
		t.Fatalf("First server.Stop failed: %v", err)
	}

	// Verify monitor job was stopped by the cascade
	job, err := server.AppService().GetMonitorJob("cascade_job")
	if err != nil {
		t.Fatalf("GetMonitorJob failed: %v", err)
	}
	if job.State != monitor.JobStateStopped {
		t.Errorf("Expected job state %s after server.Stop cascade, got %s", monitor.JobStateStopped, job.State)
	}

	// 3. Repeated Stop calls must be idempotent and succeed without errors
	if err := server.Stop(ctx); err != nil {
		t.Fatalf("Second server.Stop failed: %v", err)
	}

	// 4. Calling Close() after Stop() must also be idempotent
	if err := server.Close(); err != nil {
		t.Fatalf("server.Close after Stop failed: %v", err)
	}
}



