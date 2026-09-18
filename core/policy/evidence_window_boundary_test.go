package policy

import (
	"testing"
	"time"
)

// =============================================================================
// MinObservationWindow boundary — resolved by the production path, not by argument.
//
// A configured MinObservationWindow must always be reachable. If the scan window could
// never be wide enough to contain a span of that length, the requirement would be a
// permanent dead-end: every node would be gated `insufficient_observation_window` forever
// and the configuration would be silently unusable.
//
// These tests drive the REAL path (DefaultEvidenceWindow -> EvidenceInput.EvidenceWindow
// -> BuildEvidenceSnapshot -> evaluateSufficiency) instead of asserting on helper outputs.
// =============================================================================

// buildWindowedSnap mirrors the production call in
// application.GetMonitorRecommendation: the window is resolved once from the policy by
// DefaultEvidenceWindow, then passed to BuildEvidenceSnapshot exactly as the app does.
func buildWindowedSnap(now time.Time, p SwitchPolicy, purpose PolicyPurpose, current EvidenceNode,
	nodes []EvidenceNode, samples map[string][]EvidenceSample,
) (EvidenceSnapshot, time.Duration) {
	window := DefaultEvidenceWindow(p)
	snap := BuildEvidenceSnapshot(EvidenceInput{
		Now:     now,
		Purpose: purpose,
		Policy:  p,
		Source: EvidenceSource{
			JobID:         "job-1",
			ProfileID:     testProfile,
			ProbeSet:      "service",
			NodeSetSource: "monitor_job",
			LookbackSince: now.Add(-window),
			LookbackUntil: now,
		},
		EvidenceWindow:         window,
		CurrentNodeKey:         current.NodeKey,
		CurrentNodeIdentityKey: current.NodeIdentityKey,
		Nodes:                  nodes,
		SamplesByNode:          samples,
		RawSampleCount:         countSamples(samples),
	})
	return snap, window
}

// TestMinObservationWindow30hIsReachable is the exact scenario from the review:
//
//	MinObservationWindow = 30h
//	latest sample age    = 30s   (> 0, and fresh under MaxSampleAge)
//	real DefaultEvidenceWindow
//	real evidence build path
//
// The node must end up SUFFICIENT. Before the fix this produced a 24h scan window, so a
// 30h span could never be observed and the requirement was unreachable (29h59m30s /
// insufficient).
func TestMinObservationWindow30hIsReachable(t *testing.T) {
	now := time.Now()

	p := DefaultSwitchPolicy()
	p.Mode = ModeMonitorOnly
	p.MinObservationWindow = 30 * time.Hour
	p.MinSampleCount = 3
	p.MaxSampleAge = 5 * time.Minute

	hk := testNode("hk", "HK-01", "rev_a")

	// Samples spanning a full 30h, with the newest one only 30s old.
	step := 30 * time.Minute
	count := 61 // 60 intervals * 30m = 30h
	series := transportSeries(hk, now, 30*time.Second, step, count, true, 100, "", "")

	samples := map[string][]EvidenceSample{hk.SampleSetKey(): series}
	snap, window := buildWindowedSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk}, samples)

	t.Logf("DefaultEvidenceWindow(MinObservationWindow=30h) = %s", window)
	if window < p.MinObservationWindow {
		t.Fatalf("scan window %s is narrower than the configured MinObservationWindow %s: the "+
			"requirement is unreachable by construction", window, p.MinObservationWindow)
	}

	var cur *NodeEvidence
	for i := range snap.Nodes {
		if snap.Nodes[i].DisplayName == "HK-01" {
			cur = &snap.Nodes[i]
		}
	}
	if cur == nil {
		t.Fatalf("current node missing from snapshot")
	}

	t.Logf("ObservationWindow=%s SampleAge=%s SampleCount=%d LatencySampleCount=%d Sufficient=%v reasons=%v",
		cur.ObservationWindow, cur.SampleAge, cur.SampleCount, cur.LatencySampleCount,
		cur.Sufficiency.Sufficient, cur.Sufficiency.Reasons)

	if cur.ObservationWindow < p.MinObservationWindow {
		t.Errorf("observed span %s is still below MinObservationWindow %s even though the samples span it",
			cur.ObservationWindow, p.MinObservationWindow)
	}
	if !cur.Sufficiency.Sufficient {
		t.Fatalf("MinObservationWindow=30h is UNREACHABLE from the production path: "+
			"window=%s observed=%s reasons=%v details=%v",
			window, cur.ObservationWindow, cur.Sufficiency.Reasons, cur.Sufficiency.Details)
	}
	if len(cur.Sufficiency.Reasons) != 0 {
		t.Errorf("a sufficient node must carry no gate reasons, got %v", cur.Sufficiency.Reasons)
	}
}

// TestDefaultEvidenceWindowNeverDeadEnds sweeps a range of observation requirements and
// asserts the real invariant: given a node that is FRESH (its newest sample is at most
// MaxSampleAge old) and whose samples genuinely span MinObservationWindow, the resolved scan
// window must be wide enough for that span to be observable.
//
// The naive requirement "window >= MinObservationWindow" is NOT sufficient, because the
// newest sample may legitimately be MaxSampleAge old, which eats that much of the span. The
// assertion below is therefore on the usable span: window - MaxSampleAge >= MinObservationWindow.
func TestDefaultEvidenceWindowNeverDeadEnds(t *testing.T) {
	for _, min := range []time.Duration{
		0, time.Minute, 6 * time.Hour, 24 * time.Hour, 30 * time.Hour,
		48 * time.Hour, 72 * time.Hour, 7 * 24 * time.Hour,
	} {
		for _, maxAge := range []time.Duration{0, 5 * time.Minute, time.Hour, 24 * time.Hour} {
			p := DefaultSwitchPolicy()
			p.MinObservationWindow = min
			p.MaxSampleAge = maxAge
			window := DefaultEvidenceWindow(p)

			// The newest sample may be maxAge old, so at most window-maxAge of history remains
			// observable. That usable span must still cover the requirement.
			usable := window - maxAge
			if min > 0 && usable < min {
				t.Errorf("dead-end: MinObservationWindow=%s MaxSampleAge=%s -> window=%s, "+
					"usable span %s (< min): a fresh node could never satisfy the requirement",
					min, maxAge, window, usable)
			}
			t.Logf("min=%-8s maxAge=%-10s -> window=%-9s usable=%s", min, maxAge, window, usable)
		}
	}
}

// TestMinObservationWindow30hRejectsShortSpan is the negative control: the same 30h
// requirement must still REJECT a node whose real span is only a few hours. Reachability
// must not have been bought by weakening the requirement.
func TestMinObservationWindow30hRejectsShortSpan(t *testing.T) {
	now := time.Now()

	p := DefaultSwitchPolicy()
	p.Mode = ModeMonitorOnly
	p.MinObservationWindow = 30 * time.Hour
	p.MinSampleCount = 3
	p.MaxSampleAge = 5 * time.Minute

	hk := testNode("hk", "HK-01", "rev_a")

	// Same shape, but only 2h of real span.
	series := transportSeries(hk, now, 30*time.Second, 2*time.Minute, 61, true, 100, "", "")
	samples := map[string][]EvidenceSample{hk.SampleSetKey(): series}
	snap, _ := buildWindowedSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk}, samples)

	cur := &snap.Nodes[0]
	t.Logf("ObservationWindow=%s Sufficient=%v reasons=%v", cur.ObservationWindow, cur.Sufficiency.Sufficient, cur.Sufficiency.Reasons)

	if cur.Sufficiency.Sufficient {
		t.Fatalf("a 2h span must NOT satisfy MinObservationWindow=30h (observed %s)", cur.ObservationWindow)
	}
	found := false
	for _, r := range cur.Sufficiency.Reasons {
		if r == GateReasonInsufficientObservationWindow {
			found = true
		}
	}
	if !found {
		t.Errorf("expected %s in gate reasons, got %v", GateReasonInsufficientObservationWindow, cur.Sufficiency.Reasons)
	}
}
