package history

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
)

func TestSQLite_PersistenceAndReopen(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	// 1. Initial creation and write
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	t0 := time.Now().Truncate(time.Millisecond)
	runID := "run_test_001"
	jobID := "job_24h_001"

	run := &monitor.MonitorRun{
		RunID:        runID,
		JobID:        jobID,
		ScheduledAt:  t0,
		StartedAt:    t0.Add(50 * time.Millisecond),
		Status:       monitor.RunStatusRunning,
		TotalNodes:   2,
		SuccessNodes: 0,
		FailedNodes:  0,
	}

	if err := store.SaveMonitorRun(ctx, run); err != nil {
		t.Fatalf("SaveMonitorRun error: %v", err)
	}

	samples := []*monitor.MonitorSample{
		{
			SampleID:            "s_001",
			RunID:               runID,
			NodeKey:             "nk_ss_node1_8388_a1",
			ProfileID:           "prof_1",
			DisplayNameSnapshot: "HK-01",
			ProbeType:           "rtt",
			Target:              "https://cp.cloudflare.com/generate_204",
			Timestamp:           t0.Add(100 * time.Millisecond),
			Success:             true,
			Latency:             35 * time.Millisecond,
			TTFB:                30 * time.Millisecond,
			ErrorClass:          "none",
			ExitIP:              "1.1.1.1",
			ExitRegion:          "HK",
			Metadata:            map[string]any{"colo": "HKG"},
		},
		{
			SampleID:            "s_002",
			RunID:               runID,
			NodeKey:             "nk_vmess_node2_443_b2",
			ProfileID:           "prof_1",
			DisplayNameSnapshot: "JP-01",
			ProbeType:           "rtt",
			Target:              "https://cp.cloudflare.com/generate_204",
			Timestamp:           t0.Add(200 * time.Millisecond),
			Success:             false,
			Latency:             0,
			TTFB:                0,
			ErrorClass:          "timeout",
			ErrorDetail:         "i/o timeout after 5s",
		},
	}

	if err := store.SaveMonitorSamples(ctx, samples); err != nil {
		t.Fatalf("SaveMonitorSamples error: %v", err)
	}

	finishedAt := t0.Add(300 * time.Millisecond)
	run.FinishedAt = &finishedAt
	run.Status = monitor.RunStatusCompleted
	run.SuccessNodes = 1
	run.FailedNodes = 1

	if err := store.UpdateMonitorRun(ctx, run); err != nil {
		t.Fatalf("UpdateMonitorRun error: %v", err)
	}

	// 2. Explicitly close DB
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close error: %v", err)
	}

	// 3. Reopen from same directory
	reopenedStore, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("reopen NewStore error: %v", err)
	}
	defer reopenedStore.Close()

	// 4. Verify run persisted after reopen
	runs, err := reopenedStore.QueryMonitorRuns(ctx, jobID, 10)
	if err != nil {
		t.Fatalf("QueryMonitorRuns error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].RunID != runID || runs[0].Status != monitor.RunStatusCompleted {
		t.Errorf("unexpected run data: %+v", runs[0])
	}
	if runs[0].SuccessNodes != 1 || runs[0].FailedNodes != 1 {
		t.Errorf("unexpected node counts: %+v", runs[0])
	}

	// 5. Verify samples persisted after reopen
	queriedSamples, err := reopenedStore.QueryMonitorSamples(ctx, monitor.SampleFilter{
		RunID: runID,
	})
	if err != nil {
		t.Fatalf("QueryMonitorSamples error: %v", err)
	}
	if len(queriedSamples) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(queriedSamples))
	}

	s1 := queriedSamples[0]
	if s1.NodeKey != "nk_ss_node1_8388_a1" || !s1.Success || s1.Latency != 35*time.Millisecond {
		t.Errorf("unexpected sample 1: %+v", s1)
	}
	if s1.Metadata["colo"] != "HKG" {
		t.Errorf("expected metadata colo HKG, got %v", s1.Metadata)
	}

	s2 := queriedSamples[1]
	if s2.NodeKey != "nk_vmess_node2_443_b2" || s2.Success || s2.ErrorClass != "timeout" {
		t.Errorf("unexpected sample 2: %+v", s2)
	}
}

func TestSQLite_RawSampleChronologicalOrdering(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	defer store.Close()

	baseTime := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	nodeKey := "nk_ss_hk_8388_xyz"

	// Intentionally generate samples with timestamps in out-of-order sequence
	offsets := []time.Duration{
		5 * time.Minute,
		1 * time.Minute,
		4 * time.Minute,
		2 * time.Minute,
		3 * time.Minute,
	}

	var batch []*monitor.MonitorSample
	for i, offset := range offsets {
		batch = append(batch, &monitor.MonitorSample{
			SampleID:            filepath.Join("s", string(rune('a'+i))),
			RunID:               "run_batch",
			NodeKey:             nodeKey,
			ProfileID:           "prof_1",
			DisplayNameSnapshot: "HK-01",
			ProbeType:           "rtt",
			Target:              "https://target.com",
			Timestamp:           baseTime.Add(offset),
			Success:             true,
			Latency:             time.Duration(20+i) * time.Millisecond,
		})
	}

	if err := store.SaveMonitorSamples(ctx, batch); err != nil {
		t.Fatalf("SaveMonitorSamples error: %v", err)
	}

	// 1. Query ascending (chronological timeline)
	ascSamples, err := store.QueryMonitorSamples(ctx, monitor.SampleFilter{
		NodeKey:   nodeKey,
		OrderDesc: false,
	})
	if err != nil {
		t.Fatalf("Query ascending error: %v", err)
	}
	if len(ascSamples) != 5 {
		t.Fatalf("expected 5 samples, got %d", len(ascSamples))
	}
	for i := 0; i < len(ascSamples)-1; i++ {
		if ascSamples[i].Timestamp.After(ascSamples[i+1].Timestamp) {
			t.Errorf("expected ascending order, but sample %d (%v) is after sample %d (%v)",
				i, ascSamples[i].Timestamp, i+1, ascSamples[i+1].Timestamp)
		}
	}

	// 2. Query descending (newest first)
	descSamples, err := store.QueryMonitorSamples(ctx, monitor.SampleFilter{
		NodeKey:   nodeKey,
		OrderDesc: true,
	})
	if err != nil {
		t.Fatalf("Query descending error: %v", err)
	}
	if len(descSamples) != 5 {
		t.Fatalf("expected 5 samples, got %d", len(descSamples))
	}
	for i := 0; i < len(descSamples)-1; i++ {
		if descSamples[i].Timestamp.Before(descSamples[i+1].Timestamp) {
			t.Errorf("expected descending order, but sample %d (%v) is before sample %d (%v)",
				i, descSamples[i].Timestamp, i+1, descSamples[i+1].Timestamp)
		}
	}

	// 3. Query with range filter (Since: baseTime + 2m, Until: baseTime + 4m)
	since := baseTime.Add(2 * time.Minute)
	until := baseTime.Add(4 * time.Minute)

	rangeSamples, err := store.QueryMonitorSamples(ctx, monitor.SampleFilter{
		NodeKey:   nodeKey,
		Since:     &since,
		Until:     &until,
		OrderDesc: false,
	})
	if err != nil {
		t.Fatalf("Query range error: %v", err)
	}
	// Expected: 2m, 3m, 4m (3 samples)
	if len(rangeSamples) != 3 {
		t.Fatalf("expected 3 samples in range [2m, 4m], got %d", len(rangeSamples))
	}

	// 4. Test GetNodeTimelineSamples helper
	timelineSamples, err := store.GetNodeTimelineSamples(ctx, nodeKey, baseTime.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("GetNodeTimelineSamples error: %v", err)
	}
	// Expected: 3m, 4m, 5m (3 samples)
	if len(timelineSamples) != 3 {
		t.Fatalf("expected 3 timeline samples since 3m, got %d", len(timelineSamples))
	}
}
