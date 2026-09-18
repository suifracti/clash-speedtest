package policy

import (
	"strings"
	"testing"
	"time"
)

// -----------------------------------------------------------------------------
// Test helpers
// -----------------------------------------------------------------------------

// testProfile is the default ProfileID used by the test helpers. ProfileID is part of
// the evidence isolation key: samples from the same physical endpoint under a different
// profile must never be aggregated.
const testProfile = "prof-test"

func testNode(id, name, rev string) EvidenceNode {
	return EvidenceNode{
		ProfileID:         testProfile,
		NodeKey:           "nk_" + id,
		NodeIdentityKey:   "nid_" + id,
		ConfigRevisionKey: rev,
		DisplayName:       name,
	}
}

// transportSeries builds `count` transport probes ending `age` before now, spaced `step` apart.
func transportSeries(n EvidenceNode, now time.Time, age, step time.Duration, count int, success bool, latencyMs int, errClass, errDetail string) []EvidenceSample {
	out := make([]EvidenceSample, 0, count)
	for i := 0; i < count; i++ {
		ts := now.Add(-age).Add(-time.Duration(count-1-i) * step)
		out = append(out, EvidenceSample{
			ProfileID:         n.ProfileID,
			NodeKey:           n.NodeKey,
			NodeIdentityKey:   n.NodeIdentityKey,
			ConfigRevisionKey: n.ConfigRevisionKey,
			DisplayName:       n.DisplayName,
			ProbeType:         "rtt",
			Target:            "https://cp.cloudflare.com/generate_204",
			Timestamp:         ts,
			Success:           success,
			Latency:           time.Duration(latencyMs) * time.Millisecond,
			TTFB:              time.Duration(latencyMs) * time.Millisecond,
			ErrorClass:        errClass,
			ErrorDetail:       errDetail,
		})
	}
	return out
}

// serviceSeries builds `count` service-scope probes (e.g. service_google).
func serviceSeries(n EvidenceNode, now time.Time, age, step time.Duration, count int, probeType string, success bool, latencyMs int, errClass, errDetail string, regionBlocked bool) []EvidenceSample {
	out := make([]EvidenceSample, 0, count)
	for i := 0; i < count; i++ {
		ts := now.Add(-age).Add(-time.Duration(count-1-i) * step)
		out = append(out, EvidenceSample{
			ProfileID:         n.ProfileID,
			NodeKey:           n.NodeKey,
			NodeIdentityKey:   n.NodeIdentityKey,
			ConfigRevisionKey: n.ConfigRevisionKey,
			DisplayName:       n.DisplayName,
			ProbeType:         probeType,
			Target:            "https://www.google.com/generate_204",
			Timestamp:         ts,
			Success:           success,
			Latency:           time.Duration(latencyMs) * time.Millisecond,
			TTFB:              time.Duration(latencyMs) * time.Millisecond,
			ErrorClass:        errClass,
			ErrorDetail:       errDetail,
			RegionBlocked:     regionBlocked,
		})
	}
	return out
}

// profileSeries stamps the same probe series with an explicit ProfileID, modelling the
// same physical endpoint observed under a different subscription/profile.
func profileSeries(profile string, samples []EvidenceSample) []EvidenceSample {
	out := make([]EvidenceSample, len(samples))
	copy(out, samples)
	for i := range out {
		out[i].ProfileID = profile
	}
	return out
}

// revisionSeries stamps the same probe series with an explicit config revision.
func revisionSeries(n EvidenceNode, rev string, samples []EvidenceSample) []EvidenceSample {
	out := make([]EvidenceSample, len(samples))
	copy(out, samples)
	for i := range out {
		out[i].ConfigRevisionKey = rev
	}
	return out
}

func countSamples(m map[string][]EvidenceSample) int {
	total := 0
	for _, v := range m {
		total += len(v)
	}
	return total
}

func buildSnap(now time.Time, p SwitchPolicy, purpose PolicyPurpose, current EvidenceNode, nodes []EvidenceNode, samples map[string][]EvidenceSample) EvidenceSnapshot {
	return BuildEvidenceSnapshot(EvidenceInput{
		Now:     now,
		Purpose: purpose,
		Policy:  p,
		Source: EvidenceSource{
			JobID:         "job-1",
			ProfileID:     testProfile,
			ProbeSet:      "service",
			NodeSetSource: "monitor_job",
			LookbackSince: now.Add(-time.Hour),
			LookbackUntil: now,
		},
		CurrentNodeKey:         current.NodeKey,
		CurrentNodeIdentityKey: current.NodeIdentityKey,
		Nodes:                  nodes,
		SamplesByNode:          samples,
		RawSampleCount:         countSamples(samples),
	})
}

func assertDecision(t *testing.T, rec MonitorRecommendation, want RecommendationDecision) {
	t.Helper()
	if rec.Decision != want {
		t.Fatalf("expected decision %q, got %q (reasons: %s)", want, rec.Decision, dumpReasons(rec))
	}
}

func assertReasonCode(t *testing.T, rec MonitorRecommendation, code string) RecommendationReason {
	t.Helper()
	for _, r := range rec.Reasons {
		if r.Code == code {
			return r
		}
	}
	t.Fatalf("expected reason code %q, got: %s", code, dumpReasons(rec))
	return RecommendationReason{}
}

func assertNoReasonCode(t *testing.T, rec MonitorRecommendation, code string) {
	t.Helper()
	for _, r := range rec.Reasons {
		if r.Code == code {
			t.Fatalf("did not expect reason code %q, but found: %s", code, r.Message)
		}
	}
}

func assertRejected(t *testing.T, rec MonitorRecommendation, nodeName, code string) CandidateRejection {
	t.Helper()
	for _, r := range rec.RejectedCandidates {
		if r.NodeName == nodeName && r.Code == code {
			return r
		}
	}
	var got []string
	for _, r := range rec.RejectedCandidates {
		got = append(got, r.NodeName+":"+r.Code)
	}
	t.Fatalf("expected rejection %s/%s, got: %v", nodeName, code, got)
	return CandidateRejection{}
}

func assertNoRejectionFor(t *testing.T, rec MonitorRecommendation, nodeName string) {
	t.Helper()
	for _, r := range rec.RejectedCandidates {
		if r.NodeName == nodeName {
			t.Fatalf("did not expect %s to be rejected, but got %s: %s", nodeName, r.Code, r.Reason)
		}
	}
}

func dumpReasons(rec MonitorRecommendation) string {
	var sb strings.Builder
	for _, r := range rec.Reasons {
		sb.WriteString("\n  - [" + r.Severity + "] " + r.Code + ": " + r.Message)
	}
	return sb.String()
}

// assertNonExecuting proves the structural guarantees of PR#7 on any result.
func assertNonExecuting(t *testing.T, rec MonitorRecommendation) {
	t.Helper()
	if rec.SelectNodeCalls != 0 {
		t.Fatalf("SelectNode must never be called from the evidence path, got %d calls", rec.SelectNodeCalls)
	}
	if rec.Executed {
		t.Fatalf("evidence path must never execute a switch")
	}
	if !rec.AdvisoryOnly {
		t.Fatalf("evidence path must always be advisory-only")
	}
	if rec.AutoImplemented {
		t.Fatalf("Auto execution must remain unimplemented in PR#7")
	}
	if rec.Snapshot == nil {
		t.Fatalf("recommendation must preserve its evidence snapshot provenance")
	}
	// EvaluationMode is asserted per test: it is ModeMonitorOnly when the configured mode
	// suppressed the recommendation, and ModeRecommend when an evaluation actually ran.
}

// -----------------------------------------------------------------------------
// 1. fresh + sufficient evidence → recommend_switch
// -----------------------------------------------------------------------------

func TestRecommendFromEvidence_FreshSufficientEvidenceRecommends(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend

	hk := testNode("hk", "HK-01", "rev_a")
	sg := testNode("sg", "SG-01", "rev_a")

	samples := map[string][]EvidenceSample{
		hk.NodeKey: transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 100, "", ""),
		sg.NodeKey: transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 40, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, sg}, samples)

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})

	assertDecision(t, rec, DecisionRecommendSwitch)
	assertNonExecuting(t, rec)

	if rec.RecommendedNode == nil || rec.RecommendedNode.DisplayName != "SG-01" {
		t.Fatalf("expected SG-01 to be recommended, got %+v", rec.RecommendedNode)
	}
	if rec.CurrentNode == nil || rec.CurrentNode.DisplayName != "HK-01" {
		t.Fatalf("expected HK-01 as current node, got %+v", rec.CurrentNode)
	}
	if rec.Freshness != FreshnessFresh {
		t.Fatalf("expected fresh evidence, got %s", rec.Freshness)
	}
	if rec.SampleCount != 5 {
		t.Fatalf("expected 5 samples, got %d", rec.SampleCount)
	}
	if rec.ObservationWindow != 2*time.Minute {
		t.Fatalf("expected 2m observation window, got %s", rec.ObservationWindow)
	}
	if rec.Confidence <= 0 || rec.Confidence > 1 {
		t.Fatalf("expected confidence in (0,1], got %f", rec.Confidence)
	}
	if rec.ConfidenceBasis.Formula == "" || rec.ConfidenceBasis.Detail == "" {
		t.Fatalf("confidence must be explained, got %+v", rec.ConfidenceBasis)
	}
	if !rec.EvidenceSufficiency.Sufficient {
		t.Fatalf("expected sufficient evidence, got %+v", rec.EvidenceSufficiency)
	}

	// Explainability: the reason must be derivable from real evidence, not UI copy.
	assertReasonCode(t, rec, ReasonRecommendedSuccessRate)
	assertReasonCode(t, rec, ReasonRecommendedP50)
	p95 := assertReasonCode(t, rec, ReasonRecommendedP95)
	if p95.Evidence["target_p95"] == nil || p95.Evidence["current_p95"] == nil {
		t.Fatalf("P95 reason must carry real evidence, got %+v", p95.Evidence)
	}
	if !strings.Contains(p95.Message, "低") {
		t.Fatalf("expected P95 improvement wording, got %q", p95.Message)
	}
	// A healthy current node with no failure streak must not invent a failure reason.
	assertNoReasonCode(t, rec, ReasonCurrentConsecutiveFails)
	assertNoReasonCode(t, rec, ReasonCurrentTransportDown)
}

// -----------------------------------------------------------------------------
// 2. insufficient sample count → insufficient_evidence
// -----------------------------------------------------------------------------

func TestRecommendFromEvidence_InsufficientSampleCount(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend

	hk := testNode("hk", "HK-01", "rev_a")
	sg := testNode("sg", "SG-01", "rev_a")

	samples := map[string][]EvidenceSample{
		// only 2 samples, MinSampleCount defaults to 3
		hk.NodeKey: transportSeries(hk, now, 10*time.Second, 30*time.Second, 2, true, 100, "", ""),
		sg.NodeKey: transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 40, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, sg}, samples)

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})

	assertDecision(t, rec, DecisionInsufficientEvidence)
	assertNonExecuting(t, rec)
	if rec.RecommendedNode != nil {
		t.Fatalf("insufficient evidence must never produce a recommended node, got %+v", rec.RecommendedNode)
	}
	assertReasonCode(t, rec, ReasonInsufficientEvidence)
	assertReasonCode(t, rec, GateReasonInsufficientSampleCount)
	if rec.Confidence != 0 {
		t.Fatalf("insufficient evidence must yield zero confidence, got %f", rec.Confidence)
	}
	if rec.EvidenceSufficiency.Sufficient {
		t.Fatalf("expected insufficient sufficiency, got %+v", rec.EvidenceSufficiency)
	}
}

// -----------------------------------------------------------------------------
// 3. stale evidence → insufficient_evidence
// -----------------------------------------------------------------------------

func TestRecommendFromEvidence_StaleEvidence(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend

	hk := testNode("hk", "HK-01", "rev_a")
	sg := testNode("sg", "SG-01", "rev_a")

	samples := map[string][]EvidenceSample{
		// newest sample is 20 minutes old, MaxSampleAge defaults to 5m
		hk.NodeKey: transportSeries(hk, now, 20*time.Minute, 30*time.Second, 5, true, 100, "", ""),
		sg.NodeKey: transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 40, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, sg}, samples)

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})

	assertDecision(t, rec, DecisionInsufficientEvidence)
	assertNonExecuting(t, rec)
	if rec.Freshness != FreshnessStale {
		t.Fatalf("expected stale freshness, got %s", rec.Freshness)
	}
	assertReasonCode(t, rec, GateReasonStaleEvidence)
	if rec.RecommendedNode != nil {
		t.Fatalf("stale data must never drive a recommendation")
	}
	// The staleness reason must state the real age and the applied threshold.
	for _, r := range rec.Reasons {
		if r.Code == GateReasonStaleEvidence {
			if !strings.Contains(r.Message, "MaxSampleAge") {
				t.Fatalf("stale reason must name the applied threshold, got %q", r.Message)
			}
		}
	}
}

// -----------------------------------------------------------------------------
// 4. current node healthy → stay
// -----------------------------------------------------------------------------

func TestRecommendFromEvidence_CurrentHealthyStays(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend

	hk := testNode("hk", "HK-01", "rev_a")
	sg := testNode("sg", "SG-01", "rev_a")

	samples := map[string][]EvidenceSample{
		hk.NodeKey: transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 100, "", ""),
		// 95ms is only 5ms better: below MinImprovementRTT (30ms) and below the
		// HysteresisBuffer, so the anti-flapping rules must keep the current node.
		sg.NodeKey: transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 95, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, sg}, samples)

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})

	assertDecision(t, rec, DecisionStay)
	assertNonExecuting(t, rec)
	if rec.RecommendedNode != nil {
		t.Fatalf("stay must not carry a recommended node, got %+v", rec.RecommendedNode)
	}
	assertReasonCode(t, rec, ReasonStayCurrentStable)
	assertReasonCode(t, rec, ReasonCurrentSuccessRate)
	assertReasonCode(t, rec, ReasonEngineDecision)
	if rec.CurrentNode == nil || !rec.CurrentNode.TransportHealthy {
		t.Fatalf("expected a healthy current node, got %+v", rec.CurrentNode)
	}
}

// -----------------------------------------------------------------------------
// 5. repeated transport failure → candidate recommendation
// -----------------------------------------------------------------------------

func TestRecommendFromEvidence_RepeatedTransportFailureRecommendsCandidate(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend

	hk := testNode("hk", "HK-01", "rev_a")
	sg := testNode("sg", "SG-01", "rev_a")

	samples := map[string][]EvidenceSample{
		// 5 consecutive timeouts >= MaxConsecutiveFailures (3)
		hk.NodeKey: transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, false, 5000, "timeout", "dial tcp: i/o timeout"),
		sg.NodeKey: transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 40, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, sg}, samples)

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})

	assertDecision(t, rec, DecisionRecommendSwitch)
	assertNonExecuting(t, rec)
	if rec.RecommendedNode == nil || rec.RecommendedNode.DisplayName != "SG-01" {
		t.Fatalf("expected SG-01 failover recommendation, got %+v", rec.RecommendedNode)
	}

	consecutive := assertReasonCode(t, rec, ReasonCurrentConsecutiveFails)
	if !strings.Contains(consecutive.Message, "连续 5 次") {
		t.Fatalf("expected the real consecutive failure count in the reason, got %q", consecutive.Message)
	}
	assertReasonCode(t, rec, ReasonCurrentTransportDown)

	engineReason := assertReasonCode(t, rec, ReasonEngineDecision)
	if engineReason.Evidence["trigger_type"] != "failure_failover" {
		t.Fatalf("expected failure_failover trigger, got %+v", engineReason.Evidence)
	}
	// The recommendation must never claim to have executed anything.
	if rec.Executed || rec.SelectNodeCalls != 0 {
		t.Fatalf("failover recommendation must remain non-executing")
	}
}

// -----------------------------------------------------------------------------
// 6. service-only failure under General purpose must NOT be read as node failure
// -----------------------------------------------------------------------------

func TestRecommendFromEvidence_ServiceOnlyFailureUnderGeneralIsNotTransportFailure(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend
	p.Purpose = PurposeGeneral

	hk := testNode("hk", "HK-01", "rev_a")
	sg := testNode("sg", "SG-01", "rev_a")

	transportHK := transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 100, "", "")
	blockedGoogle := serviceSeries(hk, now, 10*time.Second, 30*time.Second, 3, "service_google", false, 0, "blocked", "HTTP status 400 FAILED_PRECONDITION", true)

	samples := map[string][]EvidenceSample{
		hk.NodeKey: append(append([]EvidenceSample{}, transportHK...), blockedGoogle...),
		sg.NodeKey: transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 95, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, sg}, samples)

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})

	assertNonExecuting(t, rec)
	assertDecision(t, rec, DecisionStay)

	cur := rec.Snapshot.CurrentNode()
	if cur == nil {
		t.Fatalf("expected current node evidence")
	}
	if !cur.TransportHealthy {
		t.Fatalf("service-only blocking under general purpose must NOT mark the node transport-unhealthy")
	}
	if cur.BlockedForPurpose {
		t.Fatalf("general purpose must never mark a node blocked_for_purpose from service-level evidence")
	}
	if cur.TransportFailureCount != 0 {
		t.Fatalf("service-level blocking must not be counted as transport failure, got %d", cur.TransportFailureCount)
	}
	if cur.ServiceFailureCount != 3 {
		t.Fatalf("expected 3 service-level blocks, got %d", cur.ServiceFailureCount)
	}
	if cur.TriageStatus != "ok" {
		t.Fatalf("expected triage ok under general purpose, got %s", cur.TriageStatus)
	}
	assertNoReasonCode(t, rec, ReasonCurrentTransportDown)
	serviceOnly := assertReasonCode(t, rec, ReasonCurrentServiceOnlyFail)
	if !strings.Contains(serviceOnly.Message, "不计为节点故障") {
		t.Fatalf("expected an explicit 'not a node failure' explanation, got %q", serviceOnly.Message)
	}
}

// -----------------------------------------------------------------------------
// 7. AI block under AI purpose is correctly eliminated
// -----------------------------------------------------------------------------

func TestRecommendFromEvidence_AIBlockUnderAIPurposeIsEliminated(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()

	hk := testNode("hk", "HK-01", "rev_a")
	aiBlocked := testNode("aib", "AI-BLOCKED", "rev_a")
	sg := testNode("sg", "SG-01", "rev_a")

	build := func(purpose PolicyPurpose) MonitorRecommendation {
		p := DefaultSwitchPolicy()
		p.Mode = ModeRecommend
		p.Purpose = purpose

		hkSamples := append(
			append([]EvidenceSample{}, transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 100, "", "")...),
			serviceSeries(hk, now, 10*time.Second, 30*time.Second, 3, "service_google", false, 0, "blocked", "HTTP status 400 FAILED_PRECONDITION", true)...,
		)
		// AI-BLOCKED is the fastest node, but carries confirmed AI region blocking.
		aibSamples := append(
			append([]EvidenceSample{}, transportSeries(aiBlocked, now, 10*time.Second, 30*time.Second, 5, true, 30, "", "")...),
			serviceSeries(aiBlocked, now, 10*time.Second, 30*time.Second, 3, "ai_check", false, 0, "blocked", "HTTP status 400 FAILED_PRECONDITION", true)...,
		)

		samples := map[string][]EvidenceSample{
			hk.NodeKey:        hkSamples,
			aiBlocked.NodeKey: aibSamples,
			sg.NodeKey:        transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 60, "", ""),
		}
		snap := buildSnap(now, p, purpose, hk, []EvidenceNode{hk, aiBlocked, sg}, samples)
		return engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})
	}

	// --- Under AI purpose: the blocked node is eliminated even though it is fastest. ---
	aiRec := build(PurposeAI)
	assertNonExecuting(t, aiRec)
	assertDecision(t, aiRec, DecisionRecommendSwitch)
	assertRejected(t, aiRec, "AI-BLOCKED", RejectAIRegionBlocked)
	if aiRec.RecommendedNode == nil || aiRec.RecommendedNode.DisplayName != "SG-01" {
		t.Fatalf("expected SG-01 to be recommended under AI purpose, got %+v", aiRec.RecommendedNode)
	}
	aiRejection := assertRejected(t, aiRec, "AI-BLOCKED", RejectAIRegionBlocked)
	if aiRejection.Evidence["purpose"] != string(PurposeAI) {
		t.Fatalf("AI rejection must record the purpose lens, got %+v", aiRejection.Evidence)
	}

	// --- Under general purpose: the same blocking is NOT disqualifying. ---
	generalRec := build(PurposeGeneral)
	assertNonExecuting(t, generalRec)
	assertDecision(t, generalRec, DecisionRecommendSwitch)
	assertNoRejectionFor(t, generalRec, "AI-BLOCKED")
	if generalRec.RecommendedNode == nil || generalRec.RecommendedNode.DisplayName != "AI-BLOCKED" {
		t.Fatalf("under general purpose the fastest node must still win, got %+v", generalRec.RecommendedNode)
	}
}

// -----------------------------------------------------------------------------
// 8. P95 / success-rate evidence participates in the reasons
// -----------------------------------------------------------------------------

func TestRecommendFromEvidence_SuccessRateAndP95DriveReasons(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend
	p.MinSampleCount = 2
	p.MinObservationWindow = 0

	hk := testNode("hk", "HK-01", "rev_a")
	sg := testNode("sg", "SG-01", "rev_a")

	// HK: 50% success rate, wide latency spread (P95 much worse than P50).
	hkSamples := append(
		transportSeries(hk, now, 10*time.Second, 20*time.Second, 3, true, 300, "", ""),
		transportSeries(hk, now, 10*time.Second, 20*time.Second, 3, false, 0, "timeout", "timeout")...,
	)
	hkSamples = append(hkSamples, EvidenceSample{
		ProfileID: hk.ProfileID,
		NodeKey:   hk.NodeKey, NodeIdentityKey: hk.NodeIdentityKey, ConfigRevisionKey: hk.ConfigRevisionKey,
		DisplayName: hk.DisplayName, ProbeType: "rtt", Target: "https://cp.cloudflare.com/generate_204",
		Timestamp: now.Add(-10 * time.Second), Success: true, Latency: 900 * time.Millisecond, TTFB: 900 * time.Millisecond,
	})

	// SG: 100% success rate, tight latency.
	sgSamples := transportSeries(sg, now, 10*time.Second, 20*time.Second, 7, true, 60, "", "")

	samples := map[string][]EvidenceSample{hk.NodeKey: hkSamples, sg.NodeKey: sgSamples}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, sg}, samples)

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})
	assertNonExecuting(t, rec)

	if rec.CurrentNode == nil {
		t.Fatalf("expected current node evidence")
	}
	if rec.CurrentNode.SuccessRate >= 1.0 {
		t.Fatalf("expected a degraded success rate for the current node, got %f", rec.CurrentNode.SuccessRate)
	}
	if rec.CurrentNode.LatencyP95 <= rec.CurrentNode.LatencyP50 {
		t.Fatalf("expected P95 > P50 for the current node, got p50=%s p95=%s",
			rec.CurrentNode.LatencyP50, rec.CurrentNode.LatencyP95)
	}

	rate := assertReasonCode(t, rec, ReasonCurrentSuccessRate)
	if !strings.Contains(rate.Message, "成功率") {
		t.Fatalf("expected success-rate reason, got %q", rate.Message)
	}
	p95 := assertReasonCode(t, rec, ReasonRecommendedP95)
	if p95.Evidence["delta"] == nil {
		t.Fatalf("P95 reason must carry the computed delta, got %+v", p95.Evidence)
	}
	candRate := assertReasonCode(t, rec, ReasonRecommendedSuccessRate)
	if candRate.NodeName != "SG-01" {
		t.Fatalf("expected the candidate success-rate reason to reference SG-01, got %q", candRate.NodeName)
	}
}

// -----------------------------------------------------------------------------
// 9. stale candidate → rejected (and does not become the recommendation)
// -----------------------------------------------------------------------------

func TestRecommendFromEvidence_StaleCandidateIsRejected(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend

	hk := testNode("hk", "HK-01", "rev_a")
	old := testNode("old", "OLD-01", "rev_a")

	samples := map[string][]EvidenceSample{
		hk.NodeKey: transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 100, "", ""),
		// OLD-01 is far faster but its newest sample is 15 minutes old.
		old.NodeKey: transportSeries(old, now, 15*time.Minute, 30*time.Second, 5, true, 10, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, old}, samples)

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})

	assertDecision(t, rec, DecisionStay)
	assertNonExecuting(t, rec)
	rejection := assertRejected(t, rec, "OLD-01", RejectStaleEvidence)
	if !strings.Contains(rejection.Reason, "MaxSampleAge") {
		t.Fatalf("stale rejection must cite the applied threshold, got %q", rejection.Reason)
	}
	if rec.RecommendedNode != nil {
		t.Fatalf("a stale candidate must never be recommended, got %+v", rec.RecommendedNode)
	}
}

// -----------------------------------------------------------------------------
// 10. ConfigRevisionKey change must not mix old-revision evidence
// -----------------------------------------------------------------------------

func TestRecommendFromEvidence_ConfigRevisionChangeDoesNotMixOldEvidence(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend

	hk := testNode("hk", "HK-01", "rev_new")
	sg := testNode("sg", "SG-01", "rev_new")

	// 20 samples from the OLD revision are extremely fast (20ms); if they were mixed in
	// the current node would look far better than it really is.
	oldRevision := revisionSeries(hk, "rev_old", transportSeries(hk, now, 10*time.Second, 10*time.Second, 20, true, 20, "", ""))
	newRevision := revisionSeries(hk, "rev_new", transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 100, "", ""))

	samples := map[string][]EvidenceSample{
		hk.NodeKey: append(append([]EvidenceSample{}, oldRevision...), newRevision...),
		sg.NodeKey: transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 40, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, sg}, samples)

	cur := snap.CurrentNode()
	if cur == nil {
		t.Fatalf("expected current node evidence")
	}
	if cur.ExcludedOtherRevisionSamples != 20 {
		t.Fatalf("expected 20 old-revision samples to be excluded, got %d", cur.ExcludedOtherRevisionSamples)
	}
	if cur.SampleCount != 5 {
		t.Fatalf("expected only the 5 current-revision samples to be counted, got %d", cur.SampleCount)
	}
	if cur.LatencyP50 != 100*time.Millisecond {
		t.Fatalf("old-revision latency leaked into the current revision evidence: P50=%s", cur.LatencyP50)
	}
	if cur.ConfigRevisionKey != "rev_new" {
		t.Fatalf("expected rev_new to be recorded, got %s", cur.ConfigRevisionKey)
	}

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})
	assertNonExecuting(t, rec)
	assertDecision(t, rec, DecisionRecommendSwitch)
	if rec.CurrentNode.LatencyP50 != 100*time.Millisecond {
		t.Fatalf("recommendation must be based on current-revision evidence only, got %s", rec.CurrentNode.LatencyP50)
	}
}

func TestRecommendFromEvidence_OnlyOtherRevisionEvidenceIsInsufficient(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend

	hk := testNode("hk", "HK-01", "rev_new")
	sg := testNode("sg", "SG-01", "rev_new")

	// The current node only has evidence recorded under a previous config revision.
	hkOld := revisionSeries(hk, "rev_old", transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 20, "", ""))

	samples := map[string][]EvidenceSample{
		hk.NodeKey: hkOld,
		sg.NodeKey: transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 40, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, sg}, samples)

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})

	assertDecision(t, rec, DecisionInsufficientEvidence)
	assertNonExecuting(t, rec)
	if rec.RecommendedNode != nil {
		t.Fatalf("evidence from another config revision must never produce a recommendation")
	}
	found := false
	for _, code := range rec.EvidenceSufficiency.Reasons {
		if code == GateReasonOtherRevisionSamplesExcluded {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected %s in sufficiency reasons, got %+v", GateReasonOtherRevisionSamplesExcluded, rec.EvidenceSufficiency)
	}
}

// -----------------------------------------------------------------------------
// Additional guards: unresolved current node, locked node, whitelist
// -----------------------------------------------------------------------------

func TestRecommendFromEvidence_UnresolvedCurrentNodeIsInsufficient(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend

	hk := testNode("hk", "HK-01", "rev_a")
	sg := testNode("sg", "SG-01", "rev_a")
	samples := map[string][]EvidenceSample{
		hk.NodeKey: transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 100, "", ""),
		sg.NodeKey: transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 40, "", ""),
	}

	snap := BuildEvidenceSnapshot(EvidenceInput{
		Now:           now,
		Purpose:       PurposeGeneral,
		Policy:        p,
		Source:        EvidenceSource{JobID: "job-1", LookbackSince: now.Add(-time.Hour), LookbackUntil: now},
		Nodes:         []EvidenceNode{hk, sg},
		SamplesByNode: samples,
	})

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})
	assertDecision(t, rec, DecisionInsufficientEvidence)
	assertNonExecuting(t, rec)
	assertReasonCode(t, rec, ReasonCurrentNodeUnresolved)
	if rec.CurrentNode != nil {
		t.Fatalf("expected no current node ref, got %+v", rec.CurrentNode)
	}
}

func TestRecommendFromEvidence_LockedNodePinsSelection(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend
	p.LockedNode = "HK-01"

	hk := testNode("hk", "HK-01", "rev_a")
	sg := testNode("sg", "SG-01", "rev_a")
	samples := map[string][]EvidenceSample{
		hk.NodeKey: transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 200, "", ""),
		sg.NodeKey: transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 20, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, sg}, samples)

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})

	assertDecision(t, rec, DecisionStay)
	assertNonExecuting(t, rec)
	assertReasonCode(t, rec, ReasonLockedNodePins)
	if rec.RecommendedNode != nil {
		t.Fatalf("a locked node must suspend recommendations, got %+v", rec.RecommendedNode)
	}
}

func TestRecommendFromEvidence_WhitelistFiltersCandidates(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend
	p.CandidateNodes = []string{"SG-01"} // SG-01 is slow, SG-FAST is excluded by whitelist

	hk := testNode("hk", "HK-01", "rev_a")
	sg := testNode("sg", "SG-01", "rev_a")
	fast := testNode("fast", "SG-FAST", "rev_a")

	samples := map[string][]EvidenceSample{
		hk.NodeKey:   transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 100, "", ""),
		sg.NodeKey:   transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 95, "", ""),
		fast.NodeKey: transportSeries(fast, now, 10*time.Second, 30*time.Second, 5, true, 10, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, sg, fast}, samples)

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})

	assertDecision(t, rec, DecisionStay)
	assertNonExecuting(t, rec)
	assertRejected(t, rec, "SG-FAST", RejectNotInWhitelist)
	if rec.RecommendedNode != nil {
		t.Fatalf("whitelisted-away node must never be recommended, got %+v", rec.RecommendedNode)
	}
}

// -----------------------------------------------------------------------------
// Unit tests for the pure classification helpers
// -----------------------------------------------------------------------------

func TestClassifyProbeScope(t *testing.T) {
	cases := []struct {
		probeType string
		want      ProbeScope
	}{
		{"rtt", ScopeTransport},
		{"ttfb", ScopeTransport},
		{"ttfb_heavy", ScopeTransport},
		{"init", ScopeTransport},
		{"exit_ip", ScopeTransport},
		{"", ScopeTransport},
		{"service_google", ScopeService},
		{"service_github", ScopeService},
		{"ai_check", ScopeAI},
		{"antigravity", ScopeAI},
	}
	for _, tc := range cases {
		if got := ClassifyProbeScope(tc.probeType); got != tc.want {
			t.Fatalf("ClassifyProbeScope(%q) = %s, want %s", tc.probeType, got, tc.want)
		}
	}
}

func TestClassifyFailure_PurposeSemantics(t *testing.T) {
	cases := []struct {
		name       string
		probeType  string
		errClass   string
		errDetail  string
		regionFlag bool
		want       FailureKind
	}{
		{"transport timeout", "rtt", "timeout", "i/o timeout", false, FailureTransport},
		{"transport dns", "rtt", "dns_error", "no such host", false, FailureTransport},
		{"service blocked", "service_google", "blocked", "", false, FailureService},
		{"service http 400", "service_google", "http_status_error", "HTTP status 400 FAILED_PRECONDITION", false, FailureService},
		{"service 451 with region flag", "service_github", "http_status_error", "HTTP status 451", true, FailureService},
		{"service plain timeout stays transport", "service_google", "timeout", "i/o timeout", false, FailureTransport},
		{"ai blocked", "ai_check", "blocked", "", false, FailureAIBlock},
		{"ai http 400", "ai_check", "http_status_error", "HTTP status 400", false, FailureAIBlock},
		{"ai plain timeout stays transport", "ai_check", "timeout", "i/o timeout", false, FailureTransport},
	}
	for _, tc := range cases {
		got := ClassifyFailure(tc.probeType, tc.errClass, tc.errDetail, tc.regionFlag)
		if got != tc.want {
			t.Fatalf("%s: ClassifyFailure = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestPercentileOf_NearestRank(t *testing.T) {
	values := []time.Duration{
		10 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond,
		40 * time.Millisecond, 100 * time.Millisecond,
	}
	if got := percentileOf(values, 0.50); got != 30*time.Millisecond {
		t.Fatalf("P50 = %s, want 30ms", got)
	}
	if got := percentileOf(values, 0.95); got != 100*time.Millisecond {
		t.Fatalf("P95 = %s, want 100ms", got)
	}
	if got := percentileOf(nil, 0.5); got != 0 {
		t.Fatalf("P50 of empty set = %s, want 0", got)
	}
}

func TestConsecutiveFailures_RespectsProbeScope(t *testing.T) {
	now := time.Now()
	n := testNode("n", "N-01", "rev_a")

	mkProbe := func(ts time.Time, probeType string, success bool, latencyMs int, errClass string) EvidenceSample {
		return EvidenceSample{
			NodeKey: n.NodeKey, NodeIdentityKey: n.NodeIdentityKey, ConfigRevisionKey: n.ConfigRevisionKey,
			DisplayName: n.DisplayName, ProbeType: probeType, Timestamp: ts,
			Success: success, Latency: time.Duration(latencyMs) * time.Millisecond,
			ErrorClass: errClass,
		}
	}

	// consecutiveFailures expects newest-first input (the evidence builder sorts before calling it).
	samples := []EvidenceSample{
		mkProbe(now, "rtt", false, 0, "timeout"),
		mkProbe(now.Add(-1*time.Minute), "rtt", false, 0, "timeout"),
		mkProbe(now.Add(-2*time.Minute), "rtt", true, 50, ""),
		mkProbe(now.Add(-3*time.Minute), "service_google", false, 0, "blocked"),
	}

	if got := consecutiveFailures(samples, ScopeTransport, false); got != 2 {
		t.Fatalf("expected 2 consecutive transport failures, got %d", got)
	}
	if got := consecutiveFailures(samples, ScopeService, false); got != 1 {
		t.Fatalf("expected 1 consecutive service failure, got %d", got)
	}
	if got := consecutiveFailures(samples, ScopeService, true); got != 1 {
		t.Fatalf("expected 1 blocking service failure, got %d", got)
	}
	if got := consecutiveFailures(samples, ScopeAI, false); got != 0 {
		t.Fatalf("expected 0 consecutive AI failures, got %d", got)
	}
}
