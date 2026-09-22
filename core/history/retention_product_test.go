package history

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
)

func TestRetentionPreviewAndCanonicalDeletion(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	cutoff := time.Now().UTC().AddDate(0, 0, -90)
	for _, fixture := range []struct {
		id     string
		at     time.Time
		status monitor.RunStatus
	}{
		{"old", cutoff.Add(-time.Hour), monitor.RunStatusCompleted},
		{"boundary", cutoff, monitor.RunStatusCompleted},
		{"recent", cutoff.Add(time.Hour), monitor.RunStatusCompleted},
		{"running", cutoff.Add(-time.Hour), monitor.RunStatusRunning},
	} {
		if err := store.SaveMonitorRun(ctx, &monitor.MonitorRun{RunID: fixture.id, JobID: "job", ScheduledAt: fixture.at, StartedAt: fixture.at, Status: fixture.status}); err != nil {
			t.Fatal(err)
		}
		if fixture.id != "running" {
			if err := store.SaveMonitorSamples(ctx, []*monitor.MonitorSample{{SampleID: fixture.id, RunID: fixture.id, NodeKey: "node", Timestamp: fixture.at, Success: true}}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := store.SaveLatencyTest(ctx, &LatencyTest{AttemptID: "workbench", ProfileID: "profile", NodeKey: "node", NodeIdentityKey: "identity", ConfigRevisionKey: "revision", DisplayName: "node", NodeType: "ss", TestProject: "latency", RequestedAt: cutoff, StartedAt: cutoff, FinishedAt: cutoff, Status: "completed", TotalSamples: 1, SuccessSamples: 1, Samples: []LatencyTestSample{{Seq: 1, Timestamp: cutoff, Success: true}}}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMonitorJobDefinition(ctx, &monitor.MonitorJobDefinition{ID: "job", Name: "saved", ProfileID: "profile", Nodes: []monitor.MonitorJobNodeReference{{NodeKey: "node", NodeIdentityKey: "identity", ConfigRevisionKey: "revision", DisplayName: "node", Type: "ss"}}, ProbeSet: monitor.ProbeSetLight, Interval: time.Minute, Timeout: time.Second, CreatedAt: cutoff, UpdatedAt: cutoff, DefinitionVersion: monitor.MonitorJobDefinitionVersion}); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(dir, "legacy", "old.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, []byte(`{"legacy":true}`), 0o600); err != nil {
		t.Fatal(err)
	}

	preview, err := store.PreviewRetention(ctx, monitor.RetentionRequest{Policy: monitor.Retention90d, CutoffTime: &cutoff})
	if err != nil {
		t.Fatal(err)
	}
	if preview.SamplesToDelete != 1 || preview.RunsToDelete != 1 {
		t.Fatalf("unexpected DB preview: %+v", preview)
	}
	usage, err := store.StorageUsage(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	var measured int64
	for _, name := range []string{"history.db", "history.db-wal", "history.db-shm"} {
		info, statErr := os.Stat(filepath.Join(dir, name))
		if statErr == nil {
			measured += info.Size()
		} else if !os.IsNotExist(statErr) {
			t.Fatal(statErr)
		}
	}
	if usage.TotalBytes != measured || usage.TotalBytes != usage.DatabaseBytes+usage.WALBytes+usage.SharedMemoryBytes || !usage.Protected {
		t.Fatalf("SQLite file accounting mismatch: %+v, stat=%d", usage, measured)
	}
	result, err := store.ApplyRetention(ctx, monitor.RetentionRequest{Policy: monitor.Retention90d, CutoffTime: &preview.Cutoff})
	if err != nil {
		t.Fatal(err)
	}
	if result.SamplesDeleted != 1 || result.RunsDeleted != 1 || result.Partial {
		t.Fatalf("unexpected deletion: %+v", result)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	samples, err := store.QueryMonitorSamples(ctx, monitor.SampleFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 2 {
		t.Fatalf("boundary and recent samples must remain: %+v", samples)
	}
	remainingIDs := map[string]bool{}
	for _, sample := range samples {
		remainingIDs[sample.SampleID] = true
	}
	if !remainingIDs["boundary"] || !remainingIDs["recent"] || remainingIDs["old"] {
		t.Fatalf("wrong Monitor sample identity after retention: %+v", remainingIDs)
	}
	runs, err := store.QueryMonitorRuns(ctx, "job", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 3 {
		t.Fatalf("running/boundary/recent runs must remain: %+v", runs)
	}
	if definitions, err := store.ListMonitorJobDefinitions(ctx); err != nil || len(definitions) != 1 {
		t.Fatalf("job definition changed: %+v %v", definitions, err)
	}
	if tests, err := store.QueryLatencyTests(ctx, LatencyTestFilter{ProfileID: "profile", NodeKey: "node", Limit: 10}); err != nil || len(tests.Tests) != 1 {
		t.Fatalf("Workbench history changed: %+v %v", tests, err)
	}
	if raw, err := os.ReadFile(legacyPath); err != nil || string(raw) != `{"legacy":true}` {
		t.Fatalf("legacy changed: %q %v", raw, err)
	}
}
