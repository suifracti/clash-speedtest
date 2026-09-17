package web

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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

func TestWebServer_MonitorHistoryEndpoints(t *testing.T) {
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

	handler := server.buildHandler()

	// 1. GET /api/monitor/samples/cursor
	reqCursor := httptest.NewRequest(http.MethodGet, "/api/monitor/samples/cursor?limit=10&order_desc=true", nil)
	reqCursor.Host = "127.0.0.1:8080"
	recCursor := httptest.NewRecorder()
	handler.ServeHTTP(recCursor, reqCursor)
	if recCursor.Code != http.StatusOK {
		t.Fatalf("GET /api/monitor/samples/cursor: expected 200 OK, got %d, body: %s", recCursor.Code, recCursor.Body.String())
	}
	var cursorPage monitor.SampleCursorPage
	if err := json.NewDecoder(recCursor.Body).Decode(&cursorPage); err != nil {
		t.Fatalf("decode cursor page failed: %v", err)
	}

	// 2. GET /api/monitor/stats
	reqStats := httptest.NewRequest(http.MethodGet, "/api/monitor/stats?probe_type=rtt", nil)
	reqStats.Host = "127.0.0.1:8080"
	recStats := httptest.NewRecorder()
	handler.ServeHTTP(recStats, reqStats)
	if recStats.Code != http.StatusOK {
		t.Fatalf("GET /api/monitor/stats: expected 200 OK, got %d, body: %s", recStats.Code, recStats.Body.String())
	}
	var stats monitor.DerivedStats
	if err := json.NewDecoder(recStats.Body).Decode(&stats); err != nil {
		t.Fatalf("decode stats failed: %v", err)
	}
	if stats.SampleCount != 0 {
		t.Errorf("expected 0 samples initially, got %d", stats.SampleCount)
	}

	// 3. POST /api/monitor/retention CSRF defense tests:
	// a. Untrusted origin must be rejected with 403 Forbidden
	retPayload := []byte(`{"policy":"keep_all"}`)
	reqCSRF := httptest.NewRequest(http.MethodPost, "/api/monitor/retention", bytes.NewReader(retPayload))
	reqCSRF.Host = "127.0.0.1:8080"
	reqCSRF.Header.Set("Origin", "http://malicious-site.com")
	reqCSRF.Header.Set("Content-Type", "application/json")
	recCSRF := httptest.NewRecorder()
	handler.ServeHTTP(recCSRF, reqCSRF)
	if recCSRF.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for untrusted origin on mutating retention, got %d", recCSRF.Code)
	}

	// b. Non-loopback Host must be rejected with 403 Forbidden
	reqHost := httptest.NewRequest(http.MethodPost, "/api/monitor/retention", bytes.NewReader(retPayload))
	reqHost.Host = "external-attacker.com:8080"
	recHost := httptest.NewRecorder()
	handler.ServeHTTP(recHost, reqHost)
	if recHost.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for non-loopback Host, got %d", recHost.Code)
	}

	// c. Valid loopback request succeeds
	reqValid := httptest.NewRequest(http.MethodPost, "/api/monitor/retention", bytes.NewReader(retPayload))
	reqValid.Host = "127.0.0.1:8080"
	reqValid.Header.Set("Origin", "http://127.0.0.1:8080")
	reqValid.Header.Set("Content-Type", "application/json")
	recValid := httptest.NewRecorder()
	handler.ServeHTTP(recValid, reqValid)
	if recValid.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid retention request, got %d, body: %s", recValid.Code, recValid.Body.String())
	}
	var retResult monitor.RetentionResult
	if err := json.NewDecoder(recValid.Body).Decode(&retResult); err != nil {
		t.Fatalf("decode retention result failed: %v", err)
	}
	if retResult.Policy != monitor.RetentionKeepAll || retResult.SamplesDeleted != 0 {
		t.Errorf("unexpected retention result: %+v", retResult)
	}
}

func TestWebServer_MonitorValidation_400BadRequest(t *testing.T) {
	// R-03: Verify client input validation errors map to HTTP 400 Bad Request
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

	handler := server.buildHandler()

	// 1. Invalid limits: limit=0, limit=2000, limit=invalid
	for _, lim := range []string{"0", "2000", "-5", "not_a_number"} {
		req := httptest.NewRequest(http.MethodGet, "/api/monitor/samples/cursor?limit="+lim, nil)
		req.Host = "127.0.0.1:8080"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request for limit=%s, got %d", lim, rec.Code)
		}
	}

	// 2. Invalid time range: since > until
	reqBadRange := httptest.NewRequest(http.MethodGet, "/api/monitor/samples/cursor?since=2026-09-02T00:00:00Z&until=2026-09-01T00:00:00Z", nil)
	reqBadRange.Host = "127.0.0.1:8080"
	recBadRange := httptest.NewRecorder()
	handler.ServeHTTP(recBadRange, reqBadRange)
	if recBadRange.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for since > until on cursor, got %d", recBadRange.Code)
	}

	reqBadStatsRange := httptest.NewRequest(http.MethodGet, "/api/monitor/stats?since=2026-09-02T00:00:00Z&until=2026-09-01T00:00:00Z", nil)
	reqBadStatsRange.Host = "127.0.0.1:8080"
	recBadStatsRange := httptest.NewRecorder()
	handler.ServeHTTP(recBadStatsRange, reqBadStatsRange)
	if recBadStatsRange.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for since > until on stats, got %d", recBadStatsRange.Code)
	}

	// 3. Oversized cursor token (> 512 bytes)
	longCursor := strings.Repeat("x", 550)
	reqOversized := httptest.NewRequest(http.MethodGet, "/api/monitor/samples/cursor?cursor="+longCursor, nil)
	reqOversized.Host = "127.0.0.1:8080"
	recOversized := httptest.NewRecorder()
	handler.ServeHTTP(recOversized, reqOversized)
	if recOversized.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for oversized cursor token, got %d", recOversized.Code)
	}

	// 4. POST /api/monitor/retention validation
	// a. Malformed JSON
	reqBadJSON := httptest.NewRequest(http.MethodPost, "/api/monitor/retention", strings.NewReader("{invalid-json"))
	reqBadJSON.Host = "127.0.0.1:8080"
	reqBadJSON.Header.Set("Origin", "http://127.0.0.1:8080")
	recBadJSON := httptest.NewRecorder()
	handler.ServeHTTP(recBadJSON, reqBadJSON)
	if recBadJSON.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for malformed JSON, got %d", recBadJSON.Code)
	}

	// b. Custom retention with custom_days <= 0
	reqCustom0 := httptest.NewRequest(http.MethodPost, "/api/monitor/retention", strings.NewReader(`{"policy":"custom","custom_days":0}`))
	reqCustom0.Host = "127.0.0.1:8080"
	reqCustom0.Header.Set("Origin", "http://127.0.0.1:8080")
	recCustom0 := httptest.NewRecorder()
	handler.ServeHTTP(recCustom0, reqCustom0)
	if recCustom0.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for custom_days=0, got %d", recCustom0.Code)
	}

	// c. Custom retention with custom_days > 36500 (N-02 hard max)
	reqCustomHuge := httptest.NewRequest(http.MethodPost, "/api/monitor/retention", strings.NewReader(`{"policy":"custom","custom_days":50000}`))
	reqCustomHuge.Host = "127.0.0.1:8080"
	reqCustomHuge.Header.Set("Origin", "http://127.0.0.1:8080")
	recCustomHuge := httptest.NewRecorder()
	handler.ServeHTTP(recCustomHuge, reqCustomHuge)
	if recCustomHuge.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for custom_days > 36500, got %d", recCustomHuge.Code)
	}

	// d. Retention with future cutoff time
	futureTime := time.Now().Add(48 * time.Hour).Format(time.RFC3339)
	reqFuture := httptest.NewRequest(http.MethodPost, "/api/monitor/retention", strings.NewReader(`{"policy":"custom","cutoff_time":"`+futureTime+`"}`))
	reqFuture.Host = "127.0.0.1:8080"
	reqFuture.Header.Set("Origin", "http://127.0.0.1:8080")
	recFuture := httptest.NewRecorder()
	handler.ServeHTTP(recFuture, reqFuture)
	if recFuture.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for future cutoff time, got %d", recFuture.Code)
	}
}

func TestWebServer_Retention_PartialFailure_500Response(t *testing.T) {
	// NEW B-05: When retention partially fails, Web API returns non-2xx (500),
	// but the JSON response must explicitly report partial=true, samples_deleted, runs_deleted, error.
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

	ctx := context.Background()
	now := time.Now().UTC()
	cutoff := now.AddDate(0, 0, -30)

	// Seed 1200 samples
	var samples []*monitor.MonitorSample
	for i := 0; i < 1200; i++ {
		samples = append(samples, &monitor.MonitorSample{
			SampleID:  fmt.Sprintf("s_web_fault_%04d", i),
			RunID:     "run_web_fault",
			NodeKey:   "nk_web_fault",
			Timestamp: cutoff.Add(-time.Duration(i+1) * time.Minute),
			Success:   true,
		})
	}
	if err := server.AppService().HistoryStore().SaveMonitorSamples(ctx, samples); err != nil {
		t.Fatalf("save samples: %v", err)
	}

	// Trigger fault on batch 3
	server.AppService().HistoryStore().SetTestBatchFailAt(3)

	handler := server.buildHandler()
	payload := fmt.Sprintf(`{"policy":"30d","cutoff_time":"%s"}`, cutoff.Format(time.RFC3339))
	req := httptest.NewRequest(http.MethodPost, "/api/monitor/retention", strings.NewReader(payload))
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Origin", "http://127.0.0.1:8080")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Non-2xx status (500 Internal Server Error)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error for partial retention failure, got %d", rec.Code)
	}

	var respBody map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&respBody); err != nil {
		t.Fatalf("decode 500 response body failed: %v", err)
	}

	// Must explicitly report partial=true, samples_deleted=1000, runs_deleted=0, and error
	if respBody["partial"] != true {
		t.Errorf("expected partial=true in response, got %v", respBody["partial"])
	}
	if respBody["samples_deleted"] != float64(1000) {
		t.Errorf("expected samples_deleted=1000 in response, got %v", respBody["samples_deleted"])
	}
	if respBody["error"] == nil || respBody["error"] == "" {
		t.Errorf("expected non-empty error message in response, got %v", respBody["error"])
	}
}

func TestWebServer_MigrationContinuity_Endpoint(t *testing.T) {
	// Verify that PR3 legacy samples + PR4 new samples are continuously returned
	// via GET /api/monitor/samples/cursor using node_identity_key + legacy_node_key.
	tmpDir := t.TempDir()
	hDir := filepath.Join(tmpDir, "history")
	_ = os.MkdirAll(hDir, 0o755)
	dbPath := filepath.Join(hDir, "history.db")

	// 1. Setup PR#3 schema
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

	tLegacy := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	_, err = rawDB.Exec(`
		INSERT INTO monitor_samples (
			sample_id, run_id, node_key, profile_id, display_name_snapshot,
			probe_type, target, timestamp, success, latency_ms, ttfb_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "s_web_legacy", "run_pr3", "nk_web_legacy_node", "prof_1", "LegacyNode", "rtt", "https://test.com", tLegacy, 1, 50, 40)
	if err != nil {
		t.Fatalf("insert legacy sample: %v", err)
	}
	_ = rawDB.Close()

	// 2. Start server
	server, err := NewServer(ServerConfig{
		Port:         0,
		ProfilePaths: profiles.Paths{Dir: filepath.Join(tmpDir, "profiles")},
		HistoryDir:   hDir,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer server.Close()

	// 3. Insert PR#4 new sample
	tPR4 := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	newIdentity := "nid_ss_web_node_8388_def456"
	err = server.AppService().HistoryStore().SaveMonitorSamples(context.Background(), []*monitor.MonitorSample{
		{
			SampleID:            "s_web_new",
			RunID:               "run_pr4",
			NodeKey:             "nk_web_legacy_node",
			NodeIdentityKey:     newIdentity,
			ConfigRevisionKey:   "rev_def456",
			ProfileID:           "prof_1",
			DisplayNameSnapshot: "LegacyNode",
			ProbeType:           "rtt",
			Target:              "https://test.com",
			Timestamp:           tPR4,
			Success:             true,
			Latency:             35 * time.Millisecond,
			TTFB:                25 * time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("save pr4 sample: %v", err)
	}

	handler := server.buildHandler()

	// 4. GET /api/monitor/samples/cursor with node_identity_key + legacy_node_key
	url := fmt.Sprintf("/api/monitor/samples/cursor?node_identity_key=%s&legacy_node_key=%s&limit=10&order_desc=true",
		newIdentity, "nk_web_legacy_node")
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Host = "127.0.0.1:8080"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var page monitor.SampleCursorPage
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decode page: %v", err)
	}

	if len(page.Items) != 2 {
		t.Fatalf("expected 2 continuous items bridging PR3 and PR4, got %d", len(page.Items))
	}
	if page.Items[0].SampleID != "s_web_new" || page.Items[1].SampleID != "s_web_legacy" {
		t.Errorf("unexpected continuous item sequence: %s, %s", page.Items[0].SampleID, page.Items[1].SampleID)
	}
}





