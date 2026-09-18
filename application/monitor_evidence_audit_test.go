package application

import (
	"context"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/policy"
)

// TestAppService_GetMonitorRecommendation_UnmatchedCurrentNodeIsRejected pins the
// consistency rule: an explicit identifier that cannot be honoured must not be silently
// ignored. Falling back to a different node would answer a question the caller did not ask.
// This mirrors the existing candidate_node_keys behaviour.
func TestAppService_GetMonitorRecommendation_UnmatchedCurrentNodeIsRejected(t *testing.T) {
	svc, _ := newEvidenceTestService(t)
	registerEvidenceJob(t, svc)
	ctx := context.Background()

	_, err := svc.GetMonitorRecommendation(ctx, MonitorRecommendationRequest{
		JobID:          "rec_job",
		CurrentNodeKey: "nk_does_not_exist",
	})
	if err == nil {
		t.Fatalf("expected an error for an unmatched current_node_key")
	}
	if !monitor.IsValidationError(err) {
		t.Fatalf("expected a validation error, got %v", err)
	}

	_, err = svc.GetMonitorRecommendation(ctx, MonitorRecommendationRequest{
		JobID:                  "rec_job",
		CurrentNodeIdentityKey: "nid_does_not_exist",
	})
	if err == nil {
		t.Fatalf("expected an error for an unmatched current_node_identity_key")
	}
	if !monitor.IsValidationError(err) {
		t.Fatalf("expected a validation error, got %v", err)
	}
}

// TestAppService_GetMonitorRecommendation_EvidenceWindowCoversFreshnessHorizon proves the
// observation window can never be narrower than the freshness horizon.
//
// With MaxSampleAge = 90m a node whose newest sample is 70m old is still FRESH. If the
// evidence window were capped at 1h, that node would be reported as having no evidence at all
// instead of being evaluated.
func TestAppService_GetMonitorRecommendation_EvidenceWindowCoversFreshnessHorizon(t *testing.T) {
	svc, hStore := newEvidenceTestService(t)

	pol := policy.DefaultSwitchPolicy()
	pol.Mode = policy.ModeRecommend
	pol.MaxSampleAge = 90 * time.Minute
	if err := svc.UpdateSwitchPolicy(context.Background(), pol); err != nil {
		t.Fatalf("UpdateSwitchPolicy: %v", err)
	}

	job, err := svc.CreateMonitorJob(monitor.MonitorJob{
		ID:        "job_freshness",
		Name:      "Freshness Horizon",
		ProfileID: "prof-fresh",
		ProbeSet:  monitor.ProbeSetLight,
		Interval:  time.Hour,
		Nodes:     []monitor.MonitoredNode{evidenceTestNode("OLD-BUT-FRESH", "10.60.0.1", 8388, "pw-old")},
	})
	if err != nil {
		t.Fatalf("CreateMonitorJob: %v", err)
	}

	now := time.Now()
	// Newest sample 70m old: beyond a 1h window, still inside MaxSampleAge.
	insertEvidenceSamples(t, hStore, job.Nodes[0], "prof-fresh", "old", now, 70*time.Minute, 30*time.Second, 5, true, 120, "")

	rec, err := svc.GetMonitorRecommendation(context.Background(), MonitorRecommendationRequest{
		JobID:          "job_freshness",
		CurrentNodeKey: job.Nodes[0].NodeKey,
	})
	if err != nil {
		t.Fatalf("GetMonitorRecommendation: %v", err)
	}
	if rec.Snapshot == nil || len(rec.Snapshot.Nodes) != 1 {
		t.Fatalf("expected the job node to be evaluated, got %+v", rec.Snapshot)
	}
	if rec.CurrentNode == nil || rec.CurrentNode.DisplayName != "OLD-BUT-FRESH" {
		t.Fatalf("expected the current node to be resolved, got %+v", rec.CurrentNode)
	}
	if rec.Freshness != policy.FreshnessFresh {
		t.Fatalf("a 70m-old newest sample must still be fresh under MaxSampleAge=90m, got %s", rec.Freshness)
	}
	if rec.SampleCount != 5 {
		t.Fatalf("expected the 5 samples to be attributed, got %d", rec.SampleCount)
	}
	if rec.Decision != policy.DecisionStay {
		t.Fatalf("with a single node there is no candidate, expected stay, got %s", rec.Decision)
	}
}
