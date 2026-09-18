package application

import (
	"context"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/policy"
)

// TestAppService_GetMonitorRecommendation_DiscoveryKeepsSameNodeKeyAcrossProfiles is a
// self-audit regression for a second-order consequence of ProfileID isolation.
//
// The same subscription node can legitimately appear in two profiles with identical
// credentials. Such nodes share NodeKey, NodeIdentityKey AND ConfigRevisionKey — the ONLY
// discriminator is ProfileID. If the per-node sample sets are keyed by NodeKey, the two
// nodes collide and one silently reads the other's evidence.
func TestAppService_GetMonitorRecommendation_DiscoveryKeepsSameNodeKeyAcrossProfiles(t *testing.T) {
	svc, hStore := newEvidenceTestService(t)

	pol := policy.DefaultSwitchPolicy()
	pol.Mode = policy.ModeRecommend
	if err := svc.UpdateSwitchPolicy(context.Background(), pol); err != nil {
		t.Fatalf("UpdateSwitchPolicy: %v", err)
	}

	// Identical proxy config → identical NodeKey / identity / revision.
	nodeA := evidenceTestNode("SHARED-A", "10.50.0.1", 8388, "same-password")
	nodeB := evidenceTestNode("SHARED-B", "10.50.0.1", 8388, "same-password")
	monitor.PopulateNodeKeys(&nodeA)
	monitor.PopulateNodeKeys(&nodeB)

	if nodeA.NodeKey != nodeB.NodeKey {
		t.Fatalf("test precondition failed: expected a shared NodeKey, got %q vs %q", nodeA.NodeKey, nodeB.NodeKey)
	}
	if nodeA.ConfigRevisionKey != nodeB.ConfigRevisionKey {
		t.Fatalf("test precondition failed: expected a shared ConfigRevisionKey")
	}

	now := time.Now()
	// Profile A is slow (300ms); profile B is fast (10ms) — same physical node.
	insertEvidenceSamples(t, hStore, nodeA, "prof-A", "pa", now, 10*time.Second, 30*time.Second, 5, true, 300, "")
	insertEvidenceSamples(t, hStore, nodeB, "prof-B", "pb", now, 10*time.Second, 30*time.Second, 5, true, 10, "")

	// No job id → node discovery path, which groups by (ProfileID, NodeIdentityKey).
	rec, err := svc.GetMonitorRecommendation(context.Background(), MonitorRecommendationRequest{
		CurrentNodeKey: nodeA.NodeKey,
	})
	if err != nil {
		t.Fatalf("GetMonitorRecommendation: %v", err)
	}
	if rec.Snapshot == nil {
		t.Fatalf("expected an evidence snapshot")
	}
	if len(rec.Snapshot.Nodes) != 2 {
		t.Fatalf("expected 2 discovered nodes (one per profile), got %d", len(rec.Snapshot.Nodes))
	}

	// Each logical node must carry its OWN evidence, not its namesake's.
	byProfile := map[string]*policy.NodeEvidence{}
	for i := range rec.Snapshot.Nodes {
		node := &rec.Snapshot.Nodes[i]
		byProfile[node.ProfileID] = node
	}
	a, okA := byProfile["prof-A"]
	b, okB := byProfile["prof-B"]
	if !okA || !okB {
		t.Fatalf("expected both profiles to be discovered, got %+v", byProfile)
	}
	if a.SampleCount != 5 || b.SampleCount != 5 {
		t.Fatalf("expected 5 samples each, got A=%d B=%d", a.SampleCount, b.SampleCount)
	}
	if a.LatencyP50 != 300*time.Millisecond {
		t.Fatalf("profile A must see its own evidence: P50=%s, want 300ms", a.LatencyP50)
	}
	if b.LatencyP50 != 10*time.Millisecond {
		t.Fatalf("profile B must see its own evidence: P50=%s, want 10ms", b.LatencyP50)
	}
}
