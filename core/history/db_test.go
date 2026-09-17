package history

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
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

func TestSQLite_SameTimestampPaginationDeterministicOrder(t *testing.T) {
	tmpDir := t.TempDir()

	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	fixedTime := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	nodeKey := "nk_same_timestamp_test"

	// Insert 6 samples with identical timestamp
	var batch []*monitor.MonitorSample
	expectedIDs := []string{"sample_01", "sample_02", "sample_03", "sample_04", "sample_05", "sample_06"}
	for _, id := range expectedIDs {
		batch = append(batch, &monitor.MonitorSample{
			SampleID:  id,
			RunID:     "run_fixed",
			NodeKey:   nodeKey,
			ProbeType: "rtt",
			Target:    "https://example.com",
			Timestamp: fixedTime,
			Success:   true,
			Latency:   50 * time.Millisecond,
		})
	}
	if err := store.SaveMonitorSamples(ctx, batch); err != nil {
		t.Fatalf("SaveMonitorSamples failed: %v", err)
	}

	// 1. Ascending pagination (Limit 3, Offset 0 and Offset 3)
	page1Asc, err := store.QueryMonitorSamples(ctx, monitor.SampleFilter{
		NodeKey:   nodeKey,
		OrderDesc: false,
		Limit:     3,
		Offset:    0,
	})
	if err != nil {
		t.Fatalf("Page 1 ASC failed: %v", err)
	}
	if len(page1Asc) != 3 {
		t.Fatalf("Expected 3 items in Page 1 ASC, got %d", len(page1Asc))
	}

	page2Asc, err := store.QueryMonitorSamples(ctx, monitor.SampleFilter{
		NodeKey:   nodeKey,
		OrderDesc: false,
		Limit:     3,
		Offset:    3,
	})
	if err != nil {
		t.Fatalf("Page 2 ASC failed: %v", err)
	}
	if len(page2Asc) != 3 {
		t.Fatalf("Expected 3 items in Page 2 ASC, got %d", len(page2Asc))
	}

	var combinedAsc []string
	for _, s := range page1Asc {
		combinedAsc = append(combinedAsc, s.SampleID)
	}
	for _, s := range page2Asc {
		combinedAsc = append(combinedAsc, s.SampleID)
	}

	for i, id := range expectedIDs {
		if combinedAsc[i] != id {
			t.Errorf("ASC pagination mismatch at %d: expected %s, got %s", i, id, combinedAsc[i])
		}
	}

	// 2. Descending pagination (Limit 3, Offset 0 and Offset 3)
	page1Desc, err := store.QueryMonitorSamples(ctx, monitor.SampleFilter{
		NodeKey:   nodeKey,
		OrderDesc: true,
		Limit:     3,
		Offset:    0,
	})
	if err != nil {
		t.Fatalf("Page 1 DESC failed: %v", err)
	}
	page2Desc, err := store.QueryMonitorSamples(ctx, monitor.SampleFilter{
		NodeKey:   nodeKey,
		OrderDesc: true,
		Limit:     3,
		Offset:    3,
	})
	if err != nil {
		t.Fatalf("Page 2 DESC failed: %v", err)
	}

	var combinedDesc []string
	for _, s := range page1Desc {
		combinedDesc = append(combinedDesc, s.SampleID)
	}
	for _, s := range page2Desc {
		combinedDesc = append(combinedDesc, s.SampleID)
	}

	expectedDesc := []string{"sample_06", "sample_05", "sample_04", "sample_03", "sample_02", "sample_01"}
	for i, id := range expectedDesc {
		if combinedDesc[i] != id {
			t.Errorf("DESC pagination mismatch at %d: expected %s, got %s", i, id, combinedDesc[i])
		}
	}
}

func TestSQLite_MigrationFromPR3Schema(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "history.db")
	ctx := context.Background()

	// 1. Manually create PR #3 schema (without node_identity_key and config_revision_key)
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}

	pr3Schema := `
	CREATE TABLE monitor_runs (
		run_id TEXT PRIMARY KEY,
		job_id TEXT NOT NULL,
		scheduled_at DATETIME NOT NULL,
		started_at DATETIME NOT NULL,
		finished_at DATETIME,
		status TEXT NOT NULL,
		total_nodes INTEGER NOT NULL DEFAULT 0,
		success_nodes INTEGER NOT NULL DEFAULT 0,
		failed_nodes INTEGER NOT NULL DEFAULT 0,
		error_message TEXT
	);

	CREATE TABLE monitor_samples (
		sample_id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		node_key TEXT NOT NULL,
		profile_id TEXT NOT NULL,
		display_name_snapshot TEXT NOT NULL,
		probe_type TEXT NOT NULL,
		target TEXT NOT NULL,
		timestamp DATETIME NOT NULL,
		success INTEGER NOT NULL,
		latency_ms INTEGER NOT NULL DEFAULT 0,
		ttfb_ms INTEGER NOT NULL DEFAULT 0,
		error_class TEXT NOT NULL DEFAULT 'none',
		error_detail TEXT,
		exit_ip TEXT,
		exit_region TEXT,
		metadata_json TEXT
	);
	`
	if _, err := rawDB.Exec(pr3Schema); err != nil {
		t.Fatalf("create pr3 schema: %v", err)
	}

	// Insert old sample
	t0 := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	_, err = rawDB.Exec(`
		INSERT INTO monitor_samples (
			sample_id, run_id, node_key, profile_id, display_name_snapshot,
			probe_type, target, timestamp, success, latency_ms, ttfb_ms
		) VALUES ('legacy_s1', 'run_0', 'nk_legacy_node', 'prof_1', 'Legacy Node', 'rtt', 'https://target.com', ?, 1, 40, 35)
	`, t0)
	if err != nil {
		t.Fatalf("insert legacy sample: %v", err)
	}
	_ = rawDB.Close()

	// 2. Open via OpenDB (should execute idempotent migrateSchema)
	db, err := OpenDB(tmpDir)
	if err != nil {
		t.Fatalf("OpenDB on legacy PR#3 database failed: %v", err)
	}
	defer db.Close()

	// 3. Verify legacy sample was backfilled with node_identity_key = node_key
	samples, err := db.QueryMonitorSamples(ctx, monitor.SampleFilter{
		NodeIdentityKey: "nk_legacy_node",
	})
	if err != nil {
		t.Fatalf("QueryMonitorSamples with backfilled NodeIdentityKey failed: %v", err)
	}
	if len(samples) != 1 || samples[0].SampleID != "legacy_s1" {
		t.Fatalf("expected 1 legacy sample with backfilled identity key, got %+v", samples)
	}
	if samples[0].NodeIdentityKey != "nk_legacy_node" {
		t.Errorf("expected NodeIdentityKey to be backfilled from node_key, got %s", samples[0].NodeIdentityKey)
	}

	// 4. Save new sample with distinct node_identity_key and config_revision_key
	newSample := &monitor.MonitorSample{
		SampleID:          "new_s2",
		RunID:             "run_0",
		NodeKey:           "nk_new_node",
		NodeIdentityKey:   "nid_vmess_server1_443_abc",
		ConfigRevisionKey: "rev_vmess_12345",
		ProfileID:         "prof_1",
		ProbeType:         "rtt",
		Target:            "https://target.com",
		Timestamp:         t0.Add(time.Minute),
		Success:           true,
		Latency:           25 * time.Millisecond,
	}
	if err := db.SaveMonitorSamples(ctx, []*monitor.MonitorSample{newSample}); err != nil {
		t.Fatalf("SaveMonitorSamples with new keys failed: %v", err)
	}

	newQueried, err := db.QueryMonitorSamples(ctx, monitor.SampleFilter{
		NodeIdentityKey: "nid_vmess_server1_443_abc",
	})
	if err != nil {
		t.Fatalf("query new sample failed: %v", err)
	}
	if len(newQueried) != 1 || newQueried[0].ConfigRevisionKey != "rev_vmess_12345" {
		t.Errorf("unexpected new sample result: %+v", newQueried)
	}
}

func TestSQLite_CursorPagination_SameTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	fixedTime := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	nodeKey := "nk_same_ts_cursor"
	nodeID := "nid_same_ts"

	var samples []*monitor.MonitorSample
	for i := 1; i <= 6; i++ {
		samples = append(samples, &monitor.MonitorSample{
			SampleID:        fmt.Sprintf("s_%02d", i),
			RunID:           "run_cursor",
			NodeKey:         nodeKey,
			NodeIdentityKey: nodeID,
			ProbeType:       "rtt",
			Target:          "https://target.com",
			Timestamp:       fixedTime,
			Success:         true,
			Latency:         time.Duration(i*10) * time.Millisecond,
		})
	}
	if err := store.SaveMonitorSamples(ctx, samples); err != nil {
		t.Fatalf("SaveMonitorSamples failed: %v", err)
	}

	// 1. Keyset pagination DESC (Limit 2)
	// Expected page 1: s_06, s_05
	p1, err := store.QueryMonitorSamplesCursor(ctx, monitor.CursorFilter{
		NodeIdentityKey: nodeID,
		OrderDesc:       true,
		Limit:           2,
	})
	if err != nil {
		t.Fatalf("Cursor p1 failed: %v", err)
	}
	if len(p1.Items) != 2 || !p1.HasMore {
		t.Fatalf("unexpected p1: len=%d, hasMore=%v", len(p1.Items), p1.HasMore)
	}
	if p1.Items[0].SampleID != "s_06" || p1.Items[1].SampleID != "s_05" {
		t.Errorf("p1 mismatch: %s, %s", p1.Items[0].SampleID, p1.Items[1].SampleID)
	}

	// Page 2: s_04, s_03
	p2, err := store.QueryMonitorSamplesCursor(ctx, monitor.CursorFilter{
		NodeIdentityKey: nodeID,
		OrderDesc:       true,
		Limit:           2,
		Cursor:          p1.NextCursor,
	})
	if err != nil {
		t.Fatalf("Cursor p2 failed: %v", err)
	}
	if len(p2.Items) != 2 || !p2.HasMore {
		t.Fatalf("unexpected p2: len=%d, hasMore=%v", len(p2.Items), p2.HasMore)
	}
	if p2.Items[0].SampleID != "s_04" || p2.Items[1].SampleID != "s_03" {
		t.Errorf("p2 mismatch: %s, %s", p2.Items[0].SampleID, p2.Items[1].SampleID)
	}

	// Page 3: s_02, s_01
	p3, err := store.QueryMonitorSamplesCursor(ctx, monitor.CursorFilter{
		NodeIdentityKey: nodeID,
		OrderDesc:       true,
		Limit:           2,
		Cursor:          p2.NextCursor,
	})
	if err != nil {
		t.Fatalf("Cursor p3 failed: %v", err)
	}
	if len(p3.Items) != 2 || p3.HasMore {
		t.Fatalf("unexpected p3: len=%d, hasMore=%v", len(p3.Items), p3.HasMore)
	}
	if p3.Items[0].SampleID != "s_02" || p3.Items[1].SampleID != "s_01" {
		t.Errorf("p3 mismatch: %s, %s", p3.Items[0].SampleID, p3.Items[1].SampleID)
	}
}

func TestSQLite_CursorPagination_DynamicInsertion(t *testing.T) {
	// Requirements:
	// "必须增加“分页过程中插入新 Sample”测试，证明不会造成旧窗口重复或遗漏。"
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	baseTime := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	nodeID := "nid_dynamic_insertion"

	// 1. Initial 10 samples (t=1..10 min)
	var initialBatch []*monitor.MonitorSample
	for i := 1; i <= 10; i++ {
		initialBatch = append(initialBatch, &monitor.MonitorSample{
			SampleID:        fmt.Sprintf("init_%02d", i),
			RunID:           "run_init",
			NodeIdentityKey: nodeID,
			NodeKey:         "nk_dyn",
			ProbeType:       "rtt",
			Target:          "https://target.com",
			Timestamp:       baseTime.Add(time.Duration(i) * time.Minute),
			Success:         true,
			Latency:         time.Duration(i*5) * time.Millisecond,
		})
	}
	if err := store.SaveMonitorSamples(ctx, initialBatch); err != nil {
		t.Fatalf("Save initial samples: %v", err)
	}

	// 2. Fetch Page 1 (Limit 5, DESC).
	// Should return init_10 down to init_06 (the newest 5)
	p1, err := store.QueryMonitorSamplesCursor(ctx, monitor.CursorFilter{
		NodeIdentityKey: nodeID,
		OrderDesc:       true,
		Limit:           5,
	})
	if err != nil {
		t.Fatalf("Page 1 query: %v", err)
	}
	if len(p1.Items) != 5 || !p1.HasMore {
		t.Fatalf("p1 expected 5 items with hasMore=true, got %d, %v", len(p1.Items), p1.HasMore)
	}
	for idx, expectedID := range []string{"init_10", "init_09", "init_08", "init_07", "init_06"} {
		if p1.Items[idx].SampleID != expectedID {
			t.Errorf("p1 item %d: expected %s, got %s", idx, expectedID, p1.Items[idx].SampleID)
		}
	}

	// 3. Concurrently / Dynamically insert 5 brand-new samples with newer timestamps (t=11..15 min)
	var dynamicBatch []*monitor.MonitorSample
	for i := 11; i <= 15; i++ {
		dynamicBatch = append(dynamicBatch, &monitor.MonitorSample{
			SampleID:        fmt.Sprintf("dynamic_%02d", i),
			RunID:           "run_dyn",
			NodeIdentityKey: nodeID,
			NodeKey:         "nk_dyn",
			ProbeType:       "rtt",
			Target:          "https://target.com",
			Timestamp:       baseTime.Add(time.Duration(i) * time.Minute),
			Success:         true,
			Latency:         time.Duration(i*5) * time.Millisecond,
		})
	}
	if err := store.SaveMonitorSamples(ctx, dynamicBatch); err != nil {
		t.Fatalf("Save dynamic samples: %v", err)
	}

	// 4. Client now fetches Page 2 using p1.NextCursor.
	// With keyset pagination (timestamp < cursor.timestamp), the newly inserted records
	// MUST NOT shift the window or cause duplicates of p1, and MUST NOT skip init_05..init_01!
	p2, err := store.QueryMonitorSamplesCursor(ctx, monitor.CursorFilter{
		NodeIdentityKey: nodeID,
		OrderDesc:       true,
		Limit:           5,
		Cursor:          p1.NextCursor,
	})
	if err != nil {
		t.Fatalf("Page 2 query: %v", err)
	}
	if len(p2.Items) != 5 {
		t.Fatalf("p2 expected 5 items, got %d", len(p2.Items))
	}
	for idx, expectedID := range []string{"init_05", "init_04", "init_03", "init_02", "init_01"} {
		if p2.Items[idx].SampleID != expectedID {
			t.Errorf("p2 item %d: expected %s, got %s (pagination corrupted by dynamic insertion!)", idx, expectedID, p2.Items[idx].SampleID)
		}
	}

	// Verify no duplicates between p1 and p2
	seen := make(map[string]bool)
	for _, it := range p1.Items {
		seen[it.SampleID] = true
	}
	for _, it := range p2.Items {
		if seen[it.SampleID] {
			t.Errorf("duplicate item %s detected between page 1 and page 2 after dynamic insertion", it.SampleID)
		}
	}
}

func TestSQLite_DerivedStats_Accuracy(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	nodeID := "nid_stats_test"

	// 1. Edge Case: 0 samples
	emptyStats, err := store.GetDerivedStats(ctx, monitor.StatsQuery{
		NodeIdentityKey: nodeID,
	})
	if err != nil {
		t.Fatalf("empty stats query failed: %v", err)
	}
	if emptyStats.SampleCount != 0 || emptyStats.SuccessRate != 0.0 || emptyStats.LatencyP50Ms != nil {
		t.Errorf("expected zero/nil values for empty stats, got %+v", emptyStats)
	}

	// 2. Edge Case: All failed samples
	baseTime := time.Date(2026, 9, 17, 14, 0, 0, 0, time.UTC)
	failedSamples := []*monitor.MonitorSample{
		{
			SampleID:        "fail_1",
			RunID:           "run_fail",
			NodeIdentityKey: nodeID,
			NodeKey:         "nk_fail",
			ProbeType:       "rtt",
			Target:          "https://target.com",
			Timestamp:       baseTime,
			Success:         false,
			ErrorClass:      "timeout",
		},
		{
			SampleID:        "fail_2",
			RunID:           "run_fail",
			NodeIdentityKey: nodeID,
			NodeKey:         "nk_fail",
			ProbeType:       "rtt",
			Target:          "https://target.com",
			Timestamp:       baseTime.Add(time.Minute),
			Success:         false,
			ErrorClass:      "dns_error",
		},
	}
	if err := store.SaveMonitorSamples(ctx, failedSamples); err != nil {
		t.Fatalf("save failed samples: %v", err)
	}

	failStats, err := store.GetDerivedStats(ctx, monitor.StatsQuery{NodeIdentityKey: nodeID})
	if err != nil {
		t.Fatalf("failed stats query: %v", err)
	}
	if failStats.SampleCount != 2 || failStats.SuccessCount != 0 || failStats.FailureCount != 2 {
		t.Errorf("unexpected counts: %+v", failStats)
	}
	if failStats.SuccessRate != 0.0 || failStats.LatencyP50Ms != nil {
		t.Errorf("unexpected success rate / latency: %+v", failStats)
	}
	if failStats.ErrorBreakdown["timeout"] != 1 || failStats.ErrorBreakdown["dns_error"] != 1 {
		t.Errorf("unexpected error breakdown: %+v", failStats.ErrorBreakdown)
	}

	// 3. Mixed / Successful samples with exact percentiles
	var successSamples []*monitor.MonitorSample
	latencies := []int64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100} // 10 successful samples
	for i, lat := range latencies {
		successSamples = append(successSamples, &monitor.MonitorSample{
			SampleID:        fmt.Sprintf("succ_%d", i),
			RunID:           "run_succ",
			NodeIdentityKey: nodeID,
			NodeKey:         "nk_succ",
			ProbeType:       "rtt",
			Target:          "https://target.com",
			Timestamp:       baseTime.Add(time.Duration(i+2) * time.Minute),
			Success:         true,
			Latency:         time.Duration(lat) * time.Millisecond,
			TTFB:            time.Duration(lat-5) * time.Millisecond,
		})
	}
	if err := store.SaveMonitorSamples(ctx, successSamples); err != nil {
		t.Fatalf("save success samples: %v", err)
	}

	// Query mixed stats: 10 success + 2 fail = 12 total
	mixedStats, err := store.GetDerivedStats(ctx, monitor.StatsQuery{NodeIdentityKey: nodeID})
	if err != nil {
		t.Fatalf("mixed stats query: %v", err)
	}
	if mixedStats.SampleCount != 12 || mixedStats.SuccessCount != 10 || mixedStats.FailureCount != 2 {
		t.Errorf("unexpected mixed counts: %+v", mixedStats)
	}
	// 10 / 12 = 0.8333
	if mixedStats.SuccessRate != 0.8333 {
		t.Errorf("expected SuccessRate 0.8333, got %f", mixedStats.SuccessRate)
	}
	if *mixedStats.LatencyMinMs != 10 || *mixedStats.LatencyMaxMs != 100 {
		t.Errorf("min/max latency mismatch: min=%v, max=%v", *mixedStats.LatencyMinMs, *mixedStats.LatencyMaxMs)
	}
	// 10 samples: p50 index = ceil(0.50*10)-1 = 4 -> 50ms; p95 index = ceil(0.95*10)-1 = 9 -> 100ms
	if *mixedStats.LatencyP50Ms != 50 {
		t.Errorf("expected LatencyP50Ms=50, got %v", *mixedStats.LatencyP50Ms)
	}
	if *mixedStats.LatencyP95Ms != 100 {
		t.Errorf("expected LatencyP95Ms=100, got %v", *mixedStats.LatencyP95Ms)
	}
}

func TestSQLite_Retention_KeepAllByDefault(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	sample := &monitor.MonitorSample{
		SampleID:  "keep_me",
		RunID:     "run_keep",
		NodeKey:   "nk_1",
		Timestamp: time.Now().AddDate(0, 0, -100), // 100 days old
		Success:   true,
	}
	_ = store.SaveMonitorSamples(ctx, []*monitor.MonitorSample{sample})

	// Apply KeepAll policy
	res, err := store.ApplyRetention(ctx, monitor.RetentionRequest{
		Policy: monitor.RetentionKeepAll,
	})
	if err != nil {
		t.Fatalf("ApplyRetention KeepAll failed: %v", err)
	}
	if res.SamplesDeleted != 0 || res.RunsDeleted != 0 {
		t.Errorf("KeepAll must not delete any samples or runs: %+v", res)
	}

	// Verify sample remains intact
	remaining, err := store.QueryMonitorSamples(ctx, monitor.SampleFilter{})
	if err != nil || len(remaining) != 1 {
		t.Fatalf("expected 1 remaining sample, got %d", len(remaining))
	}
}

func TestSQLite_Retention_BatchedCutoff(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	cutoff := now.AddDate(0, 0, -30)

	// Save completed runs: run_old (all samples older than cutoff), run_new (has recent samples)
	_ = store.SaveMonitorRun(ctx, &monitor.MonitorRun{
		RunID:        "run_old",
		JobID:        "job_retention",
		ScheduledAt:  cutoff.Add(-2 * time.Hour),
		StartedAt:    cutoff.Add(-2 * time.Hour),
		Status:       monitor.RunStatusCompleted,
		TotalNodes:   1,
		SuccessNodes: 1,
	})
	_ = store.SaveMonitorRun(ctx, &monitor.MonitorRun{
		RunID:        "run_new",
		JobID:        "job_retention",
		ScheduledAt:  now.Add(-1 * time.Hour),
		StartedAt:    now.Add(-1 * time.Hour),
		Status:       monitor.RunStatusCompleted,
		TotalNodes:   1,
		SuccessNodes: 1,
	})

	// Insert 1200 old samples (to test bounded batching with batchSize 500)
	var oldSamples []*monitor.MonitorSample
	for i := 0; i < 1200; i++ {
		oldSamples = append(oldSamples, &monitor.MonitorSample{
			SampleID:  fmt.Sprintf("old_%04d", i),
			RunID:     "run_old",
			NodeKey:   "nk_1",
			Timestamp: cutoff.Add(-time.Duration(i+1) * time.Minute),
			Success:   true,
		})
	}
	if err := store.SaveMonitorSamples(ctx, oldSamples); err != nil {
		t.Fatalf("save old samples: %v", err)
	}

	// Insert 50 new samples (should NOT be deleted)
	var newSamples []*monitor.MonitorSample
	for i := 0; i < 50; i++ {
		newSamples = append(newSamples, &monitor.MonitorSample{
			SampleID:  fmt.Sprintf("new_%04d", i),
			RunID:     "run_new",
			NodeKey:   "nk_1",
			Timestamp: now.Add(-time.Duration(i+1) * time.Minute),
			Success:   true,
		})
	}
	if err := store.SaveMonitorSamples(ctx, newSamples); err != nil {
		t.Fatalf("save new samples: %v", err)
	}

	// Apply Retention: 30d
	result, err := store.ApplyRetention(ctx, monitor.RetentionRequest{
		Policy:     monitor.Retention30d,
		CutoffTime: &cutoff,
	})
	if err != nil {
		t.Fatalf("ApplyRetention failed: %v", err)
	}

	if result.SamplesDeleted != 1200 {
		t.Errorf("expected 1200 samples deleted in batches, got %d", result.SamplesDeleted)
	}
	if result.RunsDeleted != 1 {
		t.Errorf("expected 1 orphaned run deleted, got %d", result.RunsDeleted)
	}

	// Verify remaining samples: exactly 50
	remaining, err := store.QueryMonitorSamples(ctx, monitor.SampleFilter{})
	if err != nil || len(remaining) != 50 {
		t.Fatalf("expected exactly 50 preserved samples, got %d", len(remaining))
	}

	// Verify remaining runs: only run_new remains
	runs, err := store.QueryMonitorRuns(ctx, "job_retention", 10)
	if err != nil || len(runs) != 1 || runs[0].RunID != "run_new" {
		t.Fatalf("expected only run_new to remain, got %+v", runs)
	}
}

func TestSQLite_Migration_HistoricalContinuity(t *testing.T) {
	// B-01: Verify that upgrading from PR#3 preserves legacy node_key
	// and that querying with (NodeIdentityKey + LegacyNodeKey) bridges PR3 and PR4 samples continuously.
	tmpDir := t.TempDir()
	ctx := context.Background()
	dbPath := filepath.Join(tmpDir, "history.db")

	// 1. Manually setup PR#3 schema without node_identity_key or config_revision_key
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	pr3DDL := `
	CREATE TABLE monitor_samples (
		sample_id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		node_key TEXT NOT NULL,
		profile_id TEXT NOT NULL,
		display_name_snapshot TEXT NOT NULL,
		probe_type TEXT NOT NULL,
		target TEXT NOT NULL,
		timestamp DATETIME NOT NULL,
		success INTEGER NOT NULL,
		latency_ms INTEGER NOT NULL DEFAULT 0,
		ttfb_ms INTEGER NOT NULL DEFAULT 0,
		error_class TEXT NOT NULL DEFAULT 'none',
		error_detail TEXT,
		exit_ip TEXT,
		exit_region TEXT,
		metadata_json TEXT
	);
	CREATE TABLE monitor_runs (
		run_id TEXT PRIMARY KEY,
		job_id TEXT NOT NULL,
		scheduled_at DATETIME NOT NULL,
		started_at DATETIME NOT NULL,
		finished_at DATETIME,
		status TEXT NOT NULL,
		total_nodes INTEGER NOT NULL DEFAULT 0,
		success_nodes INTEGER NOT NULL DEFAULT 0,
		failed_nodes INTEGER NOT NULL DEFAULT 0,
		error_message TEXT
	);
	`
	if _, err := rawDB.Exec(pr3DDL); err != nil {
		t.Fatalf("exec pr3 ddl: %v", err)
	}

	// Insert PR#3 legacy sample
	tLegacy := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	_, err = rawDB.Exec(`
		INSERT INTO monitor_samples (
			sample_id, run_id, node_key, profile_id, display_name_snapshot,
			probe_type, target, timestamp, success, latency_ms, ttfb_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "s_pr3_legacy", "run_pr3", "nk_legacy_test", "prof_1", "HK-01", "rtt", "https://example.com", tLegacy, 1, 45, 35)
	if err != nil {
		t.Fatalf("insert legacy sample: %v", err)
	}
	_ = rawDB.Close()

	// 2. Open DB with NewStore, which runs migrateSchema
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore with migration failed: %v", err)
	}
	defer store.Close()

	// 3. Insert PR#4 new sample with new NodeIdentityKey
	tPR4 := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	newIdentityKey := "nid_ss_1.2.3.4_8388_a1b2c3d4"
	err = store.SaveMonitorSamples(ctx, []*monitor.MonitorSample{
		{
			SampleID:            "s_pr4_new",
			RunID:               "run_pr4",
			NodeKey:             "nk_legacy_test",
			NodeIdentityKey:     newIdentityKey,
			ConfigRevisionKey:   "rev_0123456789abcdef",
			ProfileID:           "prof_1",
			DisplayNameSnapshot: "HK-01",
			ProbeType:           "rtt",
			Target:              "https://example.com",
			Timestamp:           tPR4,
			Success:             true,
			Latency:             55 * time.Millisecond,
			TTFB:                40 * time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("save pr4 sample: %v", err)
	}

	// 4. Query with (NodeIdentityKey + LegacyNodeKey) bridge
	page, err := store.QueryMonitorSamplesCursor(ctx, monitor.CursorFilter{
		NodeIdentityKey: newIdentityKey,
		LegacyNodeKey:   "nk_legacy_test",
		OrderDesc:       true,
		Limit:           10,
	})
	if err != nil {
		t.Fatalf("QueryMonitorSamplesCursor bridge failed: %v", err)
	}

	if len(page.Items) != 2 {
		t.Fatalf("expected both PR3 old sample and PR4 new sample to be returned, got %d items", len(page.Items))
	}
	if page.Items[0].SampleID != "s_pr4_new" || page.Items[1].SampleID != "s_pr3_legacy" {
		t.Errorf("unexpected continuous item sequence: %s, %s", page.Items[0].SampleID, page.Items[1].SampleID)
	}

	// 5. GetDerivedStats with (NodeIdentityKey + LegacyNodeKey) bridge
	stats, err := store.GetDerivedStats(ctx, monitor.StatsQuery{
		NodeIdentityKey: newIdentityKey,
		LegacyNodeKey:   "nk_legacy_test",
	})
	if err != nil {
		t.Fatalf("GetDerivedStats bridge failed: %v", err)
	}
	if stats.SampleCount != 2 || stats.SuccessCount != 2 {
		t.Errorf("expected 2 continuous samples in stats, got %+v", stats)
	}
}

func TestSQLite_DerivedStats_ExactQuantiles_BoundedMemory(t *testing.T) {
	// B-02: Verify exact SQL rank quantiles, non-HTTP TTFB <= 0 exclusion, and safety boundaries.
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	tBase := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	// Insert 10 successful samples with latencies 10..100ms
	// TTFB: only 5 of them have valid ttfb_ms > 0 (20, 40, 60, 80, 100). The other 5 have ttfb_ms = 0.
	var samples []*monitor.MonitorSample
	for i := 1; i <= 10; i++ {
		ttfbVal := time.Duration(0)
		if i%2 == 0 {
			ttfbVal = time.Duration(i*10) * time.Millisecond
		}
		samples = append(samples, &monitor.MonitorSample{
			SampleID:            fmt.Sprintf("stat_s_%02d", i),
			RunID:               "run_stats",
			NodeKey:             "nk_stats_test",
			NodeIdentityKey:     "nid_stats_test",
			ProfileID:           "prof_stats",
			ProbeType:           "http",
			Target:              "https://target.test",
			Timestamp:           tBase.Add(time.Duration(i) * time.Minute),
			Success:             true,
			Latency:             time.Duration(i*10) * time.Millisecond,
			TTFB:                ttfbVal,
		})
	}
	// Add 1 failed sample (latency=0, success=false)
	samples = append(samples, &monitor.MonitorSample{
		SampleID:        "stat_fail_01",
		RunID:           "run_stats",
		NodeKey:         "nk_stats_test",
		NodeIdentityKey: "nid_stats_test",
		ProfileID:       "prof_stats",
		ProbeType:       "http",
		Target:          "https://target.test",
		Timestamp:       tBase.Add(11 * time.Minute),
		Success:         false,
		Latency:         0,
		TTFB:            0,
		ErrorClass:      "dns_failure",
	})

	if err := store.SaveMonitorSamples(ctx, samples); err != nil {
		t.Fatalf("save samples: %v", err)
	}

	stats, err := store.GetDerivedStats(ctx, monitor.StatsQuery{
		NodeIdentityKey: "nid_stats_test",
	})
	if err != nil {
		t.Fatalf("GetDerivedStats failed: %v", err)
	}

	if stats.SampleCount != 11 || stats.SuccessCount != 10 || stats.FailureCount != 1 {
		t.Fatalf("unexpected summary counts: %+v", stats)
	}

	// Latency percentiles: 10 valid samples (10..100)
	// rank50 = ceil(0.50*10)-1 = 4 -> 50ms
	// rank95 = ceil(0.95*10)-1 = 9 -> 100ms
	if *stats.LatencyMinMs != 10 || *stats.LatencyMaxMs != 100 {
		t.Errorf("latency min/max mismatch: min=%v max=%v", *stats.LatencyMinMs, *stats.LatencyMaxMs)
	}
	if *stats.LatencyP50Ms != 50 {
		t.Errorf("expected LatencyP50Ms=50, got %v", *stats.LatencyP50Ms)
	}
	if *stats.LatencyP95Ms != 100 {
		t.Errorf("expected LatencyP95Ms=100, got %v", *stats.LatencyP95Ms)
	}

	// TTFB percentiles: exactly 5 valid samples (20, 40, 60, 80, 100). Samples with ttfb_ms=0 are excluded!
	// rank50 = ceil(0.50*5)-1 = 2 -> 60ms
	// rank95 = ceil(0.95*5)-1 = 4 -> 100ms
	if stats.TTFBP50Ms == nil || *stats.TTFBP50Ms != 60 {
		t.Errorf("expected TTFBP50Ms=60 (excluding 0ms), got %v", stats.TTFBP50Ms)
	}
	if stats.TTFBP95Ms == nil || *stats.TTFBP95Ms != 100 {
		t.Errorf("expected TTFBP95Ms=100 (excluding 0ms), got %v", stats.TTFBP95Ms)
	}

	// Safety boundaries: Since > Until
	since := tBase.Add(10 * time.Minute)
	until := tBase.Add(5 * time.Minute)
	_, err = store.GetDerivedStats(ctx, monitor.StatsQuery{
		Since: &since,
		Until: &until,
	})
	if err == nil || !monitor.IsValidationError(err) || !errors.Is(err, monitor.ErrInvalidTimeRange) {
		t.Errorf("expected ErrInvalidTimeRange validation error, got %v", err)
	}

	// Safety boundaries: Window width > 366 days
	wideSince := tBase
	wideUntil := tBase.AddDate(2, 0, 0)
	_, err = store.GetDerivedStats(ctx, monitor.StatsQuery{
		Since: &wideSince,
		Until: &wideUntil,
	})
	if err == nil || !monitor.IsValidationError(err) || !errors.Is(err, monitor.ErrQueryWindowTooLarge) {
		t.Errorf("expected ErrQueryWindowTooLarge for 2-year window, got %v", err)
	}
}

func TestSQLite_Retention_SafetyGuards(t *testing.T) {
	// B-03: Verify future cutoff rejection, custom_days hard max, partial_failed orphan cleanup,
	// strict preservation of running and skipped runs, and exact sample cutoff boundary.
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	// 1. Future cutoff rejects without deleting anything
	future := now.Add(24 * time.Hour)
	_, err = store.ApplyRetention(ctx, monitor.RetentionRequest{
		Policy:     monitor.RetentionCustom,
		CutoffTime: &future,
	})
	if err == nil || !monitor.IsValidationError(err) || !errors.Is(err, monitor.ErrFutureCutoff) {
		t.Errorf("expected ErrFutureCutoff for future cutoff time, got %v", err)
	}

	// 2. Invalid custom_days
	_, err = store.ApplyRetention(ctx, monitor.RetentionRequest{
		Policy:     monitor.RetentionCustom,
		CustomDays: 0,
	})
	if err == nil || !monitor.IsValidationError(err) || !errors.Is(err, monitor.ErrInvalidCustomDays) {
		t.Errorf("expected ErrInvalidCustomDays for custom_days=0, got %v", err)
	}
	_, err = store.ApplyRetention(ctx, monitor.RetentionRequest{
		Policy:     monitor.RetentionCustom,
		CustomDays: 40000, // exceeds hard max 36500
	})
	if err == nil || !monitor.IsValidationError(err) || !errors.Is(err, monitor.ErrInvalidCustomDays) {
		t.Errorf("expected ErrInvalidCustomDays for custom_days=40000, got %v", err)
	}

	// 3. Run retention semantics:
	// Insert 4 runs older than cutoff (60 days ago):
	// - run_running (status: running) -> MUST BE PRESERVED
	// - run_skipped (status: skipped) -> MUST BE PRESERVED
	// - run_partial_failed (status: partial_failed) -> MUST BE CLEANED UP
	// - run_completed (status: completed) -> MUST BE CLEANED UP
	tPast := now.AddDate(0, 0, -60)
	runs := []*monitor.MonitorRun{
		{RunID: "run_running", JobID: "j1", ScheduledAt: tPast, StartedAt: tPast, Status: monitor.RunStatusRunning},
		{RunID: "run_skipped", JobID: "j1", ScheduledAt: tPast, StartedAt: tPast, Status: monitor.RunStatusSkipped},
		{RunID: "run_partial_failed", JobID: "j1", ScheduledAt: tPast, StartedAt: tPast, Status: monitor.RunStatusPartialFailed},
		{RunID: "run_completed", JobID: "j1", ScheduledAt: tPast, StartedAt: tPast, Status: monitor.RunStatusCompleted},
	}
	for _, r := range runs {
		if err := store.SaveMonitorRun(ctx, r); err != nil {
			t.Fatalf("save run %s: %v", r.RunID, err)
		}
	}

	// Sample boundary test:
	// cutoff is 30 days ago.
	// sample_before is at cutoff - 10s -> deleted.
	// sample_at is at cutoff + 10s -> kept.
	cutoff := now.AddDate(0, 0, -30)
	err = store.SaveMonitorSamples(ctx, []*monitor.MonitorSample{
		{
			SampleID:  "s_before_cutoff",
			RunID:     "run_other",
			NodeKey:   "nk_1",
			Timestamp: cutoff.Add(-10 * time.Second),
			Success:   true,
		},
		{
			SampleID:  "s_after_cutoff",
			RunID:     "run_other",
			NodeKey:   "nk_1",
			Timestamp: cutoff.Add(10 * time.Second),
			Success:   true,
		},
	})
	if err != nil {
		t.Fatalf("save boundary samples: %v", err)
	}

	// Apply 30d retention
	res, err := store.ApplyRetention(ctx, monitor.RetentionRequest{
		Policy:     monitor.Retention30d,
		CutoffTime: &cutoff,
	})
	if err != nil {
		t.Fatalf("ApplyRetention failed: %v", err)
	}

	if res.SamplesDeleted != 1 {
		t.Errorf("expected exactly 1 sample deleted (s_before_cutoff), got %d", res.SamplesDeleted)
	}
	if res.RunsDeleted != 2 {
		t.Errorf("expected exactly 2 runs deleted (completed and partial_failed), got %d", res.RunsDeleted)
	}

	// Verify runs status: running and skipped must exist
	allRuns, err := store.QueryMonitorRuns(ctx, "j1", 10)
	if err != nil {
		t.Fatalf("query runs: %v", err)
	}
	runMap := make(map[string]monitor.RunStatus)
	for _, r := range allRuns {
		runMap[r.RunID] = r.Status
	}

	if _, ok := runMap["run_running"]; !ok {
		t.Errorf("run_running was erroneously deleted!")
	}
	if _, ok := runMap["run_skipped"]; !ok {
		t.Errorf("run_skipped was erroneously deleted!")
	}
	if _, ok := runMap["run_partial_failed"]; ok {
		t.Errorf("run_partial_failed was not cleaned up!")
	}
	if _, ok := runMap["run_completed"]; ok {
		t.Errorf("run_completed was not cleaned up!")
	}

	// Verify remaining sample is s_after_cutoff
	remainingSamples, err := store.QueryMonitorSamples(ctx, monitor.SampleFilter{})
	if err != nil || len(remainingSamples) != 1 || remainingSamples[0].SampleID != "s_after_cutoff" {
		t.Fatalf("expected only s_after_cutoff to remain, got %+v", remainingSamples)
	}
}

func TestSQLite_Cursor_DirectionAndValidation(t *testing.T) {
	// B-04: Verify direction mismatch rejection, token size limit, version check, Since > Until, and limit bounds.
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	// 1. Direction mismatch: ASC cursor reused in DESC query
	ascCursor := EncodeCursor(now, "s_01", "asc")
	_, err = store.QueryMonitorSamplesCursor(ctx, monitor.CursorFilter{
		Cursor:    ascCursor,
		OrderDesc: true, // Requested DESC, but cursor is ASC
	})
	if err == nil || !monitor.IsValidationError(err) || !errors.Is(err, monitor.ErrInvalidCursor) {
		t.Errorf("expected ErrInvalidCursor for ASC cursor in DESC query, got %v", err)
	}

	// 2. Direction mismatch: DESC cursor reused in ASC query
	descCursor := EncodeCursor(now, "s_01", "desc")
	_, err = store.QueryMonitorSamplesCursor(ctx, monitor.CursorFilter{
		Cursor:    descCursor,
		OrderDesc: false, // Requested ASC, but cursor is DESC
	})
	if err == nil || !monitor.IsValidationError(err) || !errors.Is(err, monitor.ErrInvalidCursor) {
		t.Errorf("expected ErrInvalidCursor for DESC cursor in ASC query, got %v", err)
	}

	// 3. Oversized cursor (> 512 bytes)
	longToken := strings.Repeat("A", 600)
	_, err = store.QueryMonitorSamplesCursor(ctx, monitor.CursorFilter{
		Cursor: longToken,
	})
	if err == nil || !monitor.IsValidationError(err) || !errors.Is(err, monitor.ErrInvalidCursor) {
		t.Errorf("expected ErrInvalidCursor for oversized cursor, got %v", err)
	}

	// 4. Malformed cursor
	_, err = store.QueryMonitorSamplesCursor(ctx, monitor.CursorFilter{
		Cursor: "!!!not_valid_base64???",
	})
	if err == nil || !monitor.IsValidationError(err) || !errors.Is(err, monitor.ErrInvalidCursor) {
		t.Errorf("expected ErrInvalidCursor for malformed cursor, got %v", err)
	}

	// 5. Invalid Time range: Since > Until
	since := now.Add(1 * time.Hour)
	until := now
	_, err = store.QueryMonitorSamplesCursor(ctx, monitor.CursorFilter{
		Since: &since,
		Until: &until,
	})
	if err == nil || !monitor.IsValidationError(err) || !errors.Is(err, monitor.ErrInvalidTimeRange) {
		t.Errorf("expected ErrInvalidTimeRange, got %v", err)
	}

	// 6. Invalid Limit bounds
	_, err = store.QueryMonitorSamplesCursor(ctx, monitor.CursorFilter{
		Limit: -1,
	})
	if err == nil || !monitor.IsValidationError(err) || !errors.Is(err, monitor.ErrInvalidLimit) {
		t.Errorf("expected ErrInvalidLimit for Limit < 0, got %v", err)
	}
	_, err = store.QueryMonitorSamplesCursor(ctx, monitor.CursorFilter{
		Limit: 2000,
	})
	if err == nil || !monitor.IsValidationError(err) || !errors.Is(err, monitor.ErrInvalidLimit) {
		t.Errorf("expected ErrInvalidLimit for Limit > 1000, got %v", err)
	}
}

func TestSQLite_ProfileIsolation_SameIdentity(t *testing.T) {
	// R-01: Verify that two subscriptions with the same NodeIdentityKey are strictly isolated
	// when queried with ProfileID.
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	sharedIdentity := "nid_shared_endpoint_443"

	// Insert sample from Profile A
	_ = store.SaveMonitorSamples(ctx, []*monitor.MonitorSample{
		{
			SampleID:        "s_prof_a",
			RunID:           "run_a",
			NodeKey:         "nk_node_a",
			NodeIdentityKey: sharedIdentity,
			ProfileID:       "profile_A",
			Timestamp:       now.Add(-10 * time.Minute),
			Success:         true,
			Latency:         40 * time.Millisecond,
		},
		{
			SampleID:        "s_prof_b",
			RunID:           "run_b",
			NodeKey:         "nk_node_b",
			NodeIdentityKey: sharedIdentity,
			ProfileID:       "profile_B",
			Timestamp:       now.Add(-5 * time.Minute),
			Success:         true,
			Latency:         70 * time.Millisecond,
		},
	})

	// Query Profile A
	pageA, err := store.QueryMonitorSamplesCursor(ctx, monitor.CursorFilter{
		NodeIdentityKey: sharedIdentity,
		ProfileID:       "profile_A",
	})
	if err != nil {
		t.Fatalf("query profile A failed: %v", err)
	}
	if len(pageA.Items) != 1 || pageA.Items[0].SampleID != "s_prof_a" {
		t.Errorf("expected only s_prof_a, got %+v", pageA.Items)
	}

	// Query Stats Profile B
	statsB, err := store.GetDerivedStats(ctx, monitor.StatsQuery{
		NodeIdentityKey: sharedIdentity,
		ProfileID:       "profile_B",
	})
	if err != nil {
		t.Fatalf("stats profile B failed: %v", err)
	}
	if statsB.SampleCount != 1 || *statsB.LatencyP50Ms != 70 {
		t.Errorf("expected isolated stats for Profile B, got %+v", statsB)
	}
}

