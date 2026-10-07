package history

import (
	"context"
	"testing"
	"time"
)

func TestLatestServiceChecksKeepRealFailureAndFrozenRevision(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	at := time.Now().UTC()
	for i, id := range []string{"old", "new", "other_revision", "running"} {
		rev := "r"
		if i == 2 {
			rev = "other"
		}
		a := publicServiceAttemptFixture(id, id, "p", "n", "i", rev, "cloudflare_204", at.Add(time.Duration(i)*time.Second))
		if err := store.CreatePublicServiceAttempt(ctx, a); err != nil {
			t.Fatal(err)
		}
		if i == 3 {
			continue
		}
		if err := store.BeginPublicServiceAttempt(ctx, id, a.RequestedAt); err != nil {
			t.Fatal(err)
		}
		result := PublicServiceMeasurement{Outcome: "timed_out", StartedAt: a.RequestedAt, FinishedAt: a.RequestedAt.Add(time.Millisecond), DurationMs: 1}
		if err := store.StagePublicServiceResult(ctx, id, "failed", result); err != nil {
			t.Fatal(err)
		}
		if err := store.CommitPublicServiceResult(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	checks, err := store.DB().LatestPublicServiceChecks(ctx, "p", "i", "r")
	if err != nil || len(checks) != 1 || checks[0].AttemptID != "new" || checks[0].Result.Outcome != "timed_out" {
		t.Fatalf("latest completed failed check lost: %+v %v", checks, err)
	}
}
