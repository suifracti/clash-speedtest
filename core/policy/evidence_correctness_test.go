package policy

import (
	"strings"
	"testing"
	"time"
)

// =============================================================================
// Regression tests for the external PR#7 Policy Correctness Review.
//
// Each test here pins a defect that was reproduced against the reviewed head and
// is intentionally written to FAIL on the pre-fix code.
// =============================================================================

// -----------------------------------------------------------------------------
// BLOCKER-1: "current node failed + no eligible candidate" must not be reported
// as insufficient_evidence, which contradicts the same payload's
// evidence_sufficiency.sufficient == true.
// -----------------------------------------------------------------------------

func TestRecommendFromEvidence_NoEligibleCandidateIsNotInsufficientEvidence(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend

	hk := testNode("hk", "HK-01", "rev_a")
	old := testNode("old", "OLD-01", "rev_a")

	samples := map[string][]EvidenceSample{
		// Current node: 5 consecutive transport timeouts, no success at all.
		hk.SampleSetKey(): transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, false, 0, "timeout", "dial tcp: i/o timeout"),
		// The only alternative is stale, so it is rejected by the gate.
		old.SampleSetKey(): transportSeries(old, now, 20*time.Minute, 30*time.Second, 5, true, 10, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, old}, samples)

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})

	assertNonExecuting(t, rec)

	// The current node cleared every evidence gate — that is why execution reached candidate
	// comparison at all. So the payload must not claim the evidence was insufficient.
	if !rec.EvidenceSufficiency.Sufficient {
		t.Fatalf("expected the current node's evidence to be sufficient, got %+v", rec.EvidenceSufficiency)
	}
	if rec.Decision == DecisionInsufficientEvidence {
		t.Fatalf("decision must not be %q when evidence_sufficiency.sufficient is true: the "+
			"payload would contradict itself (reasons: %s)", DecisionInsufficientEvidence, dumpReasons(rec))
	}
	assertDecision(t, rec, DecisionNoEligibleCandidate)

	if rec.CurrentNode == nil || rec.CurrentNode.TransportHealthy {
		t.Fatalf("expected the current node to be transport-unhealthy, got %+v", rec.CurrentNode)
	}
	assertRejected(t, rec, "OLD-01", RejectStaleEvidence)
	assertReasonCode(t, rec, ReasonNoEligibleCandidate)
	// No switch is recommended, and nothing was executed.
	if rec.RecommendedNode != nil {
		t.Fatalf("no candidate passed the gate, so no node may be recommended, got %+v", rec.RecommendedNode)
	}
}

// Negative control: a genuinely thin evidence set must STILL be insufficient_evidence.
// The new verdict must not swallow the real "not enough data" case.
func TestRecommendFromEvidence_ThinEvidenceIsStillInsufficientEvidence(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend
	p.MinSampleCount = 3

	hk := testNode("hk", "HK-01", "rev_a")

	samples := map[string][]EvidenceSample{
		// A single sample is below MinSampleCount, so the current node's evidence is thin.
		hk.SampleSetKey(): transportSeries(hk, now, 10*time.Second, 30*time.Second, 1, false, 0, "timeout", "i/o timeout"),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk}, samples)

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})

	assertNonExecuting(t, rec)
	assertDecision(t, rec, DecisionInsufficientEvidence)
	if rec.EvidenceSufficiency.Sufficient {
		t.Fatalf("thin evidence must not be reported as sufficient, got %+v", rec.EvidenceSufficiency)
	}
}

// -----------------------------------------------------------------------------
// BLOCKER-2: a losing candidate's reason must never state the opposite of the
// numbers carried in its own evidence map.
// -----------------------------------------------------------------------------

func TestRecommendFromEvidence_LosingCandidateReasonNeverContradictsItsEvidence(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend

	hk := testNode("hk", "HK-01", "rev_a")
	mid := testNode("mid", "MID-20ms", "rev_a")
	fast := testNode("fast", "FAST-10ms", "rev_a")

	// MID is listed first and FAST is genuinely faster. The engine's running-best rule
	// (it only advances when the improvement clears HysteresisBuffer) therefore keeps MID,
	// which is a pre-existing engine property — but the explanation must say so instead of
	// claiming FAST is not better.
	samples := map[string][]EvidenceSample{
		hk.SampleSetKey():   transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 100, "", ""),
		mid.SampleSetKey():  transportSeries(mid, now, 10*time.Second, 30*time.Second, 5, true, 20, "", ""),
		fast.SampleSetKey(): transportSeries(fast, now, 10*time.Second, 30*time.Second, 5, true, 10, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, mid, fast}, samples)

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})

	assertNonExecuting(t, rec)
	assertDecision(t, rec, DecisionRecommendSwitch)

	// The genuinely faster candidate must NOT be described as "not better".
	rejection := assertRejected(t, rec, "FAST-10ms", RejectHysteresisNotMet)
	if strings.Contains(rejection.Reason, "未优于") {
		t.Fatalf("FAST-10ms is 10ms faster than the recommended node, so its reason must not "+
			"claim it is not better: %q", rejection.Reason)
	}
	if !strings.Contains(rejection.Reason, "确实优于") {
		t.Fatalf("expected an explicit statement that the candidate IS better: %q", rejection.Reason)
	}
	if rejection.Evidence["delta_vs_recommended"] != "10ms" {
		t.Fatalf("expected the real delta against the recommended node, got %v", rejection.Evidence["delta_vs_recommended"])
	}
	if rejection.Evidence["recommended_node"] != "MID-20ms" {
		t.Fatalf("expected the recommended node to be named in the evidence, got %v", rejection.Evidence["recommended_node"])
	}
}

// Positive control: when the losing candidate really is slower, the "not better" wording
// must be used and must agree with the quoted numbers.
func TestRecommendFromEvidence_SlowerLosingCandidateKeepsNotBestWording(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend

	hk := testNode("hk", "HK-01", "rev_a")
	fast := testNode("fast", "FAST-10ms", "rev_a")
	slow := testNode("slow", "SLOW-60ms", "rev_a")

	samples := map[string][]EvidenceSample{
		hk.SampleSetKey():   transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 100, "", ""),
		fast.SampleSetKey(): transportSeries(fast, now, 10*time.Second, 30*time.Second, 5, true, 10, "", ""),
		slow.SampleSetKey(): transportSeries(slow, now, 10*time.Second, 30*time.Second, 5, true, 60, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, fast, slow}, samples)

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})

	assertDecision(t, rec, DecisionRecommendSwitch)
	if rec.RecommendedNode == nil || rec.RecommendedNode.DisplayName != "FAST-10ms" {
		t.Fatalf("expected FAST-10ms to win, got %+v", rec.RecommendedNode)
	}
	rejection := assertRejected(t, rec, "SLOW-60ms", RejectNotBestCandidate)
	if !strings.Contains(rejection.Reason, "未优于") {
		t.Fatalf("a genuinely slower candidate must be reported as not better: %q", rejection.Reason)
	}
	if rejection.Evidence["delta_vs_recommended"] != "-50ms" {
		t.Fatalf("expected a negative delta against the recommended node, got %v", rejection.Evidence["delta_vs_recommended"])
	}
}

// -----------------------------------------------------------------------------
// RELEVANT-1: the printed transport failure ratio must use scope-matched counters.
// -----------------------------------------------------------------------------

func TestRecommendFromEvidence_TransportFailureRatioUsesMatchingScopes(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend

	n := testNode("n", "N-01", "rev_a")

	// 1 transport probe that failed, plus 5 service probes that failed with a plain timeout.
	// ClassifyFailure attributes the service timeouts to the transport layer, so the
	// aggregate counter is 6 while only 1 transport-scope sample exists.
	samples := append(
		transportSeries(n, now, 10*time.Second, 30*time.Second, 1, false, 0, "timeout", "i/o timeout"),
		serviceSeries(n, now, 10*time.Second, 30*time.Second, 5, "service_google", false, 0, "timeout", "i/o timeout", false)...,
	)
	snap := buildSnap(now, p, PurposeGeneral, n, []EvidenceNode{n}, map[string][]EvidenceSample{
		n.SampleSetKey(): samples,
	})

	cur := snap.CurrentNode()
	if cur == nil {
		t.Fatalf("current node unresolved")
	}
	if cur.TransportSampleCount != 1 || cur.TransportSuccessCount != 0 {
		t.Fatalf("unexpected transport scope counts: samples=%d successes=%d",
			cur.TransportSampleCount, cur.TransportSuccessCount)
	}
	if cur.TransportScopeFailureCount() != 1 {
		t.Fatalf("TransportScopeFailureCount must count transport-scope failures only, got %d",
			cur.TransportScopeFailureCount())
	}
	if cur.TransportFailureCount != 6 {
		t.Fatalf("the aggregate transport-attributed counter is expected to include the service "+
			"timeouts (6), got %d", cur.TransportFailureCount)
	}

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "N-01"}, snap, RecommendOptions{})
	assertNonExecuting(t, rec)

	down := assertReasonCode(t, rec, ReasonCurrentTransportDown)
	if !strings.Contains(down.Message, "1/1") {
		t.Fatalf("the ratio must be scope-matched (1/1), got %q", down.Message)
	}
	if strings.Contains(down.Message, "6/1") {
		t.Fatalf("a failure count that exceeds its own denominator must never be printed: %q", down.Message)
	}
	if down.Evidence["transport_attributed_failures"] != 6 {
		t.Fatalf("the aggregate counter must still be exposed for provenance, got %v",
			down.Evidence["transport_attributed_failures"])
	}
}

// -----------------------------------------------------------------------------
// RELEVANT-2: the reason text must match the cause that actually applied.
// -----------------------------------------------------------------------------

func TestRecommendFromEvidence_StayReasonMatchesActualCause(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()

	hk := testNode("hk", "HK-01", "rev_a")
	sg := testNode("sg", "SG-01", "rev_a")

	healthySamples := map[string][]EvidenceSample{
		hk.SampleSetKey(): transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 100, "", ""),
		sg.SampleSetKey(): transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 20, "", ""),
	}

	t.Run("cooldown must not be explained as no-candidate-met-thresholds", func(t *testing.T) {
		p := DefaultSwitchPolicy()
		p.Mode = ModeRecommend

		snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, sg}, healthySamples)
		// A switch happened one minute ago, well inside the 5m cooldown.
		state := &DecisionState{CurrentNode: "HK-01", LastSwitchAt: now.Add(-time.Minute)}
		rec := engine.RecommendFromEvidence(now, p, state, snap, RecommendOptions{})

		assertNonExecuting(t, rec)
		assertDecision(t, rec, DecisionStay)
		stay := assertReasonCode(t, rec, ReasonStayCurrentStable)
		if !strings.Contains(stay.Message, "冷却") {
			t.Fatalf("expected the cooldown to be named as the cause, got %q", stay.Message)
		}
		if strings.Contains(stay.Message, "没有候选节点满足") {
			t.Fatalf("no candidate comparison was performed during cooldown, so that claim is "+
				"fabricated: %q", stay.Message)
		}
	})

	t.Run("missing latency baseline must not be explained as no-candidate-met-thresholds", func(t *testing.T) {
		p := DefaultSwitchPolicy()
		p.Mode = ModeRecommend

		// Transport probes that "succeed" but carry no measurable latency.
		noBaseline := map[string][]EvidenceSample{
			hk.SampleSetKey(): transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 0, "", ""),
			sg.SampleSetKey(): transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 20, "", ""),
		}
		snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, sg}, noBaseline)
		rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})

		assertDecision(t, rec, DecisionStay)
		stay := assertReasonCode(t, rec, ReasonStayCurrentStable)
		if !strings.Contains(stay.Message, "延迟基线") {
			t.Fatalf("expected the missing latency baseline to be named as the cause, got %q", stay.Message)
		}
		if strings.Contains(stay.Message, "没有候选节点满足") {
			t.Fatalf("the engine never compared candidates without a baseline, so that claim is "+
				"fabricated: %q", stay.Message)
		}
	})

	t.Run("genuine no-candidate case keeps the threshold wording", func(t *testing.T) {
		p := DefaultSwitchPolicy()
		p.Mode = ModeRecommend

		// SG is only 5ms faster, far below MinImprovementRTT, so the engine really does
		// conclude that no candidate clears the thresholds.
		marginal := map[string][]EvidenceSample{
			hk.SampleSetKey(): transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 100, "", ""),
			sg.SampleSetKey(): transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 95, "", ""),
		}
		snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, sg}, marginal)
		rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})

		assertDecision(t, rec, DecisionStay)
		stay := assertReasonCode(t, rec, ReasonStayCurrentStable)
		if !strings.Contains(stay.Message, "没有候选节点满足") {
			t.Fatalf("expected the genuine threshold explanation, got %q", stay.Message)
		}
	})
}

func TestRecommendFromEvidence_FailoverTriggerDoesNotClaimThresholdsWereMet(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend

	hk := testNode("hk", "HK-01", "rev_a")
	sg := testNode("sg", "SG-01", "rev_a")

	// The current node has older successes (so it HAS a latency baseline, P50 = 100ms) but
	// the newest three transport probes failed, which trips MaxConsecutiveFailures.
	hkSamples := append(
		transportSeries(hk, now, 5*time.Second, 10*time.Second, 3, false, 0, "timeout", "i/o timeout"),
		transportSeries(hk, now, 60*time.Second, 10*time.Second, 5, true, 100, "", "")...,
	)

	samples := map[string][]EvidenceSample{
		hk.SampleSetKey(): hkSamples,
		sg.SampleSetKey(): transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 40, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, sg}, samples)

	cur := snap.CurrentNode()
	if cur == nil || cur.LatencyP50 <= 0 {
		t.Fatalf("test setup must leave the failing node with a latency baseline, got %+v", cur)
	}

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})

	assertNonExecuting(t, rec)
	assertDecision(t, rec, DecisionRecommendSwitch)

	engineReason := assertReasonCode(t, rec, ReasonEngineDecision)
	if engineReason.Evidence["trigger_type"] != "failure_failover" {
		t.Fatalf("expected a failure_failover trigger, got %+v", engineReason.Evidence)
	}

	// The urgent-failover path does not apply MinImprovementRTT / MinImprovementRatio, so the
	// recommendation must not claim those thresholds were satisfied.
	p50 := assertReasonCode(t, rec, ReasonRecommendedP50)
	if strings.Contains(p50.Message, "满足 MinImprovementRTT") {
		t.Fatalf("a failure_failover recommendation must not claim the optimisation thresholds "+
			"were met: %q", p50.Message)
	}
	if !strings.Contains(p50.Message, "故障切换") {
		t.Fatalf("expected the failover trigger to be named as the selection rule, got %q", p50.Message)
	}
	if p50.Evidence["thresholds_applied"] != false {
		t.Fatalf("thresholds_applied must be false on the failover path, got %v", p50.Evidence["thresholds_applied"])
	}
	if p50.Evidence["trigger_type"] != "failure_failover" {
		t.Fatalf("the reason must record the real trigger, got %v", p50.Evidence["trigger_type"])
	}
}

// A switch that IS driven by the optimisation path must keep claiming the thresholds,
// since the engine really did apply them there.
func TestRecommendFromEvidence_LatencyTriggerStillReportsThresholds(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend

	hk := testNode("hk", "HK-01", "rev_a")
	sg := testNode("sg", "SG-01", "rev_a")

	samples := map[string][]EvidenceSample{
		hk.SampleSetKey(): transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 100, "", ""),
		sg.SampleSetKey(): transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 20, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, sg}, samples)

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})

	assertDecision(t, rec, DecisionRecommendSwitch)
	engineReason := assertReasonCode(t, rec, ReasonEngineDecision)
	if engineReason.Evidence["trigger_type"] != "latency_improvement" {
		t.Fatalf("expected a latency_improvement trigger, got %+v", engineReason.Evidence)
	}
	p50 := assertReasonCode(t, rec, ReasonRecommendedP50)
	if !strings.Contains(p50.Message, "满足 MinImprovementRTT") {
		t.Fatalf("the optimisation path must report the thresholds it applied, got %q", p50.Message)
	}
	if p50.Evidence["thresholds_applied"] != true {
		t.Fatalf("thresholds_applied must be true on the optimisation path, got %v", p50.Evidence["thresholds_applied"])
	}
}

// -----------------------------------------------------------------------------
// RELEVANT-3: region-block detection must not fire on unrelated numbers that merely
// appear in free-text error detail.
// -----------------------------------------------------------------------------

func TestIsRegionRejection_RequiresAnExplicitStatusToken(t *testing.T) {
	cases := []struct {
		name     string
		class    string
		detail   string
		region   bool
		expected bool
	}{
		{"canonical 400", "http_status_error", "HTTP status 400 FAILED_PRECONDITION", false, true},
		{"canonical 403", "http_status_error", "HTTP status 403", false, true},
		{"canonical 451", "http_status_error", "HTTP status 451", false, true},
		{"status_code form", "http_status_error", "status_code=403", false, true},
		{"region flag wins", "http_status_error", "HTTP status 200", true, true},
		{"blocked class", "blocked", "", false, true},

		// Regression: a 500 whose detail merely contains "403" as part of a duration.
		{"500 with a 403 duration is not a region block", "http_status_error", "HTTP status 500 (took 403 ms)", false, false},
		{"plain 500 is not a region block", "http_status_error", "HTTP status 500", false, false},
		{"500 with a 400 port is not a region block", "http_status_error", "HTTP status 500 dialing 10.0.0.1:8400", false, false},
		{"bare number without a status token", "http_status_error", "upstream returned 451 after 4030ms", false, false},
		{"timeout class is never a region block", "timeout", "i/o timeout", false, false},
	}

	for _, tc := range cases {
		if got := isRegionRejection(tc.class, tc.detail, tc.region); got != tc.expected {
			t.Fatalf("%s: isRegionRejection(%q, %q, %v) = %v, want %v",
				tc.name, tc.class, tc.detail, tc.region, got, tc.expected)
		}
	}
}

// End-to-end: the false positive must not be able to hard-reject a node under AI purpose.
func TestClassifyFailure_NumericCollisionDoesNotHardRejectUnderAIPurpose(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend
	p.Purpose = PurposeAI

	hk := testNode("hk", "HK-01", "rev_a")
	// Fastest node, healthy transport, but a service probe returned 500 and its detail
	// happens to contain "403" as a duration. That must NOT be read as a region block.
	noisy := testNode("noisy", "NOISY-500", "rev_a")

	samples := map[string][]EvidenceSample{
		hk.SampleSetKey(): transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 100, "", ""),
		noisy.SampleSetKey(): append(
			transportSeries(noisy, now, 10*time.Second, 30*time.Second, 5, true, 20, "", ""),
			serviceSeries(noisy, now, 10*time.Second, 30*time.Second, 3, "service_google", false, 0,
				"http_status_error", "HTTP status 500 (took 403 ms)", false)...,
		),
	}
	snap := buildSnap(now, p, PurposeAI, hk, []EvidenceNode{hk, noisy}, samples)

	noisyEv := snap.Nodes[1]
	if noisyEv.BlockedForPurpose {
		t.Fatalf("a 500 whose detail contains \"403\" must not mark the node blocked for AI purpose")
	}
	if noisyEv.AIBlockCount != 0 {
		t.Fatalf("expected no AI block evidence, got %d", noisyEv.AIBlockCount)
	}

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})
	assertDecision(t, rec, DecisionRecommendSwitch)
	assertNoRejectionFor(t, rec, "NOISY-500")
	if rec.RecommendedNode == nil || rec.RecommendedNode.DisplayName != "NOISY-500" {
		t.Fatalf("the healthy fastest node must remain selectable under AI purpose, got %+v", rec.RecommendedNode)
	}
}
