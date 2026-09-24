package history

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func publicServiceAttemptFixture(attemptID, requestID, profileID, nodeKey, identityKey, revisionKey, serviceID string, at time.Time) *PublicServiceAttempt {
	return &PublicServiceAttempt{
		AttemptID: attemptID, RequestID: requestID, ProfileID: profileID, NodeKey: nodeKey,
		NodeIdentityKey: identityKey, ConfigRevisionKey: revisionKey, DisplayName: "same-name",
		NodeType: "http", Source: "workbench_public_service", ServiceID: serviceID,
		Rule: PublicServiceRuleSnapshot{
			ServiceID: serviceID, Name: serviceID, RuleVersion: 1, TargetURL: "https://fixed.example/",
			Method: "GET", SuccessCriterion: "HTTP status is 204", RedirectPolicy: "do_not_follow",
			TimeoutSeconds: 10, MaximumBodyBytes: 65536,
		},
		RequestedAt: at.UTC(), ExecutionState: "queued", PersistenceState: "not_started",
	}
}

func TestPublicServiceAttemptsScopeAndCommitOneResult(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	started := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	attempt := publicServiceAttemptFixture("attempt-a", "request-a", "profile-a", "node-a", "identity-a", "rev-a", "cloudflare_204", started)
	if err := store.CreatePublicServiceAttempt(ctx, attempt); err != nil {
		t.Fatal(err)
	}
	if err := store.BeginPublicServiceAttempt(ctx, attempt.AttemptID, started); err != nil {
		t.Fatal(err)
	}
	finished := started.Add(500 * time.Millisecond)
	measurement := PublicServiceMeasurement{Outcome: "matched", HTTPStatus: intPointer(204), BytesRead: 0, StartedAt: started, FinishedAt: finished, DurationMs: 500}
	if err := store.StagePublicServiceResult(ctx, attempt.AttemptID, "completed", measurement); err != nil {
		t.Fatal(err)
	}
	if err := store.CommitPublicServiceResult(ctx, attempt.AttemptID); err != nil {
		t.Fatal(err)
	}
	if err := store.CommitPublicServiceResult(ctx, attempt.AttemptID); err != nil {
		t.Fatalf("idempotent result commit: %v", err)
	}
	distractors := []*PublicServiceAttempt{
		publicServiceAttemptFixture("attempt-other-profile", "request-other-profile", "profile-b", "node-a", "identity-a", "rev-a", "cloudflare_204", started.Add(time.Second)),
		publicServiceAttemptFixture("attempt-other-revision", "request-other-revision", "profile-a", "node-a", "identity-a", "rev-b", "cloudflare_204", started.Add(2*time.Second)),
		publicServiceAttemptFixture("attempt-other-service", "request-other-service", "profile-a", "node-a", "identity-a", "rev-a", "google_204", started.Add(3*time.Second)),
	}
	for _, distractor := range distractors {
		if err := store.CreatePublicServiceAttempt(ctx, distractor); err != nil {
			t.Fatal(err)
		}
	}

	query := PublicServiceFilter{ProfileID: "profile-a", NodeKey: "node-a", NodeIdentityKey: "identity-a", ConfigRevisionKey: "rev-a", ServiceID: "cloudflare_204", Limit: 1}
	page, err := store.QueryPublicServiceAttempts(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Attempts) != 1 || page.HasMore || page.Attempts[0].AttemptID != attempt.AttemptID || page.Attempts[0].PersistenceState != "saved" || page.Attempts[0].Result == nil || page.Attempts[0].Result.Outcome != "matched" {
		t.Fatalf("public-service history = %+v", page)
	}
	if _, err := store.GetPublicServiceAttempt(ctx, "attempt-a", PublicServiceFilter{ProfileID: "profile-b", NodeKey: "node-a", NodeIdentityKey: "identity-a", ConfigRevisionKey: "rev-a", ServiceID: "cloudflare_204"}); err != ErrPublicServiceAttemptNotFound {
		t.Fatalf("cross-profile read err = %v, want not found", err)
	}
	if _, err := store.GetPublicServiceAttemptByRequestID(ctx, "request-a"); err != nil {
		t.Fatalf("request ID lookup: %v", err)
	}
}

func TestPublicServiceReopenMarksInterruptedAndRetriesStagedAttemptBySameID(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "history")
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	at := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	interrupted := publicServiceAttemptFixture("attempt-interrupted", "request-interrupted", "profile-a", "node-a", "identity-a", "rev-a", "google_204", at)
	staged := publicServiceAttemptFixture("attempt-staged", "request-staged", "profile-a", "node-a", "identity-a", "rev-a", "github_api_root", at.Add(time.Second))
	for _, item := range []*PublicServiceAttempt{interrupted, staged} {
		if err := store.CreatePublicServiceAttempt(ctx, item); err != nil {
			t.Fatal(err)
		}
		if err := store.BeginPublicServiceAttempt(ctx, item.AttemptID, at); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.StagePublicServiceResult(ctx, staged.AttemptID, "completed", PublicServiceMeasurement{
		Outcome: "matched", HTTPStatus: intPointer(200), BytesRead: 65, StartedAt: at, FinishedAt: at.Add(2 * time.Second), DurationMs: 2000,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.ReconcilePublicServiceAttempts(ctx); err != nil {
		t.Fatal(err)
	}
	filter := PublicServiceFilter{ProfileID: "profile-a", NodeKey: "node-a", NodeIdentityKey: "identity-a", ConfigRevisionKey: "rev-a", ServiceID: "google_204"}
	gotInterrupted, err := reopened.GetPublicServiceAttempt(ctx, interrupted.AttemptID, filter)
	if err != nil || gotInterrupted.ExecutionState != "interrupted" || gotInterrupted.Result != nil {
		t.Fatalf("interrupted attempt = %+v, err=%v", gotInterrupted, err)
	}
	stagedFilter := filter
	stagedFilter.ServiceID = "github_api_root"
	gotStaged, err := reopened.GetPublicServiceAttempt(ctx, staged.AttemptID, stagedFilter)
	if err != nil || gotStaged.PersistenceState != "failed" || gotStaged.Result == nil || gotStaged.ExecutionState != "completed" {
		t.Fatalf("staged attempt = %+v, err=%v", gotStaged, err)
	}
	if err := reopened.CommitPublicServiceResult(ctx, staged.AttemptID); err != nil {
		t.Fatalf("retry staged save: %v", err)
	}
	gotSaved, err := reopened.GetPublicServiceAttempt(ctx, staged.AttemptID, stagedFilter)
	if err != nil || gotSaved.PersistenceState != "saved" || gotSaved.AttemptID != staged.AttemptID {
		t.Fatalf("saved retry = %+v, err=%v", gotSaved, err)
	}
}

func TestSchemaV5UpgradeAddsPublicServiceTablesWithoutChangingBudget(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.db.Exec(`UPDATE schema_meta SET schema_version=5 WHERE singleton=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.db.Exec(`DROP TABLE workbench_public_service_results; DROP TABLE workbench_public_service_attempts`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.db.Exec(`UPDATE monitor_budget_usage SET utc_day='2026-09-23', requests_used=17, bytes_used=4096 WHERE singleton=1`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	upgraded, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var version, requests int
	var bytes int64
	if err := upgraded.db.db.QueryRow(`SELECT schema_version FROM schema_meta WHERE singleton=1`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := upgraded.db.db.QueryRow(`SELECT requests_used,bytes_used FROM monitor_budget_usage WHERE singleton=1`).Scan(&requests, &bytes); err != nil {
		t.Fatal(err)
	}
	if version != CurrentSchemaVersion || requests != 17 || bytes != 4096 {
		t.Fatalf("schema/budget after upgrade = version %d requests %d bytes %d", version, requests, bytes)
	}
}

func intPointer(value int) *int { return &value }
