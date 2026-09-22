package appdata

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	_ "modernc.org/sqlite"
)

func TestMigrateLegacyDataPreservesWALHistoryAndPublishesOneAuthority(t *testing.T) {
	paths := newMigrationTestPaths(t)
	legacyHistory := paths.legacyHistoryDir()
	if err := os.MkdirAll(legacyHistory, 0o755); err != nil {
		t.Fatal(err)
	}

	legacyJSON := []byte("{\"id\":\"legacy-json\",\"airport_id\":\"profile-a\"}\n")
	legacyJSONPath := filepath.Join(legacyHistory, "legacy-json.json")
	if err := os.WriteFile(legacyJSONPath, legacyJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	settings := []byte("{\n  \"preferred_browser\": \"default\",\n  \"future_setting\": 17\n}\n")
	if err := os.WriteFile(paths.legacySettingsFile(), settings, 0o600); err != nil {
		t.Fatal(err)
	}

	source, err := history.NewStore(legacyHistory)
	if err != nil {
		t.Fatal(err)
	}
	runID := "run-wal-001"
	jobID := "job-persisted-001"
	finished := time.Date(2026, 9, 22, 1, 2, 3, 0, time.UTC)
	if err := source.SaveMonitorRun(context.Background(), &monitor.MonitorRun{
		RunID:        runID,
		JobID:        jobID,
		ScheduledAt:  finished.Add(-time.Minute),
		StartedAt:    finished.Add(-50 * time.Second),
		FinishedAt:   &finished,
		Status:       monitor.RunStatusCompleted,
		TotalNodes:   1,
		SuccessNodes: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := source.SaveMonitorSamples(context.Background(), []*monitor.MonitorSample{{
		SampleID:            "sample-wal-001",
		RunID:               runID,
		NodeKey:             "node-a",
		NodeIdentityKey:     "identity-a",
		ConfigRevisionKey:   "revision-a",
		ProfileID:           "profile-a",
		DisplayNameSnapshot: "same-name",
		ProbeType:           "rtt",
		Target:              "fixture",
		Timestamp:           finished,
		Success:             true,
		Latency:             21 * time.Millisecond,
		TTFB:                18 * time.Millisecond,
		ErrorClass:          "none",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := source.SaveLatencyTest(context.Background(), &history.LatencyTest{
		AttemptID:         "attempt-wal-001",
		ProfileID:         "profile-a",
		NodeKey:           "node-a",
		NodeIdentityKey:   "identity-a",
		ConfigRevisionKey: "revision-a",
		DisplayName:       "same-name",
		NodeType:          "ss",
		TestProject:       "latency",
		RequestedAt:       finished.Add(-2 * time.Second),
		StartedAt:         finished.Add(-time.Second),
		FinishedAt:        finished,
		Status:            "completed",
		LatencyMs:         21,
		TotalSamples:      1,
		SuccessSamples:    1,
		Samples:           []history.LatencyTestSample{{Seq: 1, Timestamp: finished, LatencyMs: 21, Success: true}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := source.SaveMonitorJobDefinition(context.Background(), &monitor.MonitorJobDefinition{
		ID:                jobID,
		Name:              "fixture job",
		ProfileID:         "profile-a",
		Nodes:             []monitor.MonitorJobNodeReference{{NodeKey: "node-a", NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a", DisplayName: "same-name", Type: "ss"}},
		ProbeSet:          monitor.ProbeSetLight,
		Interval:          time.Minute,
		Timeout:           10 * time.Second,
		CreatedAt:         finished.Add(-time.Hour),
		UpdatedAt:         finished,
		DefinitionVersion: monitor.MonitorJobDefinitionVersion,
	}); err != nil {
		t.Fatal(err)
	}

	walPath := filepath.Join(legacyHistory, "history.db-wal")
	if _, err := os.Stat(walPath); err != nil {
		t.Fatalf("fixture did not leave a committed WAL state: %v", err)
	}

	if err := paths.MigrateLegacyData(context.Background()); err != nil {
		_ = source.Close()
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(paths.HistoryDir, "history.db")); err != nil {
		t.Fatalf("canonical database missing: %v", err)
	}
	if _, err := os.Stat(legacyJSONPath); err != nil {
		t.Fatalf("legacy source was not retained: %v", err)
	}
	gotSettings, err := os.ReadFile(paths.SettingsFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotSettings) != string(settings) {
		t.Fatalf("settings bytes changed during migration: %q", gotSettings)
	}
	gotJSON, err := os.ReadFile(filepath.Join(paths.HistoryDir, "legacy", "legacy-json.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotJSON) != string(legacyJSON) {
		t.Fatalf("legacy JSON bytes changed during migration: %q", gotJSON)
	}

	target, err := history.NewStoreWithLegacyDir(paths.HistoryDir, filepath.Join(paths.HistoryDir, "legacy"))
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	runs, err := target.QueryMonitorRuns(context.Background(), jobID, 10)
	if err != nil || len(runs) != 1 || runs[0].RunID != runID {
		t.Fatalf("monitor history did not survive online backup: runs=%+v err=%v", runs, err)
	}
	samples, err := target.QueryMonitorSamples(context.Background(), monitor.SampleFilter{ProfileID: "profile-a", NodeKey: "node-a"})
	if err != nil || len(samples) != 1 || samples[0].SampleID != "sample-wal-001" {
		t.Fatalf("monitor sample did not survive online backup: samples=%+v err=%v", samples, err)
	}
	latency, err := target.QueryLatencyTests(context.Background(), history.LatencyTestFilter{ProfileID: "profile-a", NodeKey: "node-a"})
	if err != nil || len(latency.Tests) != 1 || latency.Tests[0].AttemptID != "attempt-wal-001" {
		t.Fatalf("workbench history did not survive online backup: tests=%+v err=%v", latency.Tests, err)
	}
	definitions, err := target.ListMonitorJobDefinitions(context.Background())
	if err != nil || len(definitions) != 1 || definitions[0].ID != jobID {
		t.Fatalf("persisted job did not survive online backup: definitions=%+v err=%v", definitions, err)
	}

	if _, err := target.Save(&history.TestRun{ID: "new-canonical-json", CreatedAt: finished.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	sourceJSONEntries, err := os.ReadDir(legacyHistory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range sourceJSONEntries {
		if entry.Name() == "new-canonical-json.json" {
			t.Fatal("new JSON write leaked back to the legacy source")
		}
	}
	if _, err := os.Stat(filepath.Join(paths.HistoryDir, "legacy", "new-canonical-json.json")); err != nil {
		t.Fatalf("new JSON write did not use canonical legacy authority: %v", err)
	}
}

func TestMigrateLegacyDataRejectsExistingCanonicalArtifacts(t *testing.T) {
	paths := newMigrationTestPaths(t)
	legacyHistory := paths.legacyHistoryDir()
	if err := os.MkdirAll(legacyHistory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyHistory, "legacy.json"), []byte("legacy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.DataRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	canonicalSettings := []byte("{\"preferred_browser\":\"default\"}\n")
	if err := os.WriteFile(paths.SettingsFile, canonicalSettings, 0o600); err != nil {
		t.Fatal(err)
	}

	info := paths.InspectMigration()
	if info.State != MigrationStateConflict {
		t.Fatalf("expected conflict, got %+v", info)
	}
	if err := paths.MigrateLegacyData(context.Background()); !errors.Is(err, ErrDataMigrationConflict) {
		t.Fatalf("expected conflict error, got %v", err)
	}
	got, err := os.ReadFile(paths.SettingsFile)
	if err != nil || string(got) != string(canonicalSettings) {
		t.Fatalf("canonical settings changed on conflict: %q err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Join(paths.HistoryDir, "history.db")); !os.IsNotExist(err) {
		t.Fatalf("conflict unexpectedly created canonical DB: %v", err)
	}
}

func TestMigrateLegacyDataFailureKeepsSourceAuthority(t *testing.T) {
	paths := newMigrationTestPaths(t)
	legacyHistory := paths.legacyHistoryDir()
	source, err := history.NewStore(legacyHistory)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.SaveMonitorRun(context.Background(), &monitor.MonitorRun{
		RunID:       "source-only",
		JobID:       "job-source-only",
		ScheduledAt: time.Now().UTC(),
		StartedAt:   time.Now().UTC(),
		Status:      monitor.RunStatusCompleted,
	}); err != nil {
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	backupCalled := false
	err = paths.migrateLegacyData(context.Background(), migrationHooks{backup: func(context.Context, string, string) error {
		backupCalled = true
		return errors.New("fixture backup failure")
	}})
	if err == nil || !backupCalled {
		t.Fatalf("expected injected backup failure, called=%v err=%v", backupCalled, err)
	}
	if _, statErr := os.Stat(filepath.Join(paths.HistoryDir, "history.db")); !os.IsNotExist(statErr) {
		t.Fatalf("failed migration published a target database: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(legacyHistory, "history.db")); statErr != nil {
		t.Fatalf("failed migration removed source authority: %v", statErr)
	}
	if got := paths.InspectMigration(); got.State != MigrationStatePending {
		t.Fatalf("failed migration changed source state: %+v", got)
	}
}

func TestInspectMigrationFailsClosedForUnknownSchema(t *testing.T) {
	paths := newMigrationTestPaths(t)
	dbPath := filepath.Join(paths.HistoryDir, "history.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE schema_meta (singleton INTEGER PRIMARY KEY, schema_version INTEGER NOT NULL); INSERT INTO schema_meta(singleton, schema_version) VALUES (1, 99);"); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	info := paths.InspectMigration()
	if info.State != MigrationStateInvalid || info.Error == "" {
		t.Fatalf("unknown schema was not rejected: %+v", info)
	}
	if err := paths.MigrateLegacyData(context.Background()); !errors.Is(err, ErrDataMigrationInvalid) {
		t.Fatalf("unknown schema migration error was not fail-closed: %v", err)
	}
}

func TestExplicitDataRootDoesNotScanLegacyMigrationSource(t *testing.T) {
	root := t.TempDir()
	paths, err := Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	if paths.InspectMigration().State != MigrationStateIsolated {
		t.Fatalf("explicit root was not isolated: %+v", paths.InspectMigration())
	}
	if err := paths.MigrateLegacyData(context.Background()); !errors.Is(err, ErrDataMigrationNotRequired) {
		t.Fatalf("explicit root unexpectedly offered migration: %v", err)
	}
	for _, path := range []string{paths.ProfileDir, paths.HistoryDir, paths.SettingsFile} {
		if !pathWithin(root, path) {
			t.Fatalf("explicit path escaped root: root=%q path=%q", root, path)
		}
	}
}

func newMigrationTestPaths(t *testing.T) AppPaths {
	t.Helper()
	base := t.TempDir()
	home := filepath.Join(base, "home")
	local := filepath.Join(base, "local")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(local, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv(DataDirEnv, "")
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Setenv("XDG_CONFIG_HOME", local)
	}
	paths, err := Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && len(rel) >= 3 && rel[:3] != ".."+string(os.PathSeparator)
}
