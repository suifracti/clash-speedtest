package application

import (
	"context"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/policy"
)

// =============================================================================
// Legacy key bridge (external PR#7 review, NIT).
//
// core/history/db.go only applies the PR#3 → PR#4 bridge predicate
//
//	((node_identity_key = ?) OR (node_identity_key = node_key AND node_key = ?))
//
// when BOTH NodeIdentityKey and LegacyNodeKey are supplied. Passing only the identity key
// silently under-reads history that was written before the migration, which contradicts the
// documented convention used everywhere else in the codebase.
// =============================================================================

func TestCollectEvidenceSamples_PassesBothLegacyBridgeKeys(t *testing.T) {
	node := fakePageNode()
	store := &pagingSampleStore{
		pages: [][]*monitor.MonitorSample{fakeSamples(2, 0, node)},
	}

	if _, err := collectEvidenceSamples(context.Background(), store, []policy.EvidenceNode{node}, time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("collectEvidenceSamples: %v", err)
	}
	if len(store.filters) == 0 {
		t.Fatalf("expected at least one query to be issued")
	}

	filter := store.filters[0]
	if filter.NodeIdentityKey != node.NodeIdentityKey {
		t.Fatalf("expected the transport identity key to qualify the query, got %q", filter.NodeIdentityKey)
	}
	if filter.LegacyNodeKey != node.NodeKey {
		t.Fatalf("the legacy bridge requires LegacyNodeKey to carry the node key "+
			"(core/history/db.go only applies the bridge when both are set), got %q", filter.LegacyNodeKey)
	}
}

// Without a transport identity key there is nothing to bridge: the node key itself must be
// used, and the identity key must stay empty rather than being fabricated.
func TestCollectEvidenceSamples_FallsBackToNodeKeyWithoutIdentity(t *testing.T) {
	node := fakePageNode()
	node.NodeIdentityKey = ""
	store := &pagingSampleStore{
		pages: [][]*monitor.MonitorSample{fakeSamples(2, 0, node)},
	}

	if _, err := collectEvidenceSamples(context.Background(), store, []policy.EvidenceNode{node}, time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("collectEvidenceSamples: %v", err)
	}
	if len(store.filters) == 0 {
		t.Fatalf("expected at least one query to be issued")
	}

	filter := store.filters[0]
	if filter.NodeIdentityKey != "" {
		t.Fatalf("no identity key exists for this node, so none may be sent, got %q", filter.NodeIdentityKey)
	}
	if filter.NodeKey != node.NodeKey {
		t.Fatalf("expected the node key fallback, got %q", filter.NodeKey)
	}
	if filter.LegacyNodeKey != "" {
		t.Fatalf("expected no legacy bridge key without an identity key, got %q", filter.LegacyNodeKey)
	}
}
