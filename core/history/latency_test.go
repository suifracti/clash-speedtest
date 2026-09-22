package history

import (
	"context"
	"errors"
	"fmt"
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
	if len(rows.Tests) != 1 {
		t.Fatalf("expected one idempotent history row, got %d", len(rows.Tests))
	}
	if _, err := reopened.QueryLatencyTests(ctx, LatencyTestFilter{ProfileID: "profile_b", NodeKey: "node_a"}); err != nil {
		t.Fatalf("cross-scope empty query should be valid: %v", err)
	} else {
		other, _ := reopened.QueryLatencyTests(ctx, LatencyTestFilter{ProfileID: "profile_b", NodeKey: "node_a"})
		if len(other.Tests) != 0 {
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

func TestQueryLatencyTestsFiltersRawSamplesBeforeLimitAndScopesProfiles(t *testing.T) {
	ctx := context.Background()
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	asOf := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	since := asOf.Add(-4 * time.Hour)
	until := asOf
	nodeKey := "node-shared"

	save := func(profileID, attemptID string, finishedAt time.Time, timestamps ...time.Time) {
		t.Helper()
		samples := make([]LatencyTestSample, 0, len(timestamps))
		for i, timestamp := range timestamps {
			samples = append(samples, LatencyTestSample{
				Seq:       i + 1,
				Timestamp: timestamp,
				LatencyMs: int64(20 + i),
				Success:   true,
			})
		}
		if err := store.SaveLatencyTest(ctx, &LatencyTest{
			AttemptID:      attemptID,
			ProfileID:      profileID,
			NodeKey:        nodeKey,
			TestProject:    "latency_stability",
			RequestedAt:    finishedAt.Add(-time.Second),
			StartedAt:      finishedAt.Add(-500 * time.Millisecond),
			FinishedAt:     finishedAt,
			Status:         "completed",
			TotalSamples:   len(samples),
			SuccessSamples: len(samples),
			Samples:        samples,
		}); err != nil {
			t.Fatalf("SaveLatencyTest %s: %v", attemptID, err)
		}
	}

	// These newer attempts must not consume the bounded page because none of
	// their raw samples belongs to the requested window.
	for i := 0; i < 20; i++ {
		finished := until.Add(time.Duration(i+1) * time.Minute)
		save("profile-a", fmt.Sprintf("outside-%02d", i), finished, until.Add(time.Duration(i+1)*time.Minute))
	}
	for i := 0; i < 21; i++ {
		finished := until.Add(-time.Duration(i+1) * time.Minute)
		save("profile-a", fmt.Sprintf("inside-%02d", i), finished, since.Add(time.Duration(i+1)*time.Minute))
	}
	crossingID := "crossing-attempt"
	save("profile-a", crossingID, until.Add(-90*time.Minute),
		since.Add(-time.Second),
		since,
		until.Add(-time.Second),
		until,
	)
	save("profile-b", "profile-b-same-node", until.Add(-30*time.Minute), since.Add(time.Hour))

	page, err := store.QueryLatencyTests(ctx, LatencyTestFilter{
		ProfileID: "profile-a",
		NodeKey:   nodeKey,
		Since:     &since,
		Until:     &until,
		Limit:     20,
	})
	if err != nil {
		t.Fatalf("window query: %v", err)
	}
	if len(page.Tests) != 20 || !page.HasMore {
		t.Fatalf("expected 20 matching attempts with has_more, got %d/%v", len(page.Tests), page.HasMore)
	}
	for _, test := range page.Tests {
		if test.ProfileID != "profile-a" || test.NodeKey != nodeKey {
			t.Fatalf("scope leaked into bounded page: %+v", test)
		}
		for _, sample := range test.Samples {
			if sample.Timestamp.Before(since) || !sample.Timestamp.Before(until) {
				t.Fatalf("out-of-window raw sample survived query: %+v", sample)
			}
		}
	}

	all, err := store.QueryLatencyTests(ctx, LatencyTestFilter{
		ProfileID: "profile-a",
		NodeKey:   nodeKey,
		Since:     &since,
		Until:     &until,
		Limit:     100,
	})
	if err != nil {
		t.Fatalf("unbounded window query: %v", err)
	}
	if len(all.Tests) != 22 || all.HasMore {
		t.Fatalf("expected all 22 profile A attempts in window, got %d/%v", len(all.Tests), all.HasMore)
	}

	crossing, err := store.GetLatencyTestInWindow(ctx, crossingID, &since, &until)
	if err != nil {
		t.Fatalf("crossing attempt query: %v", err)
	}
	if len(crossing.Samples) != 2 || crossing.Samples[0].Timestamp != since || crossing.Samples[1].Timestamp != until.Add(-time.Second) {
		t.Fatalf("half-open crossing filter mismatch: %+v", crossing.Samples)
	}

	profileB, err := store.QueryLatencyTests(ctx, LatencyTestFilter{
		ProfileID: "profile-b",
		NodeKey:   nodeKey,
		Since:     &since,
		Until:     &until,
	})
	if err != nil {
		t.Fatalf("profile B query: %v", err)
	}
	if len(profileB.Tests) != 1 || profileB.Tests[0].ProfileID != "profile-b" {
		t.Fatalf("same node key crossed profile boundary: %+v", profileB.Tests)
	}
}
