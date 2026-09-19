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
	"github.com/faceair/clash-speedtest/core/controller"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/policy"
	"github.com/faceair/clash-speedtest/core/profiles"
)

// webReadOnlyController is a spy controller used to prove the HTTP recommendation
// endpoint never switches a node.
type webReadOnlyController struct {
	selected    string
	selectCalls int
}

func (c *webReadOnlyController) GetVersion(ctx context.Context) (controller.VersionInfo, error) {
	return controller.VersionInfo{Version: "v1.18.5", CoreType: "mihomo"}, nil
}

func (c *webReadOnlyController) GetCapabilities(ctx context.Context) (controller.Capabilities, error) {
	return controller.Capabilities{CanSelectNode: true}, nil
}

func (c *webReadOnlyController) ListGroups(ctx context.Context) ([]controller.Group, error) {
	return []controller.Group{{Name: "PROXY", Type: "Selector", Now: c.selected}}, nil
}

func (c *webReadOnlyController) ListNodes(ctx context.Context, group string) ([]controller.Node, error) {
	return nil, nil
}

func (c *webReadOnlyController) GetCurrentSelection(ctx context.Context, group string) (string, error) {
	return c.selected, nil
}

func (c *webReadOnlyController) SelectNode(ctx context.Context, group string, nodeName string) error {
	c.selectCalls++
	c.selected = nodeName
	return nil
}

func (c *webReadOnlyController) TestDelay(ctx context.Context, proxyName string, url string, timeout time.Duration) (time.Duration, error) {
	return 0, nil
}

type createdJobDTO struct {
	ID    string `json:"id"`
	Nodes []struct {
		NodeKey           string `json:"node_key"`
		NodeIdentityKey   string `json:"node_identity_key"`
		ConfigRevisionKey string `json:"config_revision_key"`
		DisplayName       string `json:"display_name"`
	} `json:"nodes"`
}

func TestWebServer_MonitorRecommendation_ReadOnlyEndpoint(t *testing.T) {
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
	seedWebMonitorProfile(t, profileDir, "prof-web", "Recommendation Subscription", "HK-01", "SG-01")
	options, err := server.AppService().ListMonitorNodeOptions()
	if err != nil || len(options) != 2 {
		t.Fatalf("ListMonitorNodeOptions: got %d options, err=%v", len(options), err)
	}

	spy := &webReadOnlyController{selected: "HK-01"}
	server.AppService().SetController(spy, application.ControllerConfigDTO{
		Endpoint: "http://127.0.0.1:9090",
		Mode:     "external",
	})

	handler := server.Handler()

	// 1. Register the monitor job that defines the candidate universe.
	createPayload, err := json.Marshal(application.MonitorJobCreateRequest{
		Name:            "Recommendation Web Job",
		ProfileID:       "prof-web",
		NodeKeys:        []string{options[0].NodeKey, options[1].NodeKey},
		ProbeSet:        monitor.ProbeSetService,
		IntervalSeconds: 3600,
		TimeoutSeconds:  10,
	})
	if err != nil {
		t.Fatalf("marshal create payload: %v", err)
	}
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
	if len(job.Nodes) != 2 || job.Nodes[0].NodeKey == "" {
		t.Fatalf("expected 2 nodes with generated keys, got %+v", job.Nodes)
	}
	jobID := job.ID

	// 2. Configure the policy through the existing read/write policy endpoint.
	policyPayload := []byte(`{
		"purpose": "general",
		"mode": "recommend",
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

	// 3. Persist raw monitor evidence directly (no monitor run is triggered).
	now := time.Now()
	saveSamples := func(nodeKey, nodeIdentityKey, configRevisionKey, displayName string, latencyMs int) {
		t.Helper()
		samples := make([]*monitor.MonitorSample, 0, 5)
		for i := 0; i < 5; i++ {
			samples = append(samples, &monitor.MonitorSample{
				SampleID:            fmt.Sprintf("s_%s_%d", nodeKey, i),
				RunID:               "run_web",
				NodeKey:             nodeKey,
				NodeIdentityKey:     nodeIdentityKey,
				ConfigRevisionKey:   configRevisionKey,
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
	saveSamples(job.Nodes[0].NodeKey, job.Nodes[0].NodeIdentityKey, job.Nodes[0].ConfigRevisionKey, "HK-01", 120)
	saveSamples(job.Nodes[1].NodeKey, job.Nodes[1].NodeIdentityKey, job.Nodes[1].ConfigRevisionKey, "SG-01", 40)

	// 4. GET the read-only recommendation.
	url := "/api/monitor/recommendation?job_id=" + jobID + "&current_node_key=" + job.Nodes[0].NodeKey
	reqRec := httptest.NewRequest(http.MethodGet, url, nil)
	reqRec.Host = "127.0.0.1:8080"
	recRec := httptest.NewRecorder()
	handler.ServeHTTP(recRec, reqRec)
	if recRec.Code != http.StatusOK {
		t.Fatalf("GET /api/monitor/recommendation: expected 200, got %d, body: %s", recRec.Code, recRec.Body.String())
	}

	var rec policy.MonitorRecommendation
	if err := json.Unmarshal(recRec.Body.Bytes(), &rec); err != nil {
		t.Fatalf("decode recommendation: %v", err)
	}

	if rec.Decision != policy.DecisionRecommendSwitch {
		t.Fatalf("expected recommend_switch, got %s", rec.Decision)
	}
	if rec.RecommendedNode == nil || rec.RecommendedNode.DisplayName != "SG-01" {
		t.Fatalf("expected SG-01 to be recommended, got %+v", rec.RecommendedNode)
	}
	if rec.CurrentNode == nil || rec.CurrentNode.DisplayName != "HK-01" {
		t.Fatalf("expected HK-01 as the current node, got %+v", rec.CurrentNode)
	}
	if rec.SelectNodeCalls != 0 || rec.Executed || !rec.AdvisoryOnly || rec.AutoImplemented {
		t.Fatalf("web recommendation must be non-executing, got %+v", rec)
	}
	if len(rec.Reasons) == 0 {
		t.Fatalf("recommendation must carry explainable reasons")
	}
	for _, reason := range rec.Reasons {
		if reason.Message == "" || reason.Code == "" {
			t.Fatalf("every reason must carry a code and a message, got %+v", reason)
		}
	}
	if rec.Gate.MinSampleCount != 3 || rec.Gate.MaxSampleAge != 5*time.Minute {
		t.Fatalf("expected the applied evidence gate to be echoed, got %+v", rec.Gate)
	}
	if rec.ConfidenceBasis.Formula == "" || rec.ConfidenceBasis.Detail == "" {
		t.Fatalf("confidence must be explained, got %+v", rec.ConfidenceBasis)
	}
	if rec.Snapshot == nil || rec.Snapshot.Source.JobID != jobID {
		t.Fatalf("expected the evidence snapshot provenance to be preserved")
	}
	if rec.ObservationWindow != 2*time.Minute || rec.SampleCount != 5 {
		t.Fatalf("expected 5 samples over 2m, got %d / %s", rec.SampleCount, rec.ObservationWindow)
	}

	// PROOF: the HTTP read-only endpoint never switched a node.
	if spy.selectCalls != 0 {
		t.Fatalf("CRITICAL: GET /api/monitor/recommendation must never call SelectNode, got %d calls", spy.selectCalls)
	}
	if spy.selected != "HK-01" {
		t.Fatalf("controller selection changed unexpectedly: %s", spy.selected)
	}

	// 5. Validation surface → 400.
	badPurpose := httptest.NewRequest(http.MethodGet, "/api/monitor/recommendation?job_id="+jobID+"&purpose=bogus", nil)
	badPurpose.Host = "127.0.0.1:8080"
	recBadPurpose := httptest.NewRecorder()
	handler.ServeHTTP(recBadPurpose, badPurpose)
	if recBadPurpose.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an invalid purpose, got %d", recBadPurpose.Code)
	}

	unknownJob := httptest.NewRequest(http.MethodGet, "/api/monitor/recommendation?job_id=nope", nil)
	unknownJob.Host = "127.0.0.1:8080"
	recUnknown := httptest.NewRecorder()
	handler.ServeHTTP(recUnknown, unknownJob)
	if recUnknown.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an unknown job id, got %d", recUnknown.Code)
	}

	// An explicit current-node identifier that cannot be honoured is a client error, not a
	// silent fallback to a different node.
	unmatchedNode := httptest.NewRequest(http.MethodGet,
		"/api/monitor/recommendation?job_id=job_rec_web&current_node_key=nk_does_not_exist", nil)
	unmatchedNode.Host = "127.0.0.1:8080"
	recUnmatched := httptest.NewRecorder()
	handler.ServeHTTP(recUnmatched, unmatchedNode)
	if recUnmatched.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an unmatched current_node_key, got %d", recUnmatched.Code)
	}

	// 6. There must be no execution endpoint: POST is not routed.
	postRec := httptest.NewRequest(http.MethodPost, "/api/monitor/recommendation?job_id=job_rec_web", nil)
	postRec.Host = "127.0.0.1:8080"
	recPost := httptest.NewRecorder()
	handler.ServeHTTP(recPost, postRec)
	if recPost.Code == http.StatusOK || recPost.Code == http.StatusCreated {
		t.Fatalf("recommendation endpoint must not accept POST (no execution path), got %d", recPost.Code)
	}
	if spy.selectCalls != 0 {
		t.Fatalf("CRITICAL: SelectNode must remain 0 after all requests, got %d", spy.selectCalls)
	}
}
