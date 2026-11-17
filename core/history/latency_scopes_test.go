package history

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
)

func TestLatencyHistoryScopesPreserveAccountNameWindowAndCompleteIdentity(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	at := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	since, until := at.Add(-time.Hour), at.Add(time.Hour)
	// These saved tests have no batch rows at all. Scope discovery must use
	// durable evidence, not the global UI's recent-batch list.
	for _, item := range []struct {
		id, profile, name, identity, revision string
		at                                    time.Time
	}{
		{"old", "account-a", "Node", "old-id", "shared-rev", since},
		{"current", "account-a", "Node", "new-id", "shared-rev", at},
		{"repeat", "account-a", "Node", "old-id", "shared-rev", at},
		{"other-account", "account-b", "Node", "other-id", "other-rev", at},
		{"renamed", "account-a", "Other name", "renamed-id", "renamed-rev", at},
		{"outside", "account-a", "Node", "outside-id", "outside-rev", since.Add(-time.Second)},
	} {
		if err := store.SaveLatencyTest(ctx, &LatencyTest{AttemptID: item.id, ProfileID: item.profile, DisplayName: item.name, NodeKey: item.identity, NodeIdentityKey: item.identity, ConfigRevisionKey: item.revision, TestProject: "latency_stability", Status: "completed", RequestedAt: item.at, StartedAt: item.at, FinishedAt: item.at, Samples: []LatencyTestSample{{Seq: 1, Timestamp: item.at, Success: true}}}); err != nil {
			t.Fatal(err)
		}
	}
	// A round finishing after the upper boundary still has raw samples inside it.
	if err := store.SaveLatencyTest(ctx, &LatencyTest{AttemptID: "straddled", ProfileID: "account-a", DisplayName: "Node", NodeKey: "straddled-id", NodeIdentityKey: "straddled-id", ConfigRevisionKey: "revision", TestProject: "latency_stability", Status: "completed", RequestedAt: at, StartedAt: at, FinishedAt: until.Add(time.Second), Samples: []LatencyTestSample{{Seq: 1, Timestamp: at, Success: true}}}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMonitorRun(ctx, &monitor.MonitorRun{RunID: "run", JobID: "job", ScheduledAt: at, StartedAt: at, Status: monitor.RunStatusCompleted}); err != nil {
		t.Fatal(err)
	}
	var samples []*monitor.MonitorSample
	for _, item := range []struct {
		id, profile, name, probe, identity string
		at                                 time.Time
	}{
		{"monitor", "account-a", "Node", "rtt", "monitor-id", until},
		{"duplicate", "account-a", "Node", "rtt", "monitor-id", since},
		{"monitor-other-account", "account-b", "Node", "rtt", "other-id", at},
		{"monitor-renamed", "account-a", "Other name", "rtt", "renamed-id", at},
		{"service", "account-a", "Node", "http", "service-id", at},
		{"future", "account-a", "Node", "rtt", "future-id", until},
	} {
		samples = append(samples, &monitor.MonitorSample{SampleID: item.id, RunID: "run", ProfileID: item.profile, DisplayNameSnapshot: item.name, ProbeType: item.probe, NodeKey: item.identity, NodeIdentityKey: item.identity, ConfigRevisionKey: "monitor-rev", Timestamp: item.at, Target: "https://example.test", Success: true})
	}
	if err := store.SaveMonitorSamples(ctx, samples); err != nil {
		t.Fatal(err)
	}
	got, err := store.DB().ListLatencyHistoryScopes(ctx, "account-a", "Node", since, until)
	want := []LatencyHistoryScope{
		{ProfileID: "account-a", NodeKey: "monitor-id", NodeIdentityKey: "monitor-id", ConfigRevisionKey: "monitor-rev"},
		{ProfileID: "account-a", NodeKey: "new-id", NodeIdentityKey: "new-id", ConfigRevisionKey: "shared-rev"},
		{ProfileID: "account-a", NodeKey: "old-id", NodeIdentityKey: "old-id", ConfigRevisionKey: "shared-rev"},
		{ProfileID: "account-a", NodeKey: "straddled-id", NodeIdentityKey: "straddled-id", ConfigRevisionKey: "revision"},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("scopes = %+v, err = %v; want %+v", got, err, want)
	}
	got, err = store.DB().ListLatencyHistoryScopes(ctx, "missing-account", "Node", since, until)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty scope list = %#v, err = %v", got, err)
	}
}
