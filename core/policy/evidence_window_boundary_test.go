package policy

import (
	"testing"
	"time"
)

// TestDefaultEvidenceWindow_LongObservationIsNotCappedIntoPermanentInsufficiency pins the
// boundary semantics required for MinObservationWindow > 24h.
//
// DefaultEvidenceWindow applies a 24h cap so a pathological policy cannot scan unbounded
// history. That cap must NEVER truncate the window below an explicitly configured
// MinObservationWindow: doing so would make the requirement unsatisfiable forever and the node
// would be permanently insufficient_evidence with no configuration that could ever fix it.
//
// The two guarantees asserted here:
//  1. the resolved window is never smaller than MinObservationWindow, even when the cap applies;
//  2. with a window that covers the requirement, a history of the required length is usable
//     (i.e. the configuration is satisfiable in practice, not just on paper).
func TestDefaultEvidenceWindow_LongObservationIsNotCappedIntoPermanentInsufficiency(t *testing.T) {
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend
	p.MinObservationWindow = 30 * time.Hour
	p.MaxSampleAge = 5 * time.Minute

	window := DefaultEvidenceWindow(p)
	if window < p.MinObservationWindow {
		t.Fatalf("the 24h cap must never truncate an explicit 30h observation requirement: got %s", window)
	}
	if window <= 24*time.Hour {
		t.Fatalf("expected the window to exceed the 24h cap for a 30h requirement, got %s", window)
	}

	now := time.Now()
	hk := testNode("hk", "HK-01", "rev_a")
	sg := testNode("sg", "SG-01", "rev_a")

	// 61 samples 30 minutes apart, newest 10s old → a 30h observation window.
	samples := map[string][]EvidenceSample{
		hk.SampleSetKey(): transportSeries(hk, now, 10*time.Second, 30*time.Minute, 61, true, 100, "", ""),
		sg.SampleSetKey(): transportSeries(sg, now, 10*time.Second, 30*time.Minute, 61, true, 40, "", ""),
	}

	snap := BuildEvidenceSnapshot(EvidenceInput{
		Now:            now,
		Purpose:        PurposeGeneral,
		Policy:         p,
		Source:         EvidenceSource{JobID: "job-long", NodeSetSource: "monitor_job"},
		EvidenceWindow: 4 * p.MinObservationWindow,
		CurrentNodeKey: hk.NodeKey,
		Nodes:          []EvidenceNode{hk, sg},
		SamplesByNode:  samples,
	})

	cur := snap.CurrentNode()
	if cur == nil {
		t.Fatalf("expected current node evidence")
	}
	if cur.ObservationWindow != 30*time.Hour {
		t.Fatalf("expected a 30h observation window, got %s", cur.ObservationWindow)
	}
	if cur.SampleCount != 61 {
		t.Fatalf("expected all 61 samples to be attributed, got %d", cur.SampleCount)
	}
	if !cur.Sufficiency.Sufficient {
		t.Fatalf("a 30h history must satisfy a 30h MinObservationWindow, got %+v", cur.Sufficiency)
	}

	rec := NewDecisionEngine().RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})
	assertNonExecuting(t, rec)
	if rec.Decision != DecisionRecommendSwitch {
		t.Fatalf("expected recommend_switch, got %s", rec.Decision)
	}
	if rec.RecommendedNode == nil || rec.RecommendedNode.DisplayName != "SG-01" {
		t.Fatalf("expected SG-01, got %+v", rec.RecommendedNode)
	}
}
