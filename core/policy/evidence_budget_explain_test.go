package policy

import (
	"testing"
	"time"
)

// =============================================================================
// Evidence-read-budget explainability.
//
// A partial read must be reported as a partial read. It previously surfaced as
// code="no_evidence", which states the opposite (there was evidence, it could not all be
// read) and hides the one condition a retry / larger budget would fix.
// =============================================================================

// TestBudgetExceededRejectionHasItsOwnCode pins the distinct rejection code.
func TestBudgetExceededRejectionHasItsOwnCode(t *testing.T) {
	now := time.Now()

	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend

	hk := testNode("hk", "HK-01", "rev_a")
	big := testNode("big", "BIG-01", "rev_a")

	samples := map[string][]EvidenceSample{
		hk.SampleSetKey():  transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 100, "", ""),
		big.SampleSetKey(): transportSeries(big, now, 10*time.Second, 30*time.Second, 5, true, 1, "", ""),
	}

	snap := BuildEvidenceSnapshot(EvidenceInput{
		Now:                    now,
		Purpose:                PurposeGeneral,
		Policy:                 p,
		CurrentNodeKey:         hk.NodeKey,
		CurrentNodeIdentityKey: hk.NodeIdentityKey,
		Nodes:                  []EvidenceNode{hk, big},
		SamplesByNode:          samples,
		RawSampleCount:         countSamples(samples),
		BudgetExceededNodes:    map[string]bool{big.SampleSetKey(): true},
		// Supply the real page count, as the production drain does.
		PagesReadByNode: map[string]int{big.SampleSetKey(): 60},
	})

	var bigEv *NodeEvidence
	for i := range snap.Nodes {
		if snap.Nodes[i].DisplayName == "BIG-01" {
			bigEv = &snap.Nodes[i]
		}
	}
	if bigEv == nil {
		t.Fatal("BIG-01 missing")
	}
	if !bigEv.EvidenceBudgetExceeded {
		t.Fatal("test setup invalid: budget flag not set")
	}
	t.Logf("BIG-01: pages_read=%d sufficient=%v reasons=%v", bigEv.PagesRead, bigEv.Sufficiency.Sufficient, bigEv.Sufficiency.Reasons)

	engine := NewDecisionEngine()
	rec := engine.RecommendFromEvidence(now, p, &DecisionState{CurrentNode: "HK-01"}, snap, RecommendOptions{})

	for _, r := range rec.RejectedCandidates {
		if r.NodeName != "BIG-01" {
			continue
		}
		t.Logf("BIG-01 rejected: code=%q reason=%q", r.Code, r.Reason)
		if r.Code == RejectNoEvidence {
			t.Errorf("a partial read must not be reported as %q", RejectNoEvidence)
		}
		if r.Code != RejectEvidenceBudgetExceeded {
			t.Errorf("expected code %q, got %q", RejectEvidenceBudgetExceeded, r.Code)
		}
		return
	}
	t.Fatalf("BIG-01 was not rejected at all: %+v", rec.RejectedCandidates)
}

// TestBudgetExceededPagesReadIsNonZero: the budget is only ever exceeded after a page read,
// so reporting 0 would read as "nothing was read".
func TestBudgetExceededPagesReadIsNonZero(t *testing.T) {
	now := time.Now()

	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend

	hk := testNode("hk", "HK-01", "rev_a")
	samples := map[string][]EvidenceSample{
		hk.SampleSetKey(): transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 100, "", ""),
	}

	// Deliberately omit PagesReadByNode to simulate a caller that only flags the budget.
	snap := BuildEvidenceSnapshot(EvidenceInput{
		Now:                    now,
		Purpose:                PurposeGeneral,
		Policy:                 p,
		CurrentNodeKey:         hk.NodeKey,
		CurrentNodeIdentityKey: hk.NodeIdentityKey,
		Nodes:                  []EvidenceNode{hk},
		SamplesByNode:          samples,
		RawSampleCount:         countSamples(samples),
		BudgetExceededNodes:    map[string]bool{hk.SampleSetKey(): true},
	})

	ev := &snap.Nodes[0]
	t.Logf("pages_read=%d budget_exceeded=%v", ev.PagesRead, ev.EvidenceBudgetExceeded)
	if ev.EvidenceBudgetExceeded && ev.PagesRead <= 0 {
		t.Errorf("budget exceeded but pages_read=%d: provenance reads as 'nothing was read'", ev.PagesRead)
	}
}

// TestBudgetExceededDetailCarriesPageCount: the human-readable detail must show the real page
// count so an operator can tell how far the read got.
func TestBudgetExceededDetailCarriesPageCount(t *testing.T) {
	now := time.Now()

	p := DefaultSwitchPolicy()
	p.Mode = ModeRecommend

	hk := testNode("hk", "HK-01", "rev_a")
	samples := map[string][]EvidenceSample{
		hk.SampleSetKey(): transportSeries(hk, now, 10*time.Second, 30*time.Second, 5, true, 100, "", ""),
	}

	snap := BuildEvidenceSnapshot(EvidenceInput{
		Now:                    now,
		Purpose:                PurposeGeneral,
		Policy:                 p,
		CurrentNodeKey:         hk.NodeKey,
		CurrentNodeIdentityKey: hk.NodeIdentityKey,
		Nodes:                  []EvidenceNode{hk},
		SamplesByNode:          samples,
		RawSampleCount:         countSamples(samples),
		BudgetExceededNodes:    map[string]bool{hk.SampleSetKey(): true},
		PagesReadByNode:        map[string]int{hk.SampleSetKey(): 42},
	})

	ev := &snap.Nodes[0]
	if ev.PagesRead != 42 {
		t.Fatalf("expected pages_read=42 to be preserved, got %d", ev.PagesRead)
	}
	found := false
	for _, d := range ev.Sufficiency.Details {
		if contains(d, "42") {
			found = true
		}
	}
	if !found {
		t.Errorf("the budget detail must mention the pages read, got %v", ev.Sufficiency.Details)
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		(func() bool {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
			return false
		})()
}
