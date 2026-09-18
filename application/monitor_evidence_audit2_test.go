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
// Two logical nodes can share a DisplayName (the same subscription node observed under two
// profiles). The policy engine disambiguates their evaluation names ("DUP", "DUP#2"), so a
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

	shared := evidenceTestNode("DUP", "10.70.0.1", 8388, "same-password")
	monitor.PopulateNodeKeys(&shared)
	current := evidenceTestNode("HK-01", "10.70.0.9", 8388, "pw-hk")
	monitor.PopulateNodeKeys(&current)

	now := time.Now()
	// The SAME display name under two profiles. The discovery order is newest-first with a
	// sample_id tie-break, so prof-B is enumerated first and therefore receives the plain
	// "DUP" evaluation name while prof-A becomes "DUP#2". prof-A is the FASTER node, so if
	// the whitelist is not remapped onto the disambiguated names, prof-A is silently
	// excluded and the slower prof-B wins.
	insertEvidenceSamples(t, hStore, shared, "prof-A", "pa", now, 10*time.Second, 30*time.Second, 5, true, 20, "")
	insertEvidenceSamples(t, hStore, shared, "prof-B", "pb", now, 10*time.Second, 30*time.Second, 5, true, 60, "")
	insertEvidenceSamples(t, hStore, current, "prof-A", "hk", now, 10*time.Second, 30*time.Second, 5, true, 200, "")

	rec, err := svc.GetMonitorRecommendation(context.Background(), MonitorRecommendationRequest{
		CurrentNodeKey: current.NodeKey,
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
	if rec.RecommendedNode.LatencyP50 != 20*time.Millisecond {
		t.Fatalf("the faster whitelisted candidate was silently excluded: recommended P50=%s, want 20ms",
			rec.RecommendedNode.LatencyP50)
	}
	if rec.RecommendedNode.ProfileID != "prof-A" {
		t.Fatalf("expected the faster profile A node, got %q", rec.RecommendedNode.ProfileID)
	}
}
