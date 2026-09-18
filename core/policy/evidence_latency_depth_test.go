package policy

import (
	"testing"
	"time"
)

// =============================================================================
// Latency-ranking depth gate.
//
// MinSampleCount is scope-agnostic: a mixture of service probes satisfies it while the node
// contributes a single transport latency observation. Ranking on that one number would let a
// single measurement win a switch. MinLatencySampleCount gates latency ranking separately.
// =============================================================================

// TestSingleLatencySampleCannotDriveASwitch is the exact defect: a candidate whose only
// transport latency observation is 1 sample must NOT be recommended for its speed, even when
// MinSampleCount is satisfied by service-scope probes and the observation window is wide.
func TestSingleLatencySampleCannotDriveASwitch(t *testing.T) {
	now := time.Now()

	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend
	p.MinSampleCount = 3
	p.MinObservationWindow = 0
	p.MinLatencySampleCount = 3

	hk := testNode("hk", "HK-01", "rev_a")
	thin := testNode("thin", "THIN-01", "rev_a")

	// Current node: a solid 5-sample transport baseline at 100ms.
	curSeries := transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 100, "", "")

	// Thin candidate: ONE transport success at 1ms (an apparently unbeatable P50), padded to
	// MinSampleCount=3 by two service probes, spread over 70s so the observation window passes.
	thinSeries := transportSeries(thin, now, 5*time.Second, 30*time.Second, 1, true, 1, "", "")
	thinSeries = append(thinSeries, serviceSeries(thin, now, 40*time.Second, 30*time.Second, 2,
		"service_google", true, 50, "", "", false)...)

	samples := map[string][]EvidenceSample{
		hk.SampleSetKey():   curSeries,
		thin.SampleSetKey(): thinSeries,
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, thin}, samples)

	// Sanity: the pad really does satisfy the scope-agnostic sample count.
	var thinEv *NodeEvidence
	for i := range snap.Nodes {
		if snap.Nodes[i].DisplayName == "THIN-01" {
			thinEv = &snap.Nodes[i]
		}
	}
	if thinEv == nil {
		t.Fatal("THIN-01 missing from snapshot")
	}
	t.Logf("THIN-01: sample_count=%d latency_sample_count=%d observation_window=%s sufficient=%v reasons=%v",
		thinEv.SampleCount, thinEv.LatencySampleCount, thinEv.ObservationWindow,
		thinEv.Sufficiency.Sufficient, thinEv.Sufficiency.Reasons)

	if thinEv.SampleCount < p.MinSampleCount {
		t.Fatalf("test setup invalid: sample_count=%d does not satisfy MinSampleCount=%d",
			thinEv.SampleCount, p.MinSampleCount)
	}
	if thinEv.LatencySampleCount != 1 {
		t.Fatalf("test setup invalid: expected exactly 1 latency sample, got %d", thinEv.LatencySampleCount)
	}
	// Sufficiency answers "is the evidence trustworthy?" and must stay true here — the pad of
	// service probes IS trustworthy evidence. What must be false is RANKING eligibility.
	if !thinEv.Sufficiency.Sufficient {
		t.Fatalf("a well-sampled node must remain sufficient; latency depth is a ranking property, got reasons=%v",
			thinEv.Sufficiency.Reasons)
	}
	if thinEv.LatencyRankingEligible(p.MinLatencySampleCount) {
		t.Fatalf("a candidate with %d latency sample(s) must not be eligible for latency ranking",
			thinEv.LatencySampleCount)
	}

	engine := NewDecisionEngine()
	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})

	picked := "<nil>"
	if rec.RecommendedNode != nil {
		picked = rec.RecommendedNode.DisplayName
	}
	t.Logf("decision=%q picked=%s", rec.Decision, picked)

	if picked == "THIN-01" {
		t.Errorf("a candidate backed by a single latency sample was recommended on its P50 (%s)", picked)
	}
	// And the reason must be the latency-depth gate, not a vague one.
	var seen bool
	for _, r := range rec.RejectedCandidates {
		if r.NodeName == "THIN-01" && r.Code == RejectInsufficientLatencySamples {
			seen = true
		}
	}
	if !seen {
		t.Errorf("expected THIN-01 to be rejected with code %q, got %+v", RejectInsufficientLatencySamples, rec.RejectedCandidates)
	}
}

// TestCandidateRanksOnceLatencyDepthIsMet is the positive control: the same candidate becomes
// eligible as soon as it has enough REAL transport latency samples.
func TestCandidateRanksOnceLatencyDepthIsMet(t *testing.T) {
	now := time.Now()

	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend
	p.MinSampleCount = 3
	p.MinObservationWindow = 0
	p.MinLatencySampleCount = 3

	hk := testNode("hk", "HK-01", "rev_a")
	fast := testNode("fast", "FAST-01", "rev_a")

	curSeries := transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 100, "", "")
	// Exactly the threshold: 3 transport successes at 10ms, spanning 60s.
	fastSeries := transportSeries(fast, now, 10*time.Second, 30*time.Second, 3, true, 10, "", "")

	samples := map[string][]EvidenceSample{
		hk.SampleSetKey():   curSeries,
		fast.SampleSetKey(): fastSeries,
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, fast}, samples)

	for i := range snap.Nodes {
		if snap.Nodes[i].DisplayName == "FAST-01" {
			t.Logf("FAST-01: sample_count=%d latency_sample_count=%d sufficient=%v",
				snap.Nodes[i].SampleCount, snap.Nodes[i].LatencySampleCount, snap.Nodes[i].Sufficiency.Sufficient)
		}
	}

	engine := NewDecisionEngine()
	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})

	picked := "<nil>"
	if rec.RecommendedNode != nil {
		picked = rec.RecommendedNode.DisplayName
	}
	t.Logf("decision=%q picked=%s", rec.Decision, picked)

	if picked != "FAST-01" {
		t.Errorf("a candidate meeting the latency-depth threshold must be rankable, got picked=%s decision=%s",
			picked, rec.Decision)
	}
}

// TestLatencyDepthThresholdBoundary pins the off-by-one on RANKING eligibility: n-1 latency
// samples are not rankable, n are. The zero-latency case is deliberately excluded — a node
// with no transport success at all is judged on its failures (see
// TestFailedNodeIsNotLatencyGated), not on latency depth.
func TestLatencyDepthThresholdBoundary(t *testing.T) {
	now := time.Now()

	for _, tc := range []struct {
		latencySamples int
		wantEligible   bool
	}{
		{1, false},
		{2, false},
		{3, true},
		{4, true},
	} {
		p := DefaultSwitchPolicy()
		p.MinSampleCount = 0
		p.MinObservationWindow = 0
		p.MinLatencySampleCount = 3

		n := testNode("n", "N-01", "rev_a")
		series := transportSeries(n, now, 10*time.Second, 20*time.Second, tc.latencySamples, true, 10, "", "")
		samples := map[string][]EvidenceSample{n.SampleSetKey(): series}
		snap := buildSnap(now, p, PurposeGeneral, n, []EvidenceNode{n}, samples)

		ev := &snap.Nodes[0]
		eligible := ev.LatencyRankingEligible(p.MinLatencySampleCount)
		t.Logf("latency_samples=%d -> latency_sample_count=%d eligible=%v sufficient=%v",
			tc.latencySamples, ev.LatencySampleCount, eligible, ev.Sufficiency.Sufficient)

		if eligible != tc.wantEligible {
			t.Errorf("latency_samples=%d: eligible=%v, want %v",
				tc.latencySamples, eligible, tc.wantEligible)
		}
		// Sufficiency is a different question and must not have been disturbed.
		if !ev.Sufficiency.Sufficient {
			t.Errorf("latency_samples=%d: latency depth must not affect sufficiency, got %v",
				tc.latencySamples, ev.Sufficiency.Reasons)
		}
	}
}

// TestFailedNodeIsNotLatencyGated: a node with zero transport successes is judged on its
// failures, not on how much latency data it has. Requiring latency depth from an outage would
// convert a failover into an "insufficient evidence" verdict.
func TestFailedNodeIsNotLatencyGated(t *testing.T) {
	now := time.Now()

	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend
	p.MinLatencySampleCount = 3

	hk := testNode("hk", "HK-01", "rev_a")
	sg := testNode("sg", "SG-01", "rev_a")

	samples := map[string][]EvidenceSample{
		// Current node: 5 consecutive timeouts, zero transport successes.
		hk.SampleSetKey(): transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, false, 0, "timeout", "dial tcp: i/o timeout"),
		sg.SampleSetKey(): transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 40, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, sg}, samples)

	hkEv := &snap.Nodes[0]
	if hkEv.DisplayName != "HK-01" {
		t.Fatalf("unexpected node order")
	}
	t.Logf("HK-01: transport_success=%d latency_sample_count=%d ranking_eligible=%v sufficient=%v reasons=%v",
		hkEv.TransportSuccessCount, hkEv.LatencySampleCount,
		hkEv.LatencyRankingEligible(p.MinLatencySampleCount), hkEv.Sufficiency.Sufficient, hkEv.Sufficiency.Reasons)

	if !hkEv.Sufficiency.Sufficient {
		t.Errorf("a fully-failed node's evidence IS sufficient (it is clearly down); reasons=%v",
			hkEv.Sufficiency.Reasons)
	}
	for _, r := range hkEv.Sufficiency.Reasons {
		if r == GateReasonInsufficientLatencySamples {
			t.Errorf("a fully-failed node must not be gated on latency depth; reasons=%v", hkEv.Sufficiency.Reasons)
		}
	}

	engine := NewDecisionEngine()
	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})
	picked := "<nil>"
	if rec.RecommendedNode != nil {
		picked = rec.RecommendedNode.DisplayName
	}
	t.Logf("decision=%q picked=%s", rec.Decision, picked)
	if picked != "SG-01" {
		t.Errorf("the outage must still fail over to SG-01, got decision=%s picked=%s", rec.Decision, picked)
	}
}

// TestLatencyDepthGateCanBeDisabled documents the escape hatch: 0 disables the check, so a
// caller that explicitly opts out keeps the previous behaviour.
func TestLatencyDepthGateCanBeDisabled(t *testing.T) {
	now := time.Now()

	p := DefaultSwitchPolicy()
	p.MinSampleCount = 0
	p.MinObservationWindow = 0
	p.MinLatencySampleCount = 0

	n := testNode("n", "N-01", "rev_a")
	samples := map[string][]EvidenceSample{
		n.SampleSetKey(): transportSeries(n, now, 10*time.Second, 20*time.Second, 1, true, 10, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, n, []EvidenceNode{n}, samples)
	if !snap.Nodes[0].LatencyRankingEligible(p.MinLatencySampleCount) {
		t.Errorf("with MinLatencySampleCount=0 the gate must be disabled")
	}

	// And the same node with the gate active is NOT eligible (proving the test is meaningful).
	p.MinLatencySampleCount = 3
	snap2 := buildSnap(now, p, PurposeGeneral, n, []EvidenceNode{n}, samples)
	if snap2.Nodes[0].LatencyRankingEligible(p.MinLatencySampleCount) {
		t.Errorf("with MinLatencySampleCount=3 a 1-sample node must not be rankable")
	}
}
