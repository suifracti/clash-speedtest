package history

import (
	"context"
	"strconv"
	"testing"
	"time"
)

func TestNewServiceResultRecordsWriterWaitAndRemainsIdempotent(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	db := store.DB()
	now := time.Now()
	a := publicServiceAttemptFixture("timed", "timed-q", "p", "n", "i", "r", "google_204", now)
	if err = store.CreatePublicServiceAttempt(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err = store.BeginPublicServiceAttempt(ctx, a.AttemptID, now); err != nil {
		t.Fatal(err)
	}
	m := PublicServiceMeasurement{Outcome: "matched", StartedAt: now, FinishedAt: now, Details: map[string]string{"timing_version": "1"}}
	db.mu.Lock()
	done := make(chan error, 1)
	go func() { done <- store.StagePublicServiceResult(ctx, a.AttemptID, "completed", m) }()
	time.Sleep(35 * time.Millisecond)
	db.mu.Unlock()
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if err = store.CommitPublicServiceResult(ctx, a.AttemptID); err != nil {
		t.Fatal(err)
	}
	if err = store.CommitPublicServiceResult(ctx, a.AttemptID); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetPublicServiceAttemptByRequestID(ctx, a.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	waited, _ := strconv.ParseInt(got.Result.Details["stage_save_lock_wait_ns"], 10, 64)
	if waited < 20e6 {
		t.Fatalf("writer wait not captured: %+v", got.Result.Details)
	}
	if _, ok := got.Result.Details["commit_save_lock_wait_ns"]; !ok {
		t.Fatal("commit writer wait not recorded")
	}
}
