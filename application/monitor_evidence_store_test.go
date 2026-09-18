package application

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/policy"
)

// =============================================================================
// B-01 — Evidence pagination completeness.
//
// The observation window must be read COMPLETELY by draining the keyset cursor, never from a
// single page. A single cursor page is capped at 1000 rows by the store.
// =============================================================================

// failingSampleStore is a minimal monitor.SampleStore whose sample query always fails.
// The embedded interface supplies the unused methods.
type failingSampleStore struct {
	monitor.SampleStore
	err error
}

func (f *failingSampleStore) QueryMonitorSamplesCursor(ctx context.Context, filter monitor.CursorFilter) (*monitor.SampleCursorPage, error) {
	return nil, f.err
}

// pagingSampleStore is a deterministic fake cursor store: it hands out a fixed list of pages
// and records every filter it was called with, so the drain can be asserted precisely.
type pagingSampleStore struct {
	monitor.SampleStore
	pages      [][]*monitor.MonitorSample
	filters    []monitor.CursorFilter
	failOnPage int // 1-based page index that returns an error; 0 = never
}

func (s *pagingSampleStore) QueryMonitorSamplesCursor(ctx context.Context, filter monitor.CursorFilter) (*monitor.SampleCursorPage, error) {
	s.filters = append(s.filters, filter)
	pageIndex := len(s.filters) // 1-based
	if s.failOnPage == pageIndex {
		return nil, errors.New("disk I/O error")
	}
	if pageIndex > len(s.pages) {
		return &monitor.SampleCursorPage{Items: nil, HasMore: false, Limit: filter.Limit}, nil
	}
	items := s.pages[pageIndex-1]
	hasMore := pageIndex < len(s.pages)
	next := ""
	if hasMore {
		next = fmt.Sprintf("cursor-page-%d", pageIndex)
	}
	return &monitor.SampleCursorPage{
		Items:      items,
		NextCursor: next,
		HasMore:    hasMore,
		Limit:      filter.Limit,
	}, nil
}

func fakePageNode() policy.EvidenceNode {
	return policy.EvidenceNode{
		ProfileID:         "prof-A",
		NodeKey:           "nk_paged",
		NodeIdentityKey:   "nid_paged",
		ConfigRevisionKey: "rev_paged",
		DisplayName:       "PAGED",
	}
}

func fakeSamples(count int, startIndex int, node policy.EvidenceNode) []*monitor.MonitorSample {
	out := make([]*monitor.MonitorSample, 0, count)
	for i := 0; i < count; i++ {
		idx := startIndex + i
		out = append(out, &monitor.MonitorSample{
			SampleID:            fmt.Sprintf("s_paged_%06d", idx),
			NodeKey:             node.NodeKey,
			NodeIdentityKey:     node.NodeIdentityKey,
			ConfigRevisionKey:   node.ConfigRevisionKey,
			ProfileID:           node.ProfileID,
			DisplayNameSnapshot: node.DisplayName,
			ProbeType:           "rtt",
			Success:             true,
			Latency:             100 * time.Millisecond,
			Timestamp:           time.Now().Add(-time.Duration(idx) * time.Second),
		})
	}
	return out
}

// TestCollectEvidenceSamples_DrainsEveryCursorPage proves the drain follows next_cursor until
// the window is exhausted instead of stopping at the first page.
func TestCollectEvidenceSamples_DrainsEveryCursorPage(t *testing.T) {
	node := fakePageNode()
	store := &pagingSampleStore{
		pages: [][]*monitor.MonitorSample{
			fakeSamples(1000, 0, node),
			fakeSamples(1000, 1000, node),
			fakeSamples(500, 2000, node),
		},
	}

	sets, err := collectEvidenceSamples(context.Background(), store, []policy.EvidenceNode{node}, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("collectEvidenceSamples: %v", err)
	}

	key := node.SampleSetKey()
	if got := len(sets.samples[key]); got != 2500 {
		t.Fatalf("expected all 2500 samples across pages, got %d", got)
	}
	if got := sets.pagesRead[key]; got != 3 {
		t.Fatalf("expected 3 pages drained, got %d", got)
	}
	if sets.budgetExceeded[key] {
		t.Fatalf("2500 samples are well inside the budget, must not be flagged")
	}
	if len(store.filters) != 3 {
		t.Fatalf("expected exactly 3 page queries, got %d", len(store.filters))
	}
}

// TestCollectEvidenceSamples_FilterIsIdenticalOnEveryPage pins that the qualification cannot
// drift mid-drain: only the cursor token may differ between pages.
func TestCollectEvidenceSamples_FilterIsIdenticalOnEveryPage(t *testing.T) {
	node := fakePageNode()
	store := &pagingSampleStore{
		pages: [][]*monitor.MonitorSample{
			fakeSamples(1000, 0, node),
			fakeSamples(1000, 1000, node),
			fakeSamples(10, 2000, node),
		},
	}

	if _, err := collectEvidenceSamples(context.Background(), store, []policy.EvidenceNode{node}, time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("collectEvidenceSamples: %v", err)
	}
	if len(store.filters) != 3 {
		t.Fatalf("expected 3 page queries, got %d", len(store.filters))
	}

	first := store.filters[0]
	for i, filter := range store.filters {
		if filter.NodeIdentityKey != first.NodeIdentityKey {
			t.Fatalf("page %d drifted NodeIdentityKey: %q vs %q", i+1, filter.NodeIdentityKey, first.NodeIdentityKey)
		}
		if filter.NodeKey != first.NodeKey {
			t.Fatalf("page %d drifted NodeKey", i+1)
		}
		if filter.ProfileID != first.ProfileID {
			t.Fatalf("page %d drifted ProfileID: %q vs %q", i+1, filter.ProfileID, first.ProfileID)
		}
		if filter.Limit != first.Limit || filter.Limit != evidenceCursorPageSize {
			t.Fatalf("page %d drifted Limit: %d", i+1, filter.Limit)
		}
		if filter.OrderDesc != first.OrderDesc {
			t.Fatalf("page %d drifted OrderDesc", i+1)
		}
		if filter.Since == nil || first.Since == nil || !filter.Since.Equal(*first.Since) {
			t.Fatalf("page %d drifted Since", i+1)
		}
		if i == 0 && filter.Cursor != "" {
			t.Fatalf("the first page must not carry a cursor")
		}
		if i > 0 && filter.Cursor == "" {
			t.Fatalf("page %d must carry the previous page cursor", i+1)
		}
	}
}

// TestCollectEvidenceSamples_AnyPageErrorFailsTheWholeRead proves a partial read can never be
// returned as if it were complete evidence.
func TestCollectEvidenceSamples_AnyPageErrorFailsTheWholeRead(t *testing.T) {
	node := fakePageNode()
	store := &pagingSampleStore{
		pages: [][]*monitor.MonitorSample{
			fakeSamples(1000, 0, node),
			fakeSamples(1000, 1000, node),
			fakeSamples(10, 2000, node),
		},
		failOnPage: 2,
	}

	sets, err := collectEvidenceSamples(context.Background(), store, []policy.EvidenceNode{node}, time.Now().Add(-time.Hour))
	if err == nil {
		t.Fatalf("a failing page must fail the whole read, got sets=%+v", sets)
	}
	if sets != nil {
		t.Fatalf("no partial evidence may be returned on error")
	}
}

// TestCollectEvidenceSamples_BudgetExceededIsReportedNotSilentlyTruncated pins the explicit
// budget: when the window cannot be drained completely, the node is flagged so the policy
// layer can gate it, instead of the partial read being treated as the whole window.
func TestCollectEvidenceSamples_BudgetExceededIsReportedNotSilentlyTruncated(t *testing.T) {
	node := fakePageNode()
	pages := make([][]*monitor.MonitorSample, 0, 55)
	for i := 0; i < 55; i++ {
		pages = append(pages, fakeSamples(1000, i*1000, node))
	}
	store := &pagingSampleStore{pages: pages}

	sets, err := collectEvidenceSamples(context.Background(), store, []policy.EvidenceNode{node}, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("collectEvidenceSamples: %v", err)
	}
	key := node.SampleSetKey()
	if !sets.budgetExceeded[key] {
		t.Fatalf("expected the node to be flagged as budget-exceeded")
	}
	if got := len(sets.samples[key]); got < maxEvidenceSamplesPerNode {
		t.Fatalf("expected at least %d samples read before the budget stopped the drain, got %d",
			maxEvidenceSamplesPerNode, got)
	}
}

// TestBuildEvidenceSnapshot_BudgetExceededGatesTheNode proves the budget flag actually reaches
// the evidence gate: an incomplete read must never drive a recommendation.
func TestBuildEvidenceSnapshot_BudgetExceededGatesTheNode(t *testing.T) {
	now := time.Now()
	p := policy.DefaultSwitchPolicy()
	node := policy.EvidenceNode{
		ProfileID:         "prof-A",
		NodeKey:           "nk_budget",
		NodeIdentityKey:   "nid_budget",
		ConfigRevisionKey: "rev_budget",
		DisplayName:       "BUDGET",
	}

	snap := policy.BuildEvidenceSnapshot(policy.EvidenceInput{
		Now:                 now,
		Purpose:             policy.PurposeGeneral,
		Policy:              p,
		Source:              policy.EvidenceSource{JobID: "job-1", NodeSetSource: "monitor_job"},
		CurrentNodeKey:      node.NodeKey,
		Nodes:               []policy.EvidenceNode{node},
		SamplesByNode:       map[string][]policy.EvidenceSample{node.SampleSetKey(): {}},
		BudgetExceededNodes: map[string]bool{node.SampleSetKey(): true},
		PagesReadByNode:     map[string]int{node.SampleSetKey(): maxEvidencePagesPerNode},
	})

	cur := snap.CurrentNode()
	if cur == nil {
		t.Fatalf("expected current node evidence")
	}
	if !cur.EvidenceBudgetExceeded {
		t.Fatalf("expected the budget flag to be recorded on the node")
	}
	if cur.Sufficiency.Sufficient {
		t.Fatalf("an incomplete read must not be sufficient evidence")
	}
	found := false
	for _, code := range cur.Sufficiency.Reasons {
		if code == policy.GateReasonEvidenceBudgetExceeded {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected %s, got %+v", policy.GateReasonEvidenceBudgetExceeded, cur.Sufficiency)
	}
	if !snap.Truncated {
		t.Fatalf("expected the snapshot to report truncation")
	}
}

// TestCollectEvidenceSamples_FailingStoreSurfacesAsError keeps the earlier guard: a failing
// query must never be downgraded to "this node has no evidence".
func TestCollectEvidenceSamples_FailingStoreSurfacesAsError(t *testing.T) {
	store := &failingSampleStore{err: errors.New("disk I/O error")}
	nodes := []policy.EvidenceNode{
		{
			ProfileID:         "prof-A",
			NodeKey:           "nk_a",
			NodeIdentityKey:   "nid_a",
			ConfigRevisionKey: "rev_a",
			DisplayName:       "A",
		},
	}

	_, err := collectEvidenceSamples(context.Background(), store, nodes, time.Now().Add(-time.Hour))
	if err == nil {
		t.Fatalf("a failing evidence query must surface as an error")
	}
	if !errors.Is(err, store.err) {
		t.Fatalf("the underlying store error must be preserved, got %v", err)
	}
}
