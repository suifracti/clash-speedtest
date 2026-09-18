package policy

import (
	"testing"
	"time"
)

// =============================================================================
// Second-order audit findings (self-audit performed after the review fixes).
// =============================================================================

// TestBuildEvidenceSnapshot_SameNodeKeyAcrossProfilesDoesNotCollide is the audit
// regression for the per-node sample-set key.
//
// The same subscription node can legitimately appear in two profiles with identical
// credentials, in which case ProfileID, NodeKey, NodeIdentityKey and ConfigRevisionKey are
// all equal and ProfileID is the ONLY discriminator. Keying the sample sets by NodeKey
// would make the two logical nodes collide and one would silently read the other's data.
func TestBuildEvidenceSnapshot_SameNodeKeyAcrossProfilesDoesNotCollide(t *testing.T) {
	now := time.Now()
	p := DefaultSwitchPolicy()

	base := EvidenceNode{
		NodeKey:           "nk_shared_identical",
		NodeIdentityKey:   "nid_shared_identical",
		ConfigRevisionKey: "rev_identical",
	}
	profileA := base
	profileA.ProfileID = "prof-A"
	profileA.DisplayName = "SHARED-A"
	profileB := base
	profileB.ProfileID = "prof-B"
	profileB.DisplayName = "SHARED-B"

	if profileA.SampleSetKey() == profileB.SampleSetKey() {
		t.Fatalf("the sample-set key must disambiguate two profiles sharing one NodeKey")
	}

	samples := map[string][]EvidenceSample{
		profileA.SampleSetKey(): transportSeries(profileA, now, 10*time.Second, 30*time.Second, 5, true, 300, "", ""),
		profileB.SampleSetKey(): transportSeries(profileB, now, 10*time.Second, 30*time.Second, 5, true, 10, "", ""),
	}

	snap := BuildEvidenceSnapshot(EvidenceInput{
		Now:           now,
		Purpose:       PurposeGeneral,
		Policy:        p,
		Source:        EvidenceSource{JobID: "job-1", NodeSetSource: "persisted_samples"},
		Nodes:         []EvidenceNode{profileA, profileB},
		SamplesByNode: samples,
	})

	if len(snap.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(snap.Nodes))
	}
	byProfile := map[string]NodeEvidence{}
	for _, node := range snap.Nodes {
		byProfile[node.ProfileID] = node
	}
	if got := byProfile["prof-A"].LatencyP50; got != 300*time.Millisecond {
		t.Fatalf("profile A must see its own evidence: P50=%s, want 300ms", got)
	}
	if got := byProfile["prof-B"].LatencyP50; got != 10*time.Millisecond {
		t.Fatalf("profile B must see its own evidence: P50=%s, want 10ms", got)
	}
	if byProfile["prof-A"].SampleCount != 5 || byProfile["prof-B"].SampleCount != 5 {
		t.Fatalf("expected 5 samples each, got A=%d B=%d",
			byProfile["prof-A"].SampleCount, byProfile["prof-B"].SampleCount)
	}
}

// TestBuildEvidenceSnapshot_OnlyFirstMatchIsMarkedCurrent pins the deterministic behaviour
// when two logical nodes share a NodeKey: exactly one node is reported as current.
func TestBuildEvidenceSnapshot_OnlyFirstMatchIsMarkedCurrent(t *testing.T) {
	now := time.Now()
	p := DefaultSwitchPolicy()

	first := EvidenceNode{
		ProfileID:         "prof-A",
		NodeKey:           "nk_dup",
		NodeIdentityKey:   "nid_dup",
		ConfigRevisionKey: "rev_dup",
		DisplayName:       "DUP-A",
	}
	second := first
	second.ProfileID = "prof-B"
	second.DisplayName = "DUP-B"

	samples := map[string][]EvidenceSample{
		first.SampleSetKey():  transportSeries(first, now, 10*time.Second, 30*time.Second, 5, true, 100, "", ""),
		second.SampleSetKey(): transportSeries(second, now, 10*time.Second, 30*time.Second, 5, true, 50, "", ""),
	}

	snap := BuildEvidenceSnapshot(EvidenceInput{
		Now:            now,
		Purpose:        PurposeGeneral,
		Policy:         p,
		Source:         EvidenceSource{JobID: "job-1"},
		CurrentNodeKey: first.NodeKey,
		Nodes:          []EvidenceNode{first, second},
		SamplesByNode:  samples,
	})

	currentCount := 0
	for i := range snap.Nodes {
		if snap.Nodes[i].IsCurrent {
			currentCount++
		}
	}
	if currentCount != 1 {
		t.Fatalf("expected exactly one node to be marked current, got %d", currentCount)
	}
	if cur := snap.CurrentNode(); cur == nil || cur.ProfileID != "prof-A" {
		t.Fatalf("expected the first match (prof-A) to be current, got %+v", cur)
	}
}

// TestRecommendFromEvidence_EmptyModeDefaultsToMonitorOnly guards the secure default: an
// unset mode must not silently fall through to "evaluate".
func TestRecommendFromEvidence_EmptyModeDefaultsToMonitorOnly(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = "" // unset

	hk := testNode("hk", "HK-01", "rev_a")
	sg := testNode("sg", "SG-01", "rev_a")
	samples := map[string][]EvidenceSample{
		hk.SampleSetKey(): transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 200, "", ""),
		sg.SampleSetKey(): transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 20, "", ""),
	}
	snap := buildSnap(now, p, PurposeGeneral, hk, []EvidenceNode{hk, sg}, samples)

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})
	assertNonExecuting(t, rec)
	if rec.ConfiguredMode != ModeMonitorOnly {
		t.Fatalf("an unset mode must be treated as the secure default monitor_only, got %q", rec.ConfiguredMode)
	}
	if !rec.RecommendationSuppressed || rec.RecommendedNode != nil {
		t.Fatalf("an unset mode must not produce a recommendation, got %+v", rec)
	}
}

// TestRecommendFromEvidence_ProfileIsolationUnknownIsRecorded covers the residual case
// where no sample carries any profile provenance: the evidence is still usable, but the
// isolation gap is recorded explicitly instead of being hidden.
func TestRecommendFromEvidence_ProfileIsolationUnknownIsRecorded(t *testing.T) {
	now := time.Now()
	engine := NewDecisionEngine()
	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend

	hk := EvidenceNode{
		NodeKey:           "nk_noprofile",
		NodeIdentityKey:   "nid_noprofile",
		ConfigRevisionKey: "rev_a",
		DisplayName:       "NO-PROFILE",
	}
	sg := testNode("sg", "SG-01", "rev_a")

	samples := map[string][]EvidenceSample{
		hk.SampleSetKey(): profileSeries("", transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 200, "", "")),
		sg.SampleSetKey(): transportSeries(sg, now, 10*time.Second, 30*time.Second, 5, true, 20, "", ""),
	}

	snap := BuildEvidenceSnapshot(EvidenceInput{
		Now:            now,
		Purpose:        PurposeGeneral,
		Policy:         p,
		Source:         EvidenceSource{JobID: "job-1", NodeSetSource: "persisted_samples"},
		CurrentNodeKey: hk.NodeKey,
		Nodes:          []EvidenceNode{hk, sg},
		SamplesByNode:  samples,
	})

	cur := snap.CurrentNode()
	if cur == nil {
		t.Fatalf("expected current node evidence")
	}
	if !cur.ProfileIsolationUnknown {
		t.Fatalf("expected the missing profile provenance to be recorded")
	}
	if cur.SampleCount != 5 {
		t.Fatalf("evidence without profile provenance must still be usable, got %d samples", cur.SampleCount)
	}

	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "NO-PROFILE"}, snap, RecommendOptions{})
	assertNonExecuting(t, rec)
	reason := assertReasonCode(t, rec, ReasonProfileIsolation)
	if reason.Evidence["profile_isolation_unknown"] != true {
		t.Fatalf("expected the isolation gap in the reason evidence, got %+v", reason.Evidence)
	}
}
