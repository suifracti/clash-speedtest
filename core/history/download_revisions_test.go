package history

import (
	"context"
	"testing"
	"time"
)

func TestListNodeHistoryRevisionsIncludesDownloadOnlyRevision(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	observedAt := time.Date(2026, 9, 24, 4, 5, 6, 0, time.UTC)
	for _, attempt := range []*WorkbenchDownloadAttempt{
		{AttemptID: "download-old-revision", RequestID: "request-download-old", ProfileID: "profile-a", NodeKey: "node-old", NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-download-only", DisplayName: "Old name", Source: WorkbenchDownloadSource, RequestedAt: observedAt, ExecutionState: "interrupted", PersistenceState: "not_applicable", Rule: WorkbenchDownloadRuleSnapshot{RuleVersion: 1, TargetURL: "https://speed.cloudflare.com/__down?bytes=1001", Method: "GET", MaximumBytes: 1000, MaximumDurationNS: int64(10 * time.Second)}},
		{AttemptID: "download-other-profile", RequestID: "request-download-other-profile", ProfileID: "profile-b", NodeKey: "node-old", NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-other-profile", DisplayName: "Old name", Source: WorkbenchDownloadSource, RequestedAt: observedAt.Add(time.Minute), ExecutionState: "interrupted", PersistenceState: "not_applicable", Rule: WorkbenchDownloadRuleSnapshot{RuleVersion: 1, TargetURL: "https://speed.cloudflare.com/__down?bytes=1001", Method: "GET", MaximumBytes: 1000, MaximumDurationNS: int64(10 * time.Second)}},
		{AttemptID: "download-other-identity", RequestID: "request-download-other-identity", ProfileID: "profile-a", NodeKey: "node-other", NodeIdentityKey: "identity-b", ConfigRevisionKey: "revision-other-identity", DisplayName: "Old name", Source: WorkbenchDownloadSource, RequestedAt: observedAt.Add(2 * time.Minute), ExecutionState: "interrupted", PersistenceState: "not_applicable", Rule: WorkbenchDownloadRuleSnapshot{RuleVersion: 1, TargetURL: "https://speed.cloudflare.com/__down?bytes=1001", Method: "GET", MaximumBytes: 1000, MaximumDurationNS: int64(10 * time.Second)}},
	} {
		if err := store.CreateWorkbenchDownloadAttempt(ctx, attempt); err != nil {
			t.Fatalf("create download history %s: %v", attempt.AttemptID, err)
		}
	}

	revisions, err := store.ListNodeHistoryRevisions(ctx, "profile-a", "identity-a")
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if len(revisions) != 1 || revisions[0].ConfigRevisionKey != "revision-download-only" || revisions[0].NodeKey != "node-old" {
		t.Fatalf("download-only revision should be listed without other identities: %+v", revisions)
	}
	page, err := store.QueryWorkbenchDownloadAttempts(ctx, WorkbenchDownloadFilter{
		ProfileID: "profile-a", NodeKey: "node-old", NodeIdentityKey: "identity-a",
		ConfigRevisionKey: revisions[0].ConfigRevisionKey, Limit: 10,
	})
	if err != nil || len(page.Attempts) != 1 || page.Attempts[0].AttemptID != "download-old-revision" {
		t.Fatalf("enumerated revision must query its download attempt: page=%+v err=%v", page, err)
	}
}
