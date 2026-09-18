package application

import (
	"context"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/policy"
)

// =============================================================================
// Regression tests for the external review findings on PR#7, at the application
// (wiring) layer.
// =============================================================================

// TestAppService_GetMonitorRecommendation_ProfileIsolation is the merge-gate test for
// ProfileID evidence isolation.
//
// The two nodes below are byte-identical proxy configurations (same type/server/port/
// credentials) observed under two different subscriptions/profiles. They therefore share
// the same NodeIdentityKey, the same ConfigRevisionKey and even the same NodeKey — the
// ONLY discriminator is ProfileID. If evidence were aggregated across profiles, profile
// A would inherit profile B's latency.
func TestAppService_GetMonitorRecommendation_ProfileIsolation(t *testing.T) {
	svc, hStore := newEvidenceTestService(t)

	pol := policy.DefaultSwitchPolicy()
	pol.Mode = policy.ModeRecommend
	if err := svc.UpdateSwitchPolicy(context.Background(), pol); err != nil {
		t.Fatalf("UpdateSwitchPolicy: %v", err)
	}

	sharedServer := "10.40.0.1"
	sharedPort := 8388
	sharedPassword := "same-password"

	jobA, err := svc.CreateMonitorJob(monitor.MonitorJob{
		ID:        "job_prof_a",
		Name:      "Profile A",
		ProfileID: "prof-A",
		ProbeSet:  monitor.ProbeSetLight,
		Interval:  time.Hour,
		Nodes:     []monitor.MonitoredNode{evidenceTestNode("SHARED-A", sharedServer, sharedPort, sharedPassword)},
	})
	if err != nil {
		t.Fatalf("CreateMonitorJob(A): %v", err)
	}
	jobB, err := svc.CreateMonitorJob(monitor.MonitorJob{
		ID:        "job_prof_b",
		Name:      "Profile B",
		ProfileID: "prof-B",
		ProbeSet:  monitor.ProbeSetLight,
		Interval:  time.Hour,
		Nodes:     []monitor.MonitoredNode{evidenceTestNode("SHARED-B", sharedServer, sharedPort, sharedPassword)},
	})
	if err != nil {
		t.Fatalf("CreateMonitorJob(B): %v", err)
	}

	// Precondition: the identity key is genuinely shared, so only ProfileID can separate them.
	if jobA.Nodes[0].NodeIdentityKey != jobB.Nodes[0].NodeIdentityKey {
		t.Fatalf("test precondition failed: expected a shared NodeIdentityKey, got %q vs %q",
			jobA.Nodes[0].NodeIdentityKey, jobB.Nodes[0].NodeIdentityKey)
	}
	if jobA.Nodes[0].ConfigRevisionKey != jobB.Nodes[0].ConfigRevisionKey {
		t.Fatalf("test precondition failed: expected a shared ConfigRevisionKey")
	}

	now := time.Now()
	// Profile A is slow (300ms); profile B is fast (10ms) on the very same endpoint.
	insertEvidenceSamples(t, hStore, jobA.Nodes[0], "prof-A", "pa", now, 10*time.Second, 30*time.Second, 5, true, 300, "")
	insertEvidenceSamples(t, hStore, jobB.Nodes[0], "prof-B", "pb", now, 10*time.Second, 30*time.Second, 5, true, 10, "")

	ctx := context.Background()

	recA, err := svc.GetMonitorRecommendation(ctx, MonitorRecommendationRequest{
		JobID:          "job_prof_a",
		CurrentNodeKey: jobA.Nodes[0].NodeKey,
	})
	if err != nil {
		t.Fatalf("GetMonitorRecommendation(A): %v", err)
	}
	if recA.CurrentNode == nil {
		t.Fatalf("expected current node evidence for profile A")
	}
	if recA.CurrentNode.ProfileID != "prof-A" {
		t.Fatalf("expected profile A attribution, got %q", recA.CurrentNode.ProfileID)
	}
	if recA.CurrentNode.LatencyP50 != 300*time.Millisecond {
		t.Fatalf("cross-profile evidence leaked: profile A P50 = %s, want 300ms", recA.CurrentNode.LatencyP50)
	}
	if recA.CurrentNode.SampleCount != 5 {
		t.Fatalf("expected only profile A's 5 samples, got %d", recA.CurrentNode.SampleCount)
	}
	if recA.Snapshot.Source.ProfileID != "prof-A" {
		t.Fatalf("expected the provenance to record prof-A, got %q", recA.Snapshot.Source.ProfileID)
	}

	// Cross-check: profile B must see its own evidence, proving the filter is not just
	// dropping everything.
	recB, err := svc.GetMonitorRecommendation(ctx, MonitorRecommendationRequest{
		JobID:          "job_prof_b",
		CurrentNodeKey: jobB.Nodes[0].NodeKey,
	})
	if err != nil {
		t.Fatalf("GetMonitorRecommendation(B): %v", err)
	}
	if recB.CurrentNode == nil || recB.CurrentNode.LatencyP50 != 10*time.Millisecond {
		t.Fatalf("profile B must see its own evidence, got %+v", recB.CurrentNode)
	}
	if recB.CurrentNode.ProfileID != "prof-B" {
		t.Fatalf("expected profile B attribution, got %q", recB.CurrentNode.ProfileID)
	}

	// Every reason must be auditable down to the profile it was computed from.
	foundProfileReason := false
	for _, reason := range recA.Reasons {
		if reason.Code == policy.ReasonProfileIsolation {
			foundProfileReason = true
			if reason.Evidence["profile_id"] != "prof-A" {
				t.Fatalf("profile isolation reason must record prof-A, got %+v", reason.Evidence)
			}
		}
	}
	if !foundProfileReason {
		t.Fatalf("expected a profile isolation reason in the output")
	}
}

// TestAppService_GetMonitorRecommendation_PreviewOptIn covers the explicit preview
// semantic at the application layer: monitor_only suppresses by default, and the
// override is only reachable through an explicit opt-in.
func TestAppService_GetMonitorRecommendation_PreviewOptIn(t *testing.T) {
	svc, hStore := newEvidenceTestService(t)

	mockCtrl := &testMockController{selectedNode: "HK-01"}
	svc.SetController(mockCtrl, ControllerConfigDTO{Endpoint: "http://127.0.0.1:9090", Mode: "external"})

	// Deliberately keep the secure default: monitor_only.
	pol := policy.DefaultSwitchPolicy()
	pol.Mode = policy.ModeMonitorOnly
	pol.TargetGroup = "PROXY"
	if err := svc.UpdateSwitchPolicy(context.Background(), pol); err != nil {
		t.Fatalf("UpdateSwitchPolicy: %v", err)
	}

	job := registerEvidenceJob(t, svc)
	now := time.Now()
	insertEvidenceSamples(t, hStore, job.Nodes[0], "prof-1", "hk", now, 10*time.Second, 30*time.Second, 5, true, 200, "")
	insertEvidenceSamples(t, hStore, job.Nodes[1], "prof-1", "sg", now, 10*time.Second, 30*time.Second, 5, true, 20, "")

	ctx := context.Background()

	plain, err := svc.GetMonitorRecommendation(ctx, MonitorRecommendationRequest{
		JobID:          "rec_job",
		CurrentNodeKey: job.Nodes[0].NodeKey,
	})
	if err != nil {
		t.Fatalf("GetMonitorRecommendation: %v", err)
	}
	if !plain.RecommendationSuppressed || plain.RecommendedNode != nil {
		t.Fatalf("monitor_only must suppress by default, got %+v", plain)
	}

	preview, err := svc.GetMonitorRecommendation(ctx, MonitorRecommendationRequest{
		JobID:          "rec_job",
		CurrentNodeKey: job.Nodes[0].NodeKey,
		Preview:        true,
	})
	if err != nil {
		t.Fatalf("GetMonitorRecommendation (preview): %v", err)
	}
	if preview.RecommendationSuppressed || !preview.Preview {
		t.Fatalf("an explicit preview must produce a flagged preview, got %+v", preview)
	}
	if preview.Decision != policy.DecisionRecommendSwitch {
		t.Fatalf("expected recommend_switch in the preview, got %s", preview.Decision)
	}
	if preview.RecommendedNode == nil || preview.RecommendedNode.DisplayName != "SG-01" {
		t.Fatalf("expected SG-01 in the preview, got %+v", preview.RecommendedNode)
	}

	// Both paths remain strictly read-only.
	if mockCtrl.selectCalls != 0 {
		t.Fatalf("CRITICAL: preview must never call SelectNode, got %d calls", mockCtrl.selectCalls)
	}
	if mockCtrl.selectedNode != "HK-01" {
		t.Fatalf("controller selection changed unexpectedly: %s", mockCtrl.selectedNode)
	}
	if preview.SelectNodeCalls != 0 || preview.Executed || plain.Executed {
		t.Fatalf("both paths must remain non-executing")
	}
}

// TestAppService_GetMonitorRecommendation_EvidenceWindowIndependentOfMaxSampleAge
// proves the scan window is not MaxSampleAge at the wiring layer, including the DB query
// window: a policy requiring a 30m observation with a 5m freshness threshold must still
// see a 40m history.
func TestAppService_GetMonitorRecommendation_EvidenceWindowIndependentOfMaxSampleAge(t *testing.T) {
	svc, hStore := newEvidenceTestService(t)

	pol := policy.DefaultSwitchPolicy()
	pol.Mode = policy.ModeRecommend
	pol.MaxSampleAge = 5 * time.Minute
	pol.MinObservationWindow = 30 * time.Minute
	pol.MinSampleCount = 3
	if err := svc.UpdateSwitchPolicy(context.Background(), pol); err != nil {
		t.Fatalf("UpdateSwitchPolicy: %v", err)
	}

	job := registerEvidenceJob(t, svc)
	now := time.Now()
	// 41 samples one minute apart, newest 10s old → 40m of observation inside the job.
	insertEvidenceSamples(t, hStore, job.Nodes[0], "prof-1", "hk", now, 10*time.Second, time.Minute, 41, true, 200, "")
	insertEvidenceSamples(t, hStore, job.Nodes[1], "prof-1", "sg", now, 10*time.Second, time.Minute, 41, true, 40, "")

	rec, err := svc.GetMonitorRecommendation(context.Background(), MonitorRecommendationRequest{
		JobID:          "rec_job",
		CurrentNodeKey: job.Nodes[0].NodeKey,
	})
	if err != nil {
		t.Fatalf("GetMonitorRecommendation: %v", err)
	}

	if rec.Gate.EvidenceWindow != policy.DefaultEvidenceWindow(pol) {
		t.Fatalf("expected the policy-resolved window %s, got %s",
			policy.DefaultEvidenceWindow(pol), rec.Gate.EvidenceWindow)
	}
	if rec.Gate.EvidenceWindow == pol.MaxSampleAge {
		t.Fatalf("the evidence window must not be MaxSampleAge")
	}
	if rec.SampleCount != 41 {
		t.Fatalf("expected all 41 samples inside the evidence window, got %d", rec.SampleCount)
	}
	if rec.ObservationWindow != 40*time.Minute {
		t.Fatalf("expected a 40m observation window, got %s", rec.ObservationWindow)
	}
	if rec.Decision != policy.DecisionRecommendSwitch {
		t.Fatalf("expected recommend_switch, got %s", rec.Decision)
	}
}
