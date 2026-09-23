package history

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
	_ "modernc.org/sqlite"
)

func TestMonitorSamplingSourceFiltersStatsPagingAndRunAssociation(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	now := time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC)
	job := &monitor.MonitorJobDefinition{
		ID: "tiered-job", Name: "Tiered", ProfileID: "profile-a", ProbeSet: monitor.ProbeSetLight,
		SamplingTier: monitor.SamplingTierRegular, Interval: time.Minute, Timeout: 5 * time.Second,
		CreatedAt: now, UpdatedAt: now, DefinitionVersion: monitor.MonitorJobDefinitionVersion,
	}
	if err := store.SaveMonitorJobDefinition(ctx, job); err != nil {
		t.Fatalf("save job definition: %v", err)
	}

	for _, run := range []*monitor.MonitorRun{
		{RunID: "scheduled-run", JobID: job.ID, SamplingTier: monitor.SamplingTierRegular, TriggerType: monitor.SamplingTriggerScheduled, SamplingStrategyVersion: monitor.SamplingStrategyVersion, ScheduledAt: now, StartedAt: now, Status: monitor.RunStatusCompleted, TotalNodes: 1, SuccessNodes: 1},
		{RunID: "diagnostic-run", JobID: job.ID, SamplingTier: monitor.SamplingTierDiagnostic, TriggerType: monitor.SamplingTriggerManual, SamplingStrategyVersion: monitor.SamplingStrategyVersion, ScheduledAt: now, StartedAt: now, Status: monitor.RunStatusFailed, TotalNodes: 1, FailedNodes: 1},
	} {
		if err := store.SaveMonitorRun(ctx, run); err != nil {
			t.Fatalf("save run %s: %v", run.RunID, err)
		}
	}
	// This row represents an old run whose source cannot be recovered from its job.
	if _, err := store.db.db.ExecContext(ctx, `
		INSERT INTO monitor_runs (run_id, job_id, scheduled_at, started_at, status, sampling_tier, trigger_type, sampling_strategy_version)
		VALUES ('legacy-run', ?, ?, ?, 'completed', 'legacy_unknown', 'legacy_unknown', 0)
	`, job.ID, now, now); err != nil {
		t.Fatalf("insert legacy unknown run: %v", err)
	}

	samples := []*monitor.MonitorSample{
		{SampleID: "regular-ok", RunID: "scheduled-run", NodeKey: "node-a", NodeIdentityKey: "identity-a", ConfigRevisionKey: "rev-a", ProfileID: "profile-a", DisplayNameSnapshot: "A", ProbeType: "rtt", Target: "known", Timestamp: now, Success: true, Latency: 40 * time.Millisecond, ErrorClass: "none"},
		{SampleID: "diagnostic-fail", RunID: "diagnostic-run", NodeKey: "node-a", NodeIdentityKey: "identity-a", ConfigRevisionKey: "rev-a", ProfileID: "profile-a", DisplayNameSnapshot: "A", ProbeType: "rtt", Target: "known", Timestamp: now, Success: false, ErrorClass: "timeout"},
		{SampleID: "legacy-ok", RunID: "legacy-run", NodeKey: "node-a", NodeIdentityKey: "identity-a", ConfigRevisionKey: "rev-a", ProfileID: "profile-a", DisplayNameSnapshot: "A", ProbeType: "rtt", Target: "known", Timestamp: now, Success: true, Latency: 25 * time.Millisecond, ErrorClass: "none"},
	}
	if err := store.SaveMonitorSamples(ctx, samples); err != nil {
		t.Fatalf("save raw samples: %v", err)
	}

	// Updating today's job tier cannot rewrite the immutable source of earlier runs.
	if _, err := store.db.db.ExecContext(ctx, `UPDATE monitor_job_definitions SET sampling_tier = 'sparse' WHERE job_id = ?`, job.ID); err != nil {
		t.Fatalf("change current job tier: %v", err)
	}
	all, err := store.QueryMonitorSamples(ctx, monitor.SampleFilter{NodeIdentityKey: "identity-a", Limit: 10})
	if err != nil || len(all) != 3 {
		t.Fatalf("all raw samples must remain visible: count=%d err=%v", len(all), err)
	}
	sourceByID := make(map[string]monitor.SamplingTier, len(all))
	for _, sample := range all {
		sourceByID[sample.SampleID] = sample.SamplingTier
	}
	if sourceByID["regular-ok"] != monitor.SamplingTierRegular || sourceByID["diagnostic-fail"] != monitor.SamplingTierDiagnostic || sourceByID["legacy-ok"] != monitor.SamplingTierLegacyUnknown {
		t.Fatalf("run-associated source was not preserved after job change: %+v", sourceByID)
	}

	regularPage, err := store.QueryMonitorSamplesCursor(ctx, monitor.CursorFilter{
		NodeIdentityKey: "identity-a", SamplingTier: "", RegularObservationOnly: true, Limit: 10, OrderDesc: true,
	})
	if err != nil || len(regularPage.Items) != 1 || regularPage.Items[0].SampleID != "regular-ok" {
		t.Fatalf("cursor regular-observation filter: page=%+v err=%v", regularPage, err)
	}
	regularStats, err := store.GetDerivedStats(ctx, monitor.StatsQuery{NodeIdentityKey: "identity-a", Since: &now, Until: &now, RegularObservationOnly: true})
	if err != nil {
		t.Fatalf("regular-observation stats: %v", err)
	}
	if regularStats.SampleCount != 1 || regularStats.SuccessCount != 1 || regularStats.FailureCount != 0 || regularStats.SuccessRate != 1 || !regularStats.RegularObservationOnly || len(regularStats.IncludedSamplingTiers) != 1 || regularStats.IncludedSamplingTiers[0] != monitor.SamplingTierRegular {
		t.Fatalf("diagnostic/unknown sources entered regular stats: %+v", regularStats)
	}
	diagnosticStats, err := store.GetDerivedStats(ctx, monitor.StatsQuery{NodeIdentityKey: "identity-a", SamplingTier: monitor.SamplingTierDiagnostic})
	if err != nil || diagnosticStats.SampleCount != 1 || diagnosticStats.FailureCount != 1 {
		t.Fatalf("diagnostic stats must remain separately queryable: stats=%+v err=%v", diagnosticStats, err)
	}
	legacy, err := store.QueryMonitorSamples(ctx, monitor.SampleFilter{SamplingTier: monitor.SamplingTierLegacyUnknown, Limit: 10})
	if err != nil || len(legacy) != 1 {
		t.Fatalf("legacy unknown raw history must remain queryable: count=%d err=%v", len(legacy), err)
	}
}

func TestMonitorSchemaV2UpgradePreservesHistoryBudgetAndDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(ddlSchema); err != nil {
		t.Fatalf("create v2 base schema: %v", err)
	}
	if _, err := raw.Exec(monitorJobDDL); err != nil {
		t.Fatalf("create v2 job schema: %v", err)
	}
	if _, err := raw.Exec(schemaMetaDDL + ` INSERT INTO schema_meta (singleton, schema_version) VALUES (1, 2);`); err != nil {
		t.Fatalf("mark v2 schema: %v", err)
	}
	if _, err := raw.Exec(monitorBudgetDDL); err != nil {
		t.Fatalf("create v2 budget ledger: %v", err)
	}
	if _, err := raw.Exec(`UPDATE monitor_budget_usage SET utc_day='2026-09-23', requests_used=7, bytes_used=1234 WHERE singleton=1`); err != nil {
		t.Fatalf("seed budget usage: %v", err)
	}
	now := time.Date(2026, 9, 22, 8, 0, 0, 0, time.UTC)
	nowText := now.Format(time.RFC3339Nano)
	if _, err := raw.Exec(`INSERT INTO monitor_job_definitions (job_id, name, profile_id, probe_set, interval_ns, timeout_ns, created_at, updated_at, definition_version) VALUES ('old-job', 'Old', 'profile-a', 'light', ?, ?, ?, ?, 1)`, time.Minute.Nanoseconds(), (5 * time.Second).Nanoseconds(), nowText, nowText); err != nil {
		t.Fatalf("seed old job definition: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO monitor_runs (run_id, job_id, scheduled_at, started_at, finished_at, status, total_nodes, success_nodes, failed_nodes, error_message) VALUES ('old-run', 'old-job', ?, ?, ?, 'completed', 1, 1, 0, NULL)`, nowText, nowText, nowText); err != nil {
		t.Fatalf("seed old run: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO monitor_samples (sample_id, run_id, node_key, node_identity_key, config_revision_key, profile_id, display_name_snapshot, probe_type, target, timestamp, success, latency_ms, ttfb_ms, error_class) VALUES ('old-sample', 'old-run', 'node-a', 'identity-a', 'rev-a', 'profile-a', 'A', 'rtt', 'known', ?, 1, 40, 0, 'none')`, nowText); err != nil {
		t.Fatalf("seed v2 records: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	upgraded, err := OpenDB(dir)
	if err != nil {
		t.Fatalf("upgrade v2 database: %v", err)
	}
	defer upgraded.Close()
	var version int
	if err := upgraded.db.QueryRow("SELECT schema_version FROM schema_meta WHERE singleton=1").Scan(&version); err != nil || version != CurrentSchemaVersion {
		t.Fatalf("schema version after upgrade = %d, err=%v", version, err)
	}
	usage, err := upgraded.MonitorBudgetUsage(context.Background(), "2026-09-23")
	if err != nil || usage.RequestsUsed != 7 || usage.BytesUsed != 1234 {
		t.Fatalf("P1-11A budget usage changed during migration: %+v err=%v", usage, err)
	}
	runs, err := upgraded.QueryMonitorRuns(context.Background(), "old-job", 10)
	if err != nil || len(runs) != 1 || runs[0].SamplingTier != monitor.SamplingTierLegacyUnknown || runs[0].TriggerType != monitor.SamplingTriggerLegacyUnknown || runs[0].SamplingStrategyVersion != 0 || runs[0].Status != monitor.RunStatusCompleted {
		t.Fatalf("old run/source did not survive as unknown: runs=%+v err=%v", runs, err)
	}
	samples, err := upgraded.QueryMonitorSamples(context.Background(), monitor.SampleFilter{RunID: "old-run"})
	if err != nil || len(samples) != 1 || samples[0].SamplingTier != monitor.SamplingTierLegacyUnknown || samples[0].NodeIdentityKey != "identity-a" || samples[0].ConfigRevisionKey != "rev-a" {
		t.Fatalf("old raw sample changed or disappeared: samples=%+v err=%v", samples, err)
	}
	definitions, err := upgraded.ListMonitorJobDefinitions(context.Background())
	if err != nil || len(definitions) != 1 || definitions[0].SamplingTier != monitor.SamplingTierRegular || definitions[0].Interval != time.Minute || definitions[0].ProbeSet != monitor.ProbeSetLight ||
		definitions[0].ResumeOnLaunch || definitions[0].DesiredState != monitor.JobStateStopped || definitions[0].DefinitionVersion != monitor.MonitorJobDefinitionVersion {
		t.Fatalf("old job must stay a compatible regular task: definitions=%+v err=%v", definitions, err)
	}
}
