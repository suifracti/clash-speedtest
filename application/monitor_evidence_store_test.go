package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/policy"
)

// failingSampleStore is a minimal monitor.SampleStore whose sample query always fails.
// The embedded interface supplies the unused methods.
type failingSampleStore struct {
	monitor.SampleStore
	err error
}

func (f *failingSampleStore) QueryMonitorSamples(ctx context.Context, filter monitor.SampleFilter) ([]*monitor.MonitorSample, error) {
	return nil, f.err
}

// TestCollectEvidenceSamples_QueryFailureIsNotSilentlyTreatedAsNoEvidence guards against a
// silent omission: a failing persistence query must surface as an error. Swallowing it would
// report an infrastructure failure as "this node simply has no evidence", which looks like an
// ordinary insufficient-data verdict and hides the real problem.
func TestCollectEvidenceSamples_QueryFailureIsNotSilentlyTreatedAsNoEvidence(t *testing.T) {
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

	_, _, _, err := collectEvidenceSamples(context.Background(), store, nodes, time.Now().Add(-time.Hour))
	if err == nil {
		t.Fatalf("a failing evidence query must surface as an error")
	}
	if !errors.Is(err, store.err) {
		t.Fatalf("the underlying store error must be preserved, got %v", err)
	}
}
