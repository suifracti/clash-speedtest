package policy

import (
	"strings"
	"testing"
	"time"
)

// =============================================================================
// Regression tests for the three external review findings on PR#7.
//
//   1. ProfileID evidence isolation (fingerprint.go R-01).
//   2. monitor_only must not be silently bypassed; preview must be explicit.
//   3. MaxSampleAge is a freshness gate, NOT the observation window.
// =============================================================================

// -----------------------------------------------------------------------------
// Finding 1 — ProfileID evidence isolation.
//
// NodeIdentityKey is a pure transport endpoint and is explicitly NOT unique per logical
// node: two profiles may share the same CDN host / reverse-proxy endpoint. Evidence must
// therefore be qualified by ProfileID.
// -----------------------------------------------------------------------------

func TestRecommendFromEvidence_ProfileIsolation(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend

	hk := testNode("hk", "HK-01", "rev_a")
	sg := testNode("sg", "SG-01", "rev_a")

	// 20 samples of the SAME physical endpoint recorded under a DIFFERENT profile are
	// extremely fast (20ms). If they leaked in, the current node would look far better
	// than its own profile's evidence actually shows.
	otherProfile := profileSeries("prof-OTHER", transportSeries(hk, now, 10*time.Second, 30*time.Second, 20, true, 20, "", ""))
	ownProfile := transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 100, "", "")

	samples := map[string][]EvidenceSample{
		hk.SampleSetKey(): append(append([]EvidenceSample{}, otherProfile...), ownProfile...),
		sg.SampleSetKey(): transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 40, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, sg}, samples)

	cur := snap.CurrentNode()
	if cur == nil {
		t.Fatalf("expected current node evidence")
	}
	if cur.ProfileID != testProfile {
		t.Fatalf("expected the target profile %s, got %q", testProfile, cur.ProfileID)
	}
	if cur.ExcludedOtherProfileSamples != 20 {
		t.Fatalf("expected 20 cross-profile samples to be excluded, got %d", cur.ExcludedOtherProfileSamples)
	}
	if cur.SampleCount != 5 {
		t.Fatalf("expected only the 5 same-profile samples to be counted, got %d", cur.SampleCount)
	}
	if cur.LatencyP50 != 100*time.Millisecond {
		t.Fatalf("cross-profile latency leaked into the evidence: P50=%s", cur.LatencyP50)
	}

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})
	assertNonExecuting(t, rec)
	if rec.EvaluationMode != ModeRecommend {
		t.Fatalf("expected evaluation under ModeRecommend, got %s", rec.EvaluationMode)
	}
	assertDecision(t, rec, DecisionRecommendSwitch)
	if rec.CurrentNode.LatencyP50 != 100*time.Millisecond {
		t.Fatalf("recommendation must use same-profile evidence only, got %s", rec.CurrentNode.LatencyP50)
	}

	// The attribution must be explicit and auditable.
	profileReason := assertReasonCode(t, rec, ReasonProfileIsolation)
	if !strings.Contains(profileReason.Message, testProfile) {
		t.Fatalf("profile isolation reason must name the effective profile, got %q", profileReason.Message)
	}
	if profileReason.Evidence["excluded_other_profile_samples"] != 20 {
		t.Fatalf("expected the exclusion count in the reason evidence, got %+v", profileReason.Evidence)
	}
}

func TestRecommendFromEvidence_ProfileIsolationAmbiguous(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend

	// Target profile unknown, and the same transport endpoint carries two profiles:
	// attribution is impossible, so we must refuse to guess instead of averaging.
	shared := EvidenceNode{
		NodeKey:           "nk_shared",
		NodeIdentityKey:   "nid_shared",
		ConfigRevisionKey: "rev_a",
		DisplayName:       "SHARED",
	}
	sg := testNode("sg", "SG-01", "rev_a")

	sharedA := profileSeries("prof-A", transportSeries(shared, now, 10*time.Second, 30*time.Second, 5, true, 100, "", ""))
	sharedB := profileSeries("prof-B", transportSeries(shared, now, 10*time.Second, 30*time.Second, 5, true, 50, "", ""))

	samples := map[string][]EvidenceSample{
		shared.SampleSetKey(): append(append([]EvidenceSample{}, sharedA...), sharedB...),
		sg.SampleSetKey():     transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 40, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, shared, []EvidenceNode{shared, sg}, samples)

	cur := snap.CurrentNode()
	if cur == nil {
		t.Fatalf("expected current node evidence")
	}
	if !cur.ProfileIsolationAmbiguous {
		t.Fatalf("expected the profile isolation to be flagged ambiguous")
	}
	if cur.SampleCount != 0 {
		t.Fatalf("ambiguous attribution must not contribute samples, got %d", cur.SampleCount)
	}
	if cur.ExcludedOtherProfileSamples != 10 {
		t.Fatalf("expected 10 un-attributable samples to be excluded, got %d", cur.ExcludedOtherProfileSamples)
	}

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "SHARED"}, snap, RecommendOptions{})
	assertNonExecuting(t, rec)
	assertDecision(t, rec, DecisionInsufficientEvidence)
	if rec.RecommendedNode != nil {
		t.Fatalf("ambiguous profile attribution must never produce a recommendation")
	}
	found := false
	for _, code := range rec.EvidenceSufficiency.Reasons {
		if code == GateReasonProfileIsolationAmbiguous {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected %s in sufficiency reasons, got %+v", GateReasonProfileIsolationAmbiguous, rec.EvidenceSufficiency)
	}
}

func TestRecommendFromEvidence_OtherProfileOnlyEvidenceIsInsufficient(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend

	hk := testNode("hk", "HK-01", "rev_a")
	sg := testNode("sg", "SG-01", "rev_a")

	// Only another profile has evidence for this endpoint.
	otherProfile := profileSeries("prof-OTHER", transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 20, "", ""))

	samples := map[string][]EvidenceSample{
		hk.SampleSetKey(): otherProfile,
		sg.SampleSetKey(): transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 40, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, sg}, samples)

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})
	assertNonExecuting(t, rec)
	assertDecision(t, rec, DecisionInsufficientEvidence)
	if rec.RecommendedNode != nil {
		t.Fatalf("another profile's evidence must never produce a recommendation")
	}
	found := false
	for _, code := range rec.EvidenceSufficiency.Reasons {
		if code == GateReasonOtherProfileSamplesExcluded {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected %s in sufficiency reasons, got %+v", GateReasonOtherProfileSamplesExcluded, rec.EvidenceSufficiency)
	}
}

// TestBuildEvidenceSnapshot_ProfileIDInferredFromSingleObservedProfile covers the case
// where the caller does not know the profile: attribution is still safe when every
// observed sample agrees on one profile, and the inference is recorded as such.
func TestBuildEvidenceSnapshot_ProfileIDInferredFromSingleObservedProfile(t *testing.T) {
	now := time.Now()
	p := DefaultSwitchPolicy()

	node := EvidenceNode{
		NodeKey:           "nk_infer",
		NodeIdentityKey:   "nid_infer",
		ConfigRevisionKey: "rev_a",
		DisplayName:       "INFER",
	}
	samples := map[string][]EvidenceSample{
		node.SampleSetKey(): profileSeries("prof-only", transportSeries(node, now, 10*time.Second, 30*time.Second, 5, true, 50, "", "")),
	}

	snap := buildSnap(now, p, PurposeGeneral, node, []EvidenceNode{node}, samples)
	cur := snap.CurrentNode()
	if cur == nil {
		t.Fatalf("expected current node evidence")
	}
	if cur.ProfileID != "prof-only" || !cur.ProfileIDInferred {
		t.Fatalf("expected the single observed profile to be inferred, got %q inferred=%v", cur.ProfileID, cur.ProfileIDInferred)
	}
	if cur.SampleCount != 5 {
		t.Fatalf("expected the samples to be attributed, got %d", cur.SampleCount)
	}
}

// -----------------------------------------------------------------------------
// Finding 2 — the configured mode must never be silently bypassed.
// -----------------------------------------------------------------------------

func TestRecommendFromEvidence_MonitorOnlySuppressesUnlessPreview(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeMonitorOnly

	hk := testNode("hk", "HK-01", "rev_a")
	sg := testNode("sg", "SG-01", "rev_a")
	samples := map[string][]EvidenceSample{
		hk.SampleSetKey(): transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 200, "", ""),
		sg.SampleSetKey(): transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 20, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, sg}, samples)
	state := &DecisionState{CurrentNode: "HK-01"}

	// 1. Default: the configured mode is respected — no recommendation is produced.
	suppressed := engine.RecommendFromEvidence(now, p, state, snap, RecommendOptions{})
	assertNonExecuting(t, suppressed)
	if !suppressed.RecommendationSuppressed {
		t.Fatalf("monitor_only must suppress the recommendation by default")
	}
	if suppressed.SuppressedReason != SuppressReasonMonitorOnly {
		t.Fatalf("expected suppressed reason %s, got %q", SuppressReasonMonitorOnly, suppressed.SuppressedReason)
	}
	if suppressed.EvaluationMode != ModeMonitorOnly {
		t.Fatalf("expected EvaluationMode monitor_only when suppressed, got %s", suppressed.EvaluationMode)
	}
	if suppressed.Preview {
		t.Fatalf("a default evaluation must not be flagged as preview")
	}
	if suppressed.RecommendedNode != nil {
		t.Fatalf("monitor_only must not recommend a node, got %+v", suppressed.RecommendedNode)
	}
	if len(suppressed.RejectedCandidates) != 0 {
		t.Fatalf("monitor_only must not compare candidates, got %+v", suppressed.RejectedCandidates)
	}
	assertReasonCode(t, suppressed, ReasonMonitorOnlySuppresses)
	// Telemetry is still reported: monitor_only collects data, it just does not recommend.
	if suppressed.CurrentNode == nil || suppressed.SampleCount != 5 {
		t.Fatalf("expected current-node telemetry to still be reported, got %+v", suppressed.CurrentNode)
	}

	// 2. Explicit preview: the override is visible and auditable, never silent.
	preview := engine.RecommendFromEvidence(now, p, state, snap, RecommendOptions{Preview: true})
	assertNonExecuting(t, preview)
	if preview.RecommendationSuppressed {
		t.Fatalf("an explicit preview must not be suppressed")
	}
	if !preview.Preview {
		t.Fatalf("expected the result to be flagged as preview")
	}
	if preview.EvaluationMode != ModeRecommend {
		t.Fatalf("expected EvaluationMode recommend for a preview, got %s", preview.EvaluationMode)
	}
	if preview.ConfiguredMode != ModeMonitorOnly {
		t.Fatalf("expected the configured mode to still be recorded, got %s", preview.ConfiguredMode)
	}
	assertReasonCode(t, preview, ReasonPreviewNotice)
	assertDecision(t, preview, DecisionRecommendSwitch)
	if preview.RecommendedNode == nil || preview.RecommendedNode.DisplayName != "SG-01" {
		t.Fatalf("expected SG-01 in the preview, got %+v", preview.RecommendedNode)
	}
}

func TestRecommendFromEvidence_ConfiguredRecommendModeNeedsNoPreview(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend

	hk := testNode("hk", "HK-01", "rev_a")
	sg := testNode("sg", "SG-01", "rev_a")
	samples := map[string][]EvidenceSample{
		hk.SampleSetKey(): transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 200, "", ""),
		sg.SampleSetKey(): transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 20, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, sg}, samples)

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})
	assertNonExecuting(t, rec)
	if rec.RecommendationSuppressed || rec.Preview {
		t.Fatalf("recommend mode must evaluate directly, without suppression or preview")
	}
	if rec.EvaluationMode != ModeRecommend {
		t.Fatalf("expected EvaluationMode recommend, got %s", rec.EvaluationMode)
	}
	assertDecision(t, rec, DecisionRecommendSwitch)
	assertNoReasonCode(t, rec, ReasonPreviewNotice)
}

func TestRecommendFromEvidence_AutoModeIsAdvisoryOnly(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeAuto

	hk := testNode("hk", "HK-01", "rev_a")
	sg := testNode("sg", "SG-01", "rev_a")
	samples := map[string][]EvidenceSample{
		hk.SampleSetKey(): transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 200, "", ""),
		sg.SampleSetKey(): transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 20, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, sg}, samples)

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})
	assertNonExecuting(t, rec)
	assertDecision(t, rec, DecisionRecommendSwitch)
	assertReasonCode(t, rec, ReasonAutoNotImplemented)
	if rec.Executed || rec.SelectNodeCalls != 0 {
		t.Fatalf("auto mode must remain advisory-only in PR#7")
	}
}

// -----------------------------------------------------------------------------
// Finding 3 — MaxSampleAge is a freshness gate, not the observation window.
//
// A node whose newest sample is brand new must not be gated as insufficient merely
// because MaxSampleAge is small compared to the required observation window.
// -----------------------------------------------------------------------------

func TestRecommendFromEvidence_MaxSampleAgeIsFreshnessGateNotObservationWindow(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend
	p.MaxSampleAge = 5 * time.Minute
	// The required observation is far longer than MaxSampleAge. If MaxSampleAge were used
	// as the scan window, this node could NEVER satisfy MinObservationWindow — a guaranteed
	// false negative.
	p.MinObservationWindow = 30 * time.Minute
	p.MinSampleCount = 3

	hk := testNode("hk", "HK-01", "rev_a")
	sg := testNode("sg", "SG-01", "rev_a")

	// 41 samples one minute apart, newest only 10s old → 40m of observation.
	samples := map[string][]EvidenceSample{
		hk.SampleSetKey(): transportSeries(hk, now, 10*time.Second, time.Minute, 41, true, 100, "", ""),
		sg.SampleSetKey(): transportSeries(sg, now, 10*time.Second, time.Minute, 41, true, 40, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, sg}, samples)

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})
	assertNonExecuting(t, rec)

	// The scan window must be driven by the observation requirement, not MaxSampleAge.
	if rec.Gate.EvidenceWindow == p.MaxSampleAge {
		t.Fatalf("the evidence window must not equal MaxSampleAge (%s)", p.MaxSampleAge)
	}
	expectedWindow := DefaultEvidenceWindow(p)
	if rec.Gate.EvidenceWindow != expectedWindow {
		t.Fatalf("expected evidence window %s, got %s", expectedWindow, rec.Gate.EvidenceWindow)
	}
	if expectedWindow < 4*p.MinObservationWindow {
		t.Fatalf("expected the window to carry headroom over MinObservationWindow, got %s", expectedWindow)
	}

	cur := rec.Snapshot.CurrentNode()
	if cur == nil {
		t.Fatalf("expected current node evidence")
	}
	if cur.SampleCount != 41 {
		t.Fatalf("expected all 41 samples inside the scan window, got %d", cur.SampleCount)
	}
	if cur.ObservationWindow != 40*time.Minute {
		t.Fatalf("expected a 40m observation window, got %s", cur.ObservationWindow)
	}
	if cur.Freshness != FreshnessFresh {
		t.Fatalf("expected fresh (newest sample 10s old), got %s", cur.Freshness)
	}
	if !cur.Sufficiency.Sufficient {
		t.Fatalf("a fresh node with a long enough history must pass the gate, got %+v", cur.Sufficiency)
	}

	assertDecision(t, rec, DecisionRecommendSwitch)
	if rec.RecommendedNode == nil || rec.RecommendedNode.DisplayName != "SG-01" {
		t.Fatalf("expected SG-01, got %+v", rec.RecommendedNode)
	}
}

func TestRecommendFromEvidence_StaleLatestSampleStillGatesEvenWithLongHistory(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend
	p.MaxSampleAge = 5 * time.Minute

	hk := testNode("hk", "HK-01", "rev_a")
	sg := testNode("sg", "SG-01", "rev_a")

	// Long history, but the newest sample is 20 minutes old → stale, must be gated.
	samples := map[string][]EvidenceSample{
		hk.SampleSetKey(): transportSeries(hk, now, 20*time.Minute, time.Minute, 41, true, 100, "", ""),
		sg.SampleSetKey(): transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 40, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, sg}, samples)

	cur := snap.CurrentNode()
	if cur == nil {
		t.Fatalf("expected current node evidence")
	}
	if cur.SampleCount != 41 {
		t.Fatalf("expected the long history to still be scanned, got %d samples", cur.SampleCount)
	}
	if cur.Freshness != FreshnessStale {
		t.Fatalf("expected stale freshness from the newest sample age, got %s", cur.Freshness)
	}

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})
	assertNonExecuting(t, rec)
	assertDecision(t, rec, DecisionInsufficientEvidence)
	assertReasonCode(t, rec, GateReasonStaleEvidence)
}

func TestDefaultEvidenceWindow_NotDerivedFromMaxSampleAgeAlone(t *testing.T) {
	p := DefaultSwitchPolicy()
	p.MaxSampleAge = 5 * time.Minute
	p.MinObservationWindow = 1 * time.Minute
	if got := DefaultEvidenceWindow(p); got != time.Hour {
		t.Fatalf("expected the 1h floor, got %s", got)
	}

	p.MinObservationWindow = 30 * time.Minute
	if got := DefaultEvidenceWindow(p); got != 2*time.Hour {
		t.Fatalf("expected 4x MinObservationWindow = 2h, got %s", got)
	}

	// The cap bounds the scan, but it must never make an explicit requirement unsatisfiable:
	// with a 10h requirement the 24h cap still covers it.
	p.MinObservationWindow = 10 * time.Hour
	got := DefaultEvidenceWindow(p)
	if got != 24*time.Hour {
		t.Fatalf("expected the 24h cap to bound a 40h candidate, got %s", got)
	}
	if got < p.MinObservationWindow {
		t.Fatalf("the window must never be smaller than MinObservationWindow: %s < %s", got, p.MinObservationWindow)
	}

	// Beyond the cap, the explicit requirement wins over the safety default.
	p.MinObservationWindow = 100 * time.Hour
	if got := DefaultEvidenceWindow(p); got != 100*time.Hour {
		t.Fatalf("expected MinObservationWindow to win over the cap, got %s", got)
	}
}
