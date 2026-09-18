package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/application"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/policy"
	"github.com/faceair/clash-speedtest/core/profiles"
)

// TestWebServer_MonitorRecommendation_ConfiguredModeRespectedAndPreviewOptIn covers the
// external review finding on mode semantics at the HTTP layer:
//
//   - with the secure default (monitor_only) the endpoint returns an explicit suppression,
//     not a silent recommendation;
//   - ?preview=true is the only way to override it, and the override is flagged;
//   - ?preview=<not a bool> is rejected;
//   - neither path ever calls SelectNode.
func TestWebServer_MonitorRecommendation_ConfiguredModeRespectedAndPreviewOptIn(t *testing.T) {
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

	spy := &webReadOnlyController{selected: "HK-01"}
	server.AppService().SetController(spy, application.ControllerConfigDTO{
		Endpoint: "http://127.0.0.1:9090",
		Mode:     "external",
	})

	handler := server.Handler()

	createPayload := []byte(`{
		"id": "job_rec_mode",
		"name": "Recommendation Mode Job",
		"profile_id": "prof-web",
		"probe_set": "light",
		"interval": 3600000000000,
		"nodes": [
			{"display_name": "HK-01", "type": "ss", "server": "10.31.0.1", "port": 8388,
			 "raw_config": {"type": "ss", "server": "10.31.0.1", "port": 8388, "password": "pw-hk"}},
			{"display_name": "SG-01", "type": "ss", "server": "10.31.0.2", "port": 8388,
			 "raw_config": {"type": "ss", "server": "10.31.0.2", "port": 8388, "password": "pw-sg"}}
		]
	}`)
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/monitor/jobs", bytes.NewReader(createPayload))
	reqCreate.Host = "127.0.0.1:8080"
	reqCreate.Header.Set("Content-Type", "application/json")
	recCreate := httptest.NewRecorder()
	handler.ServeHTTP(recCreate, reqCreate)
	if recCreate.Code != http.StatusCreated {
		t.Fatalf("POST /api/monitor/jobs: expected 201, got %d, body: %s", recCreate.Code, recCreate.Body.String())
	}
	var job createdJobDTO
	if err := json.Unmarshal(recCreate.Body.Bytes(), &job); err != nil {
		t.Fatalf("decode created job: %v", err)
	}

	// Keep the secure default: monitor_only.
	policyPayload := []byte(`{
		"purpose": "general",
		"mode": "monitor_only",
		"target_group": "PROXY",
		"interval": 60000000000,
		"max_consecutive_failures": 3,
		"min_improvement_rtt": 30000000,
		"min_improvement_ratio": 0.2,
		"cooldown_duration": 300000000000,
		"hysteresis_buffer": 15000000,
		"max_sample_age": 300000000000,
		"min_sample_count": 3,
		"min_observation_window": 60000000000
	}`)
	reqPolicy := httptest.NewRequest(http.MethodPost, "/api/controller/policy", bytes.NewReader(policyPayload))
	reqPolicy.Host = "127.0.0.1:8080"
	reqPolicy.Header.Set("Content-Type", "application/json")
	recPolicy := httptest.NewRecorder()
	handler.ServeHTTP(recPolicy, reqPolicy)
	if recPolicy.Code != http.StatusOK {
		t.Fatalf("POST /api/controller/policy: expected 200, got %d, body: %s", recPolicy.Code, recPolicy.Body.String())
	}

	now := time.Now()
	saveSamples := func(nodeKey, identityKey, revisionKey, displayName string, latencyMs int) {
		t.Helper()
		samples := make([]*monitor.MonitorSample, 0, 5)
		for i := 0; i < 5; i++ {
			samples = append(samples, &monitor.MonitorSample{
				SampleID:            fmt.Sprintf("s_%s_%d", nodeKey, i),
				RunID:               "run_mode",
				NodeKey:             nodeKey,
				NodeIdentityKey:     identityKey,
				ConfigRevisionKey:   revisionKey,
				ProfileID:           "prof-web",
				DisplayNameSnapshot: displayName,
				ProbeType:           "rtt",
				Target:              "https://cp.cloudflare.com/generate_204",
				Timestamp:           now.Add(-10 * time.Second).Add(-time.Duration(4-i) * 30 * time.Second),
				Success:             true,
				Latency:             time.Duration(latencyMs) * time.Millisecond,
				TTFB:                time.Duration(latencyMs) * time.Millisecond,
				ErrorClass:          "none",
			})
		}
		if err := server.AppService().HistoryStore().SaveMonitorSamples(context.Background(), samples); err != nil {
			t.Fatalf("save samples: %v", err)
		}
	}
	saveSamples(job.Nodes[0].NodeKey, job.Nodes[0].NodeIdentityKey, job.Nodes[0].ConfigRevisionKey, "HK-01", 200)
	saveSamples(job.Nodes[1].NodeKey, job.Nodes[1].NodeIdentityKey, job.Nodes[1].ConfigRevisionKey, "SG-01", 20)

	base := "/api/monitor/recommendation?job_id=job_rec_mode&current_node_key=" + job.Nodes[0].NodeKey

	get := func(target string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Host = "127.0.0.1:8080"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	// 1. No preview: the configured monitor_only mode is respected.
	recSuppressed := get(base)
	if recSuppressed.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", recSuppressed.Code, recSuppressed.Body.String())
	}
	var suppressed policy.MonitorRecommendation
	if err := json.Unmarshal(recSuppressed.Body.Bytes(), &suppressed); err != nil {
		t.Fatalf("decode suppressed recommendation: %v", err)
	}
	if !suppressed.RecommendationSuppressed || suppressed.SuppressedReason != policy.SuppressReasonMonitorOnly {
		t.Fatalf("monitor_only must suppress by default, got %+v", suppressed)
	}
	if suppressed.RecommendedNode != nil {
		t.Fatalf("monitor_only must not recommend a node, got %+v", suppressed.RecommendedNode)
	}
	if suppressed.EvaluationMode != policy.ModeMonitorOnly {
		t.Fatalf("expected EvaluationMode monitor_only, got %s", suppressed.EvaluationMode)
	}

	// 2. Explicit preview: the override is applied and flagged.
	recPreview := get(base + "&preview=true")
	if recPreview.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", recPreview.Code, recPreview.Body.String())
	}
	var preview policy.MonitorRecommendation
	if err := json.Unmarshal(recPreview.Body.Bytes(), &preview); err != nil {
		t.Fatalf("decode preview recommendation: %v", err)
	}
	if !preview.Preview || preview.RecommendationSuppressed {
		t.Fatalf("expected a flagged preview, got %+v", preview)
	}
	if preview.Decision != policy.DecisionRecommendSwitch {
		t.Fatalf("expected recommend_switch in the preview, got %s", preview.Decision)
	}
	if preview.RecommendedNode == nil || preview.RecommendedNode.DisplayName != "SG-01" {
		t.Fatalf("expected SG-01 in the preview, got %+v", preview.RecommendedNode)
	}
	if preview.CurrentNode == nil || preview.CurrentNode.ProfileID != "prof-web" {
		t.Fatalf("expected the profile attribution to be preserved, got %+v", preview.CurrentNode)
	}

	// 3. A malformed preview flag is a client error, not a silent default.
	if recBad := get(base + "&preview=maybe"); recBad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an invalid preview parameter, got %d", recBad.Code)
	}

	// 4. Neither path may ever switch a node.
	if spy.selectCalls != 0 {
		t.Fatalf("CRITICAL: SelectNode must stay at 0, got %d calls", spy.selectCalls)
	}
	if spy.selected != "HK-01" {
		t.Fatalf("controller selection changed unexpectedly: %s", spy.selected)
	}
	if suppressed.SelectNodeCalls != 0 || preview.SelectNodeCalls != 0 ||
		suppressed.Executed || preview.Executed {
		t.Fatalf("both paths must remain non-executing")
	}
}
