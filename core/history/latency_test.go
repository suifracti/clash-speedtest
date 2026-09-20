package history

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLatencyTestPersistenceReopenAndIdempotency(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	started := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	finished := started.Add(800 * time.Millisecond)
	testRecord := &LatencyTest{
		AttemptID:         "latency_attempt_001",
		ProfileID:         "profile_a",
		NodeKey:           "node_a",
		NodeIdentityKey:   "identity_a",
		ConfigRevisionKey: "revision_a",
		DisplayName:       "同名节点",
		NodeType:          "ss",
		TestProject:       "latency_stability",
		RequestedAt:       started.Add(-100 * time.Millisecond),
		StartedAt:         started,
		FinishedAt:        finished,
		Status:            "partial_failed",
		LatencyMs:         42,
		JitterMs:          8,
		PacketLoss:        0.5,
		TotalSamples:      2,
		SuccessSamples:    1,
		FailureSamples:    1,
		ErrorMessage:      "连接超时",
		Samples: []LatencyTestSample{
			{Seq: 1, Timestamp: started.Add(100 * time.Millisecond), LatencyMs: 42, Success: true},
			{Seq: 2, Timestamp: started.Add(600 * time.Millisecond), Success: false, Error: "连接超时"},
		},
	}

	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.SaveLatencyTest(ctx, testRecord); err != nil {
		t.Fatalf("SaveLatencyTest: %v", err)
	}
	// The same execution may be retried after a transport response is lost.
	if err := store.SaveLatencyTest(ctx, testRecord); err != nil {
		t.Fatalf("idempotent SaveLatencyTest: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := NewStore(dir)
	if err != nil {
		t.Fatalf("reopen NewStore: %v", err)
	}
	defer reopened.Close()

	got, err := reopened.GetLatencyTest(ctx, testRecord.AttemptID)
	if err != nil {
		t.Fatalf("GetLatencyTest after reopen: %v", err)
	}
	if got.ProfileID != testRecord.ProfileID || got.NodeKey != testRecord.NodeKey || got.Status != testRecord.Status {
		t.Fatalf("identity/status not preserved: %+v", got)
	}
	if len(got.Samples) != 2 || got.Samples[1].Success || got.Samples[1].Error != "连接超时" {
		t.Fatalf("raw samples not preserved: %+v", got.Samples)
	}

	rows, err := reopened.QueryLatencyTests(ctx, LatencyTestFilter{ProfileID: "profile_a", NodeKey: "node_a"})
	if err != nil {
		t.Fatalf("QueryLatencyTests: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one idempotent history row, got %d", len(rows))
	}
	if _, err := reopened.QueryLatencyTests(ctx, LatencyTestFilter{ProfileID: "profile_b", NodeKey: "node_a"}); err != nil {
		t.Fatalf("cross-scope empty query should be valid: %v", err)
	} else {
		other, _ := reopened.QueryLatencyTests(ctx, LatencyTestFilter{ProfileID: "profile_b", NodeKey: "node_a"})
		if len(other) != 0 {
			t.Fatalf("same node key leaked across profile scope: %+v", other)
		}
	}
}

func TestLatencyTestAttemptIDConflictIsRejected(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	base := &LatencyTest{
		AttemptID:   "latency_conflict",
		ProfileID:   "profile_a",
		NodeKey:     "node_a",
		TestProject: "latency_stability",
		RequestedAt: time.Now().UTC(),
		StartedAt:   time.Now().UTC(),
		FinishedAt:  time.Now().UTC(),
		Status:      "completed",
	}
	if err := store.SaveLatencyTest(ctx, base); err != nil {
		t.Fatalf("initial save: %v", err)
	}
	conflict := *base
	conflict.NodeKey = "node_b"
	if err := store.SaveLatencyTest(ctx, &conflict); err == nil {
		t.Fatal("expected attempt ID conflict")
	} else if errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected context error: %v", err)
	}
}
