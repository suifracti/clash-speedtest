package application

import (
	"context"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/policy"
)

// TestAppService_GetMonitorRecommendation_WhitelistMatchesDisambiguatedNodes pins the
// interaction between the candidate whitelist and duplicate display names.
//
// Two nodes of the same job can share a DisplayName (a duplicate entry, or the same node
// re-imported). The policy engine disambiguates their evaluation names ("DUP", "DUP#2"), so a
// whitelist expressed with display names must be remapped onto those evaluation names —
// otherwise a legitimate candidate is silently excluded.
func TestAppService_GetMonitorRecommendation_WhitelistMatchesDisambiguatedNodes(t *testing.T) {
	svc, hStore := newEvidenceTestService(t)

	pol := policy.DefaultSwitchPolicy()
	pol.Mode = policy.ModeRecommend
	pol.CandidateNodes = []string{"DUP"}
	if err := svc.UpdateSwitchPolicy(context.Background(), pol); err != nil {
		t.Fatalf("UpdateSwitchPolicy: %v", err)
	}

	// The job node order is preserved into the evidence snapshot, so DUP-a becomes the plain
	// "DUP" evaluation name and DUP-b becomes "DUP#2".
	job, err := svc.CreateMonitorJob(monitor.MonitorJob{
		ID:        "job_dup",
		Name:      "Duplicate names",
		ProfileID: "prof-dup",
		ProbeSet:  monitor.ProbeSetLight,
		Interval:  time.Hour,
		Nodes: []monitor.MonitoredNode{
			evidenceTestNode("HK-01", "10.70.0.9", 8388, "pw-hk"),
			evidenceTestNode("DUP", "10.70.0.1", 8388, "pw-a"),
			evidenceTestNode("DUP", "10.70.0.2", 8388, "pw-b"),
		},
	})
	if err != nil {
		t.Fatalf("CreateMonitorJob: %v", err)
	}

	now := time.Now()
	insertEvidenceSamples(t, hStore, job.Nodes[0], "prof-dup", "hk", now, 10*time.Second, 30*time.Second, 5, true, 200, "")
	insertEvidenceSamples(t, hStore, job.Nodes[1], "prof-dup", "da", now, 10*time.Second, 30*time.Second, 5, true, 60, "")
	insertEvidenceSamples(t, hStore, job.Nodes[2], "prof-dup", "db", now, 10*time.Second, 30*time.Second, 5, true, 20, "")

	rec, err := svc.GetMonitorRecommendation(context.Background(), MonitorRecommendationRequest{
		JobID:          "job_dup",
		CurrentNodeKey: job.Nodes[0].NodeKey,
	})
	if err != nil {
		t.Fatalf("GetMonitorRecommendation: %v", err)
	}
	if rec.Decision != policy.DecisionRecommendSwitch {
		t.Fatalf("expected recommend_switch, got %s", rec.Decision)
	}
	if rec.RecommendedNode == nil {
		t.Fatalf("expected a recommended node")
	}
	// The whitelist must not have silently excluded the faster duplicate.
	if rec.RecommendedNode.NodeKey != job.Nodes[2].NodeKey {
		t.Fatalf("the faster whitelisted duplicate was silently excluded: recommended %s (P50=%s), want %s",
			rec.RecommendedNode.DisplayName, rec.RecommendedNode.LatencyP50, job.Nodes[2].NodeKey)
	}
	if rec.RecommendedNode.LatencyP50 != 20*time.Millisecond {
		t.Fatalf("expected the 20ms duplicate, got P50=%s", rec.RecommendedNode.LatencyP50)
	}
}
