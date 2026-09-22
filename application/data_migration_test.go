package application

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/appdata"
	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
)

func TestAppServiceMigrationReopensJobsWithoutStartingThem(t *testing.T) {
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
	t.Setenv(appdata.DataDirEnv, "")
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Setenv("XDG_CONFIG_HOME", local)
	}
	paths, err := appdata.Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	legacyHistory := paths.LegacyRoot()
	legacyHistory = filepath.Join(legacyHistory, "history")
	source, err := history.NewStore(legacyHistory)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.SaveMonitorJobDefinition(context.Background(), &monitor.MonitorJobDefinition{
		ID:                "job-reopen-001",
		Name:              "persisted job",
		ProfileID:         "missing-profile",
		Nodes:             []monitor.MonitorJobNodeReference{{NodeKey: "node-a", NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a", DisplayName: "node-a", Type: "ss"}},
		ProbeSet:          monitor.ProbeSetLight,
		Interval:          time.Minute,
		Timeout:           time.Second,
		CreatedAt:         time.Now().UTC().Add(-time.Hour),
		UpdatedAt:         time.Now().UTC(),
		DefinitionVersion: monitor.MonitorJobDefinitionVersion,
	}); err != nil {
		_ = source.Close()
		t.Fatal(err)
	}
	service := NewAppServiceWithPaths(source, paths, nil)
	if err := service.MigrateLegacyData(); err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	jobs := service.ListMonitorJobs()
	if len(jobs) != 1 || jobs[0].ID != "job-reopen-001" {
		t.Fatalf("persisted job was not reloaded: %+v", jobs)
	}
	if jobs[0].State != monitor.JobStateBlocked {
		t.Fatalf("missing profile should block restored job, got %+v", jobs[0])
	}
	runs, err := service.QueryMonitorRuns(context.Background(), "job-reopen-001", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("migration startup created monitor runs: %+v", runs)
	}
	if info := paths.InspectMigration(); info.State != appdata.MigrationStateReady {
		t.Fatalf("canonical migration did not become authoritative: %+v", info)
	}
}
