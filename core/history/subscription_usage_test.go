package history

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/subscriptionusage"
)

func TestSubscriptionUsageMigrationPersistenceAndSingleDailyClaim(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMonitorSamples(ctx, []*monitor.MonitorSample{{SampleID: "existing", RunID: "existing-run", NodeKey: "n", Timestamp: time.Now(), Success: true}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", filepath.Join(dir, "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`DROP TABLE subscription_usage_snapshots; DROP TABLE subscription_usage_refresh; UPDATE schema_meta SET schema_version=9`); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	s, err = NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	up, down := int64(10), int64(20)
	at := time.Date(2026, 10, 1, 0, 5, 0, 0, subscriptionusage.Location)
	snap := subscriptionusage.Snapshot{AccountKey: "account", CapturedAt: at, Status: "ok", Upload: &up, Download: &down}
	for i := 0; i < 2; i++ {
		if err := s.SaveSubscriptionUsage(ctx, snap); err != nil {
			t.Fatal(err)
		}
	}
	if claimed, err := s.ClaimSubscriptionRefresh(ctx, at, subscriptionusage.NextDaily(at), false); err != nil || !claimed {
		t.Fatalf("first claim: %v %v", claimed, err)
	}
	if claimed, err := s.ClaimSubscriptionRefresh(ctx, at, subscriptionusage.NextDaily(at), false); err != nil || claimed {
		t.Fatalf("duplicate claim: %v %v", claimed, err)
	}
	s.Close()
	s, err = NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if claimed, err := s.ClaimSubscriptionRefresh(ctx, at, subscriptionusage.NextDaily(at), false); err != nil || claimed {
		t.Fatalf("reopen replayed update: %v %v", claimed, err)
	}
	snaps, err := s.QuerySubscriptionUsage(ctx, at.Add(time.Hour), at.AddDate(0, 0, 2))
	if err != nil || len(snaps) != 1 || *snaps[0].Download != 20 {
		t.Fatalf("baseline readback/dedup: %+v %v", snaps, err)
	}
	if samples, err := s.QueryMonitorSamples(ctx, monitor.SampleFilter{Limit: 10}); err != nil || len(samples) != 1 || samples[0].SampleID != "existing" {
		t.Fatalf("migration touched existing history: %+v %v", samples, err)
	}
	if claimed, err := s.ClaimSubscriptionRefresh(ctx, at.AddDate(0, 0, 90), subscriptionusage.NextDaily(at.AddDate(0, 0, 90)), false); err != nil || !claimed {
		t.Fatalf("missed dates should allow one current catch-up: %v %v", claimed, err)
	}
}
