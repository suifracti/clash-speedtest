package application

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/policy"
	"github.com/faceair/clash-speedtest/core/profiles"
)

// -----------------------------------------------------------------------------
// PR#7 — Application-level, read-only evidence → recommendation path.
// -----------------------------------------------------------------------------

func newEvidenceTestService(t *testing.T) (*AppService, *history.Store) {
	t.Helper()
	tmpDir := t.TempDir()
	hStore, err := history.NewStore(filepath.Join(tmpDir, "history"))
	if err != nil {
		t.Fatalf("create history store: %v", err)
	}
	svc := NewAppService(hStore, profiles.Paths{Dir: filepath.Join(tmpDir, "profiles")}, NewMemoryEventEmitter())
	t.Cleanup(func() { _ = svc.Close() })
	return svc, hStore
}

func evidenceTestNode(name, server string, port int, password string) monitor.MonitoredNode {
	return monitor.MonitoredNode{
		DisplayName: name,
		Type:        "ss",
		Server:      server,
		Port:        port,
		RawConfig: map[string]any{
			"type":     "ss",
			"server":   server,
			"port":     port,
			"password": password,
		},
	}
}

func insertEvidenceSamples(
	t *testing.T,
	hStore *history.Store,
	node monitor.MonitoredNode,
	profileID, tag string,
	now time.Time,
	age, step time.Duration,
	count int,
	success bool,
	latencyMs int,
	errClass string,
) {
	t.Helper()
	runID := "run_" + tag + "_rtt"
	startedAt := now.Add(-age)
	finishedAt := startedAt.Add(time.Duration(count) * step)
	if err := hStore.SaveMonitorRun(context.Background(), &monitor.MonitorRun{
		RunID: runID, JobID: "evidence-fixture", SamplingTier: monitor.SamplingTierRegular,
		TriggerType: monitor.SamplingTriggerScheduled, SamplingStrategyVersion: monitor.SamplingStrategyVersion,
		ScheduledAt: startedAt, StartedAt: startedAt, FinishedAt: &finishedAt,
		Status: monitor.RunStatusCompleted, TotalNodes: 1, SuccessNodes: 1,
	}); err != nil {
		t.Fatalf("save scheduled evidence run: %v", err)
	}
	samples := make([]*monitor.MonitorSample, 0, count)
	for i := 0; i < count; i++ {
		ts := now.Add(-age).Add(-time.Duration(count-1-i) * step)
		samples = append(samples, &monitor.MonitorSample{
			SampleID:            fmt.Sprintf("s_%s_%s_%d", tag, node.NodeKey, i),
			RunID:               runID,
			NodeKey:             node.NodeKey,
			NodeIdentityKey:     node.NodeIdentityKey,
			ConfigRevisionKey:   node.ConfigRevisionKey,
			ProfileID:           profileID,
			DisplayNameSnapshot: node.DisplayName,
			ProbeType:           "rtt",
			Target:              "https://cp.cloudflare.com/generate_204",
			Timestamp:           ts,
			Success:             success,
			Latency:             time.Duration(latencyMs) * time.Millisecond,
			TTFB:                time.Duration(latencyMs) * time.Millisecond,
			ErrorClass:          errClass,
		})
	}
	if err := hStore.SaveMonitorSamples(context.Background(), samples); err != nil {
		t.Fatalf("save monitor samples: %v", err)
	}
}

func insertBlockedServiceSamples(
	t *testing.T,
	hStore *history.Store,
	node monitor.MonitoredNode,
	profileID, tag string,
	now time.Time,
	age, step time.Duration,
	count int,
) {
	t.Helper()
	runID := "run_" + tag + "_blocked"
	startedAt := now.Add(-age)
	finishedAt := startedAt.Add(time.Duration(count) * step)
	if err := hStore.SaveMonitorRun(context.Background(), &monitor.MonitorRun{
		RunID: runID, JobID: "evidence-fixture", SamplingTier: monitor.SamplingTierRegular,
		TriggerType: monitor.SamplingTriggerScheduled, SamplingStrategyVersion: monitor.SamplingStrategyVersion,
		ScheduledAt: startedAt, StartedAt: startedAt, FinishedAt: &finishedAt,
		Status: monitor.RunStatusCompleted, TotalNodes: 1, SuccessNodes: 0, FailedNodes: 1,
	}); err != nil {
		t.Fatalf("save blocked-service evidence run: %v", err)
	}
	samples := make([]*monitor.MonitorSample, 0, count)
	for i := 0; i < count; i++ {
		ts := now.Add(-age).Add(-time.Duration(count-1-i) * step)
		samples = append(samples, &monitor.MonitorSample{
			SampleID:            fmt.Sprintf("s_%s_blocked_%d", tag, i),
			RunID:               runID,
			NodeKey:             node.NodeKey,
			NodeIdentityKey:     node.NodeIdentityKey,
			ConfigRevisionKey:   node.ConfigRevisionKey,
			ProfileID:           profileID,
			DisplayNameSnapshot: node.DisplayName,
			ProbeType:           "service_google",
			Target:              "https://www.google.com/generate_204",
			Timestamp:           ts,
			Success:             false,
			ErrorClass:          "blocked",
			ErrorDetail:         "HTTP status 400 FAILED_PRECONDITION",
			Metadata:            map[string]any{"region_blocked": true},
		})
	}
	if err := hStore.SaveMonitorSamples(context.Background(), samples); err != nil {
		t.Fatalf("save blocked service samples: %v", err)
	}
}

func registerEvidenceJob(t *testing.T, svc *AppService) *monitor.MonitorJob {
	t.Helper()
	job := monitor.MonitorJob{
		ID:        "rec_job",
		Name:      "Recommendation Job",
		ProfileID: "prof-1",
		ProbeSet:  monitor.ProbeSetService,
		Interval:  time.Hour,
		Nodes: []monitor.MonitoredNode{
			evidenceTestNode("HK-01", "10.10.0.1", 8388, "pw-hk"),
			evidenceTestNode("SG-01", "10.10.0.2", 8388, "pw-sg"),
			evidenceTestNode("JP-01", "10.10.0.3", 8388, "pw-jp"),
		},
	}
	created, err := svc.CreateMonitorJob(job)
	if err != nil {
		t.Fatalf("CreateMonitorJob: %v", err)
	}
	return created
}

// TestAppService_GetMonitorRecommendation_ReadOnlyEvidencePath is the end-to-end
// read-only path test: raw samples → evidence snapshot → policy → recommendation,
// with a hard assertion that no node was ever switched.
func TestAppService_GetMonitorRecommendation_ReadOnlyEvidencePath(t *testing.T) {
	svc, hStore := newEvidenceTestService(t)

	mockCtrl := &testMockController{selectedNode: "HK-01"}
	svc.SetController(mockCtrl, ControllerConfigDTO{Endpoint: "http://127.0.0.1:9090", Mode: "external"})

	job := registerEvidenceJob(t, svc)

	// Default policy stays ModeMonitorOnly: the advisory endpoint must still produce an
	// evidence-backed recommendation while executing absolutely nothing.
	pol := policy.DefaultSwitchPolicy()
	pol.TargetGroup = "PROXY"
	if err := svc.UpdateSwitchPolicy(context.Background(), pol); err != nil {
		t.Fatalf("UpdateSwitchPolicy: %v", err)
	}

	now := time.Now()
	insertEvidenceSamples(t, hStore, job.Nodes[0], "prof-1", "hk", now, 10*time.Second, 30*time.Second, 5, true, 120, "")
	insertEvidenceSamples(t, hStore, job.Nodes[1], "prof-1", "sg", now, 10*time.Second, 30*time.Second, 5, true, 45, "")
	insertEvidenceSamples(t, hStore, job.Nodes[2], "prof-1", "jp", now, 10*time.Second, 30*time.Second, 5, true, 90, "")

	// 1. Default (no preview): the configured monitor_only mode is respected, so no
	// recommendation is produced. Telemetry is still reported.
	suppressed, err := svc.GetMonitorRecommendation(context.Background(), MonitorRecommendationRequest{
		JobID:          "rec_job",
		CurrentNodeKey: job.Nodes[0].NodeKey,
	})
	if err != nil {
		t.Fatalf("GetMonitorRecommendation (monitor_only): %v", err)
	}
	if !suppressed.RecommendationSuppressed || suppressed.SuppressedReason != policy.SuppressReasonMonitorOnly {
		t.Fatalf("monitor_only must suppress the recommendation, got %+v", suppressed)
	}
	if suppressed.RecommendedNode != nil {
		t.Fatalf("monitor_only must not recommend a node, got %+v", suppressed.RecommendedNode)
	}
	if suppressed.EvaluationMode != policy.ModeMonitorOnly {
		t.Fatalf("expected EvaluationMode monitor_only, got %s", suppressed.EvaluationMode)
	}
	if suppressed.CurrentNode == nil {
		t.Fatalf("telemetry must still be reported under monitor_only")
	}

	// 2. Explicit preview: the override is visible and auditable.
	rec, err := svc.GetMonitorRecommendation(context.Background(), MonitorRecommendationRequest{
		JobID:          "rec_job",
		CurrentNodeKey: job.Nodes[0].NodeKey,
		Preview:        true,
	})
	if err != nil {
		t.Fatalf("GetMonitorRecommendation (preview): %v", err)
	}
	if rec == nil {
		t.Fatalf("expected a recommendation result")
	}

	// --- Structural non-execution guarantees ---
	if mockCtrl.selectCalls != 0 {
		t.Fatalf("CRITICAL: recommendation path must never call Controller.SelectNode, got %d calls", mockCtrl.selectCalls)
	}
	if mockCtrl.selectedNode != "HK-01" {
		t.Fatalf("controller selection must not change, got %s", mockCtrl.selectedNode)
	}
	if rec.SelectNodeCalls != 0 || rec.Executed || !rec.AdvisoryOnly || rec.AutoImplemented {
		t.Fatalf("expected non-executing advisory result, got select=%d executed=%v advisory=%v auto=%v",
			rec.SelectNodeCalls, rec.Executed, rec.AdvisoryOnly, rec.AutoImplemented)
	}
	if rec.ConfiguredMode != policy.ModeMonitorOnly {
		t.Fatalf("expected configured mode monitor_only, got %s", rec.ConfiguredMode)
	}
	if rec.EvaluationMode != policy.ModeRecommend {
		t.Fatalf("expected advisory evaluation mode recommend, got %s", rec.EvaluationMode)
	}

	// --- Evidence provenance ---
	if rec.Snapshot == nil {
		t.Fatalf("recommendation must carry its evidence snapshot")
	}
	if rec.Snapshot.Source.JobID != "rec_job" {
		t.Fatalf("expected job provenance rec_job, got %q", rec.Snapshot.Source.JobID)
	}
	if rec.Snapshot.Source.NodeSetSource != "monitor_job" {
		t.Fatalf("expected monitor_job node set source, got %q", rec.Snapshot.Source.NodeSetSource)
	}
	if rec.Snapshot.Source.ProbeSet != string(monitor.ProbeSetService) {
		t.Fatalf("expected probe set provenance, got %q", rec.Snapshot.Source.ProbeSet)
	}
	if len(rec.Snapshot.Nodes) != 3 {
		t.Fatalf("expected 3 nodes in the snapshot, got %d", len(rec.Snapshot.Nodes))
	}
	if rec.SampleCount != 5 || rec.ObservationWindow != 2*time.Minute {
		t.Fatalf("expected 5 samples over a 2m window, got %d / %s", rec.SampleCount, rec.ObservationWindow)
	}
	if rec.Freshness != policy.FreshnessFresh {
		t.Fatalf("expected fresh evidence, got %s", rec.Freshness)
	}
	if rec.Gate.MinSampleCount != pol.MinSampleCount || rec.Gate.MaxSampleAge != pol.MaxSampleAge {
		t.Fatalf("expected the applied gate to be echoed, got %+v", rec.Gate)
	}

	// --- Decision + explainability ---
	if rec.Decision != policy.DecisionRecommendSwitch {
		t.Fatalf("expected recommend_switch, got %s", rec.Decision)
	}
	if rec.RecommendedNode == nil || rec.RecommendedNode.DisplayName != "SG-01" {
		t.Fatalf("expected SG-01 to be recommended, got %+v", rec.RecommendedNode)
	}
	if rec.CurrentNode == nil || rec.CurrentNode.DisplayName != "HK-01" {
		t.Fatalf("expected HK-01 as the current node, got %+v", rec.CurrentNode)
	}
	if len(rec.Reasons) == 0 {
		t.Fatalf("recommendation must carry explainable reasons")
	}
	for _, reason := range rec.Reasons {
		if reason.Message == "" {
			t.Fatalf("reason %s must carry a human-readable message", reason.Code)
		}
	}
	if rec.ConfidenceBasis.Detail == "" || rec.ConfidenceBasis.Formula == "" {
		t.Fatalf("confidence must be derivable from the reported basis, got %+v", rec.ConfidenceBasis)
	}

	// The explicit preview must be surfaced as such, never silently applied.
	if !rec.Preview {
		t.Fatalf("expected the result to be flagged as a preview")
	}
	foundNotice := false
	for _, reason := range rec.Reasons {
		if reason.Code == policy.ReasonPreviewNotice {
			foundNotice = true
		}
	}
	if !foundNotice {
		t.Fatalf("expected a preview notice when overriding monitor_only")
	}
}

// TestAppService_GetMonitorRecommendation_SelectNodeStaysZeroAcrossEveryDecisionPath
// exercises stay / recommend_switch / insufficient_evidence and asserts that the
// controller is never touched in any of them.
func TestAppService_GetMonitorRecommendation_SelectNodeStaysZeroAcrossEveryDecisionPath(t *testing.T) {
	svc, hStore := newEvidenceTestService(t)

	mockCtrl := &testMockController{selectedNode: "HK-01"}
	svc.SetController(mockCtrl, ControllerConfigDTO{Endpoint: "http://127.0.0.1:9090", Mode: "external"})

	pol := policy.DefaultSwitchPolicy()
	pol.Mode = policy.ModeRecommend
	pol.TargetGroup = "PROXY"
	if err := svc.UpdateSwitchPolicy(context.Background(), pol); err != nil {
		t.Fatalf("UpdateSwitchPolicy: %v", err)
	}

	job := registerEvidenceJob(t, svc)
	now := time.Now()

	// Node 0: healthy but slower; Node 1: clearly faster; Node 2: stale.
	insertEvidenceSamples(t, hStore, job.Nodes[0], "prof-1", "hk", now, 10*time.Second, 30*time.Second, 5, true, 120, "")
	insertEvidenceSamples(t, hStore, job.Nodes[1], "prof-1", "sg", now, 10*time.Second, 30*time.Second, 5, true, 45, "")
	insertEvidenceSamples(t, hStore, job.Nodes[2], "prof-1", "jp", now, 20*time.Minute, 30*time.Second, 5, true, 20, "")

	ctx := context.Background()

	// 1. recommend_switch
	rec, err := svc.GetMonitorRecommendation(ctx, MonitorRecommendationRequest{
		JobID:          "rec_job",
		CurrentNodeKey: job.Nodes[0].NodeKey,
	})
	if err != nil {
		t.Fatalf("recommend_switch path: %v", err)
	}
	if rec.Decision != policy.DecisionRecommendSwitch {
		t.Fatalf("expected recommend_switch, got %s", rec.Decision)
	}
	if len(rec.RejectedCandidates) == 0 {
		t.Fatalf("expected the stale candidate to be rejected with a reason")
	}
	rejectedStale := false
	for _, rejection := range rec.RejectedCandidates {
		if rejection.NodeName == "JP-01" && rejection.Code == policy.RejectStaleEvidence {
			rejectedStale = true
		}
	}
	if !rejectedStale {
		t.Fatalf("expected JP-01 to be rejected as stale, got %+v", rec.RejectedCandidates)
	}

	// 2. stay — evaluate from the fastest healthy node's perspective with no better option.
	recStay, err := svc.GetMonitorRecommendation(ctx, MonitorRecommendationRequest{
		JobID:          "rec_job",
		CurrentNodeKey: job.Nodes[1].NodeKey,
	})
	if err != nil {
		t.Fatalf("stay path: %v", err)
	}
	if recStay.Decision != policy.DecisionStay {
		t.Fatalf("expected stay, got %s", recStay.Decision)
	}

	// 3. insufficient_evidence — a stale current node cannot drive a verdict.
	recIns, err := svc.GetMonitorRecommendation(ctx, MonitorRecommendationRequest{
		JobID:          "rec_job",
		CurrentNodeKey: job.Nodes[2].NodeKey,
	})
	if err != nil {
		t.Fatalf("insufficient path: %v", err)
	}
	if recIns.Decision != policy.DecisionInsufficientEvidence {
		t.Fatalf("expected insufficient_evidence, got %s", recIns.Decision)
	}
	if recIns.RecommendedNode != nil {
		t.Fatalf("insufficient evidence must not produce a recommended node")
	}

	// PROOF: every path stayed read-only.
	if mockCtrl.selectCalls != 0 {
		t.Fatalf("CRITICAL: SelectNode must stay at 0 across all paths, got %d", mockCtrl.selectCalls)
	}
	for _, r := range []*policy.MonitorRecommendation{rec, recStay, recIns} {
		if r.SelectNodeCalls != 0 || r.Executed {
			t.Fatalf("every recommendation must be non-executing, got %+v", r)
		}
	}
}

// TestAppService_GetMonitorRecommendation_PurposeLens verifies that the AI purpose lens
// eliminates a region-blocked candidate while general purpose does not.
func TestAppService_GetMonitorRecommendation_PurposeLens(t *testing.T) {
	svc, hStore := newEvidenceTestService(t)

	mockCtrl := &testMockController{selectedNode: "HK-01"}
	svc.SetController(mockCtrl, ControllerConfigDTO{Endpoint: "http://127.0.0.1:9090", Mode: "external"})

	pol := policy.DefaultSwitchPolicy()
	pol.Mode = policy.ModeRecommend
	pol.TargetGroup = "PROXY"
	if err := svc.UpdateSwitchPolicy(context.Background(), pol); err != nil {
		t.Fatalf("UpdateSwitchPolicy: %v", err)
	}

	job := registerEvidenceJob(t, svc)
	now := time.Now()

	// HK-01: slow but clean transport, blocked on Google.
	insertEvidenceSamples(t, hStore, job.Nodes[0], "prof-1", "hk", now, 10*time.Second, 30*time.Second, 5, true, 150, "")
	insertBlockedServiceSamples(t, hStore, job.Nodes[0], "prof-1", "hk", now, 10*time.Second, 30*time.Second, 3)
	// SG-01: fastest AND clean.
	insertEvidenceSamples(t, hStore, job.Nodes[1], "prof-1", "sg", now, 10*time.Second, 30*time.Second, 5, true, 40, "")
	// JP-01: mid latency, blocked on Google.
	insertEvidenceSamples(t, hStore, job.Nodes[2], "prof-1", "jp", now, 10*time.Second, 30*time.Second, 5, true, 70, "")
	insertBlockedServiceSamples(t, hStore, job.Nodes[2], "prof-1", "jp", now, 10*time.Second, 30*time.Second, 3)

	ctx := context.Background()

	// --- general purpose: service-level blocking is NOT a node failure ---
	generalRec, err := svc.GetMonitorRecommendation(ctx, MonitorRecommendationRequest{
		JobID:          "rec_job",
		CurrentNodeKey: job.Nodes[0].NodeKey,
		Purpose:        string(policy.PurposeGeneral),
	})
	if err != nil {
		t.Fatalf("general purpose: %v", err)
	}
	if generalRec.Decision != policy.DecisionRecommendSwitch {
		t.Fatalf("expected recommend_switch under general purpose, got %s", generalRec.Decision)
	}
	if generalRec.RecommendedNode == nil || generalRec.RecommendedNode.DisplayName != "SG-01" {
		t.Fatalf("expected SG-01 under general purpose, got %+v", generalRec.RecommendedNode)
	}
	for _, rejection := range generalRec.RejectedCandidates {
		if rejection.NodeName == "JP-01" && rejection.Code == policy.RejectAIRegionBlocked {
			t.Fatalf("general purpose must not eliminate a node for service-level blocking")
		}
	}

	// --- AI purpose: the same blocking is disqualifying ---
	aiRec, err := svc.GetMonitorRecommendation(ctx, MonitorRecommendationRequest{
		JobID:          "rec_job",
		CurrentNodeKey: job.Nodes[0].NodeKey,
		Purpose:        string(policy.PurposeAI),
	})
	if err != nil {
		t.Fatalf("ai purpose: %v", err)
	}
	eliminated := false
	for _, rejection := range aiRec.RejectedCandidates {
		if rejection.NodeName == "JP-01" && rejection.Code == policy.RejectAIRegionBlocked {
			eliminated = true
		}
	}
	if !eliminated {
		t.Fatalf("expected JP-01 to be eliminated under AI purpose, got %+v", aiRec.RejectedCandidates)
	}
	if aiRec.Purpose != policy.PurposeAI {
		t.Fatalf("expected the AI purpose to be recorded, got %s", aiRec.Purpose)
	}

	if mockCtrl.selectCalls != 0 {
		t.Fatalf("CRITICAL: SelectNode must stay at 0, got %d", mockCtrl.selectCalls)
	}
}

// TestAppService_GetMonitorRecommendation_ValidationErrors covers the client-visible
// error surface of the read-only endpoint.
func TestAppService_GetMonitorRecommendation_ValidationErrors(t *testing.T) {
	svc, _ := newEvidenceTestService(t)
	registerEvidenceJob(t, svc)
	ctx := context.Background()

	if _, err := svc.GetMonitorRecommendation(ctx, MonitorRecommendationRequest{
		JobID:   "does_not_exist",
		Purpose: string(policy.PurposeGeneral),
	}); err == nil {
		t.Fatalf("expected an error for an unknown job id")
	} else if !monitor.IsValidationError(err) {
		t.Fatalf("expected a validation error for an unknown job id, got %v", err)
	}

	if _, err := svc.GetMonitorRecommendation(ctx, MonitorRecommendationRequest{
		JobID:   "rec_job",
		Purpose: "not-a-purpose",
	}); err == nil {
		t.Fatalf("expected an error for an invalid purpose")
	} else if !monitor.IsValidationError(err) {
		t.Fatalf("expected a validation error for an invalid purpose, got %v", err)
	}

	if _, err := svc.GetMonitorRecommendation(ctx, MonitorRecommendationRequest{
		JobID:             "rec_job",
		CandidateNodeKeys: []string{"nk_does_not_exist"},
	}); err == nil {
		t.Fatalf("expected an error when candidate_node_keys matches nothing")
	} else if !monitor.IsValidationError(err) {
		t.Fatalf("expected a validation error for unmatched candidate keys, got %v", err)
	}
}

// TestAppService_GetMonitorRecommendation_DoesNotMutateOrchestratorState proves the
// read-only path leaves the policy and decision state untouched.
func TestAppService_GetMonitorRecommendation_DoesNotMutateOrchestratorState(t *testing.T) {
	svc, hStore := newEvidenceTestService(t)

	pol := policy.DefaultSwitchPolicy()
	pol.Mode = policy.ModeRecommend
	pol.TargetGroup = "PROXY"
	pol.LockedNode = "SG-01"
	if err := svc.UpdateSwitchPolicy(context.Background(), pol); err != nil {
		t.Fatalf("UpdateSwitchPolicy: %v", err)
	}

	job := registerEvidenceJob(t, svc)
	now := time.Now()
	insertEvidenceSamples(t, hStore, job.Nodes[0], "prof-1", "hk", now, 10*time.Second, 30*time.Second, 5, true, 200, "")
	insertEvidenceSamples(t, hStore, job.Nodes[1], "prof-1", "sg", now, 10*time.Second, 30*time.Second, 5, true, 20, "")

	before, err := svc.GetSwitchPolicy(context.Background())
	if err != nil {
		t.Fatalf("GetSwitchPolicy: %v", err)
	}

	rec, err := svc.GetMonitorRecommendation(context.Background(), MonitorRecommendationRequest{
		JobID:          "rec_job",
		CurrentNodeKey: job.Nodes[0].NodeKey,
	})
	if err != nil {
		t.Fatalf("GetMonitorRecommendation: %v", err)
	}
	// Locked node semantics are honored by the advisory path too.
	if rec.Decision != policy.DecisionStay {
		t.Fatalf("expected stay while a node is locked, got %s", rec.Decision)
	}

	after, err := svc.GetSwitchPolicy(context.Background())
	if err != nil {
		t.Fatalf("GetSwitchPolicy: %v", err)
	}
	if before.Mode != after.Mode || before.LockedNode != after.LockedNode ||
		before.TargetGroup != after.TargetGroup || before.MinSampleCount != after.MinSampleCount ||
		len(before.CandidateNodes) != len(after.CandidateNodes) {
		t.Fatalf("the read-only path must not mutate the configured policy: before=%+v after=%+v", before, after)
	}

	trail, err := svc.GetSwitchAuditTrail(context.Background())
	if err != nil {
		t.Fatalf("GetSwitchAuditTrail: %v", err)
	}
	if len(trail) != 0 {
		t.Fatalf("the read-only path must not append to the switch audit trail, got %d entries", len(trail))
	}
}
