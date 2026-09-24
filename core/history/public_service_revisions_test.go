package history

import (
	"context"
	"testing"
	"time"
)

func TestListNodeHistoryRevisionsIncludesPublicServiceOnlyRevision(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	observedAt := time.Date(2026, 9, 24, 1, 2, 3, 0, time.UTC)
	rule := PublicServiceRuleSnapshot{
		ServiceID: "cloudflare_204", Name: "Cloudflare 204", RuleVersion: 1,
		TargetURL: "https://cp.cloudflare.com/generate_204", Method: "GET",
		SuccessCriterion: "HTTP 204", RedirectPolicy: "do_not_follow", TimeoutSeconds: 10, MaximumBodyBytes: 65536,
	}
	for _, attempt := range []*PublicServiceAttempt{
		{AttemptID: "service-old-revision", RequestID: "request-service-old", ProfileID: "profile-a", NodeKey: "node-old", NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-service-only", DisplayName: "Name", Source: "workbench_public_service", ServiceID: "cloudflare_204", Rule: rule, RequestedAt: observedAt, ExecutionState: "interrupted", PersistenceState: "not_applicable"},
		{AttemptID: "other-profile", RequestID: "request-other-profile", ProfileID: "profile-b", NodeKey: "node-old", NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-other-profile", DisplayName: "Name", Source: "workbench_public_service", ServiceID: "cloudflare_204", Rule: rule, RequestedAt: observedAt.Add(time.Minute), ExecutionState: "interrupted", PersistenceState: "not_applicable"},
		{AttemptID: "other-identity", RequestID: "request-other-identity", ProfileID: "profile-a", NodeKey: "node-other", NodeIdentityKey: "identity-b", ConfigRevisionKey: "revision-other-identity", DisplayName: "Name", Source: "workbench_public_service", ServiceID: "cloudflare_204", Rule: rule, RequestedAt: observedAt.Add(2 * time.Minute), ExecutionState: "interrupted", PersistenceState: "not_applicable"},
	} {
		if err := store.CreatePublicServiceAttempt(ctx, attempt); err != nil {
			t.Fatalf("create service history %s: %v", attempt.AttemptID, err)
		}
	}

	revisions, err := store.ListNodeHistoryRevisions(ctx, "profile-a", "identity-a")
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if len(revisions) != 1 || revisions[0].ConfigRevisionKey != "revision-service-only" || revisions[0].NodeKey != "node-old" {
		t.Fatalf("service-only revision should be listed without other identities: %+v", revisions)
	}
	page, err := store.QueryPublicServiceAttempts(ctx, PublicServiceFilter{
		ProfileID: "profile-a", NodeKey: "node-old", NodeIdentityKey: "identity-a",
		ConfigRevisionKey: revisions[0].ConfigRevisionKey, ServiceID: "cloudflare_204", Limit: 10,
	})
	if err != nil || len(page.Attempts) != 1 || page.Attempts[0].AttemptID != "service-old-revision" {
		t.Fatalf("enumerated revision must query its service attempt: page=%+v err=%v", page, err)
	}
}
