package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/appdata"
	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
)

func TestMonitorRetentionPreferenceUsesCanonicalSettingsWithoutDeletion(t *testing.T) {
	paths, err := appdata.Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.SettingsFile, []byte(`{"preferred_browser":"edge","unrelated_future_key":"preserve"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewAppServiceWithPaths(nil, paths, nil)
	initial, err := service.GetSettings()
	if err != nil || initial.MonitorRetentionPolicy != monitor.RetentionKeepAll {
		t.Fatalf("existing settings must default to keep_all: %+v %v", initial, err)
	}
	if *initial.MonitorStorageWarningBytes != 1<<30 || *initial.MonitorStorageHardBytes != 2<<30 {
		t.Fatalf("legacy settings lost 1/2 GiB default thresholds: %+v", initial)
	}
	if *initial.MonitorBudgetMaxConcurrent != 4 || *initial.MonitorBudgetDailyRequests != 20000 || *initial.MonitorBudgetDailyBytes != 32<<20 || *initial.MonitorBudgetResponseBytes != 256<<10 {
		t.Fatalf("legacy settings lost Monitor budget defaults: %+v", initial)
	}
	initial.MonitorRetentionPolicy = monitor.RetentionCustom
	initial.MonitorRetentionCustomDays = 45
	newRequests := int64(120)
	initial.MonitorBudgetDailyRequests = &newRequests
	if err := service.SaveSettings(initial); err != nil {
		t.Fatal(err)
	}
	reopened := NewAppServiceWithPaths(nil, paths, nil)
	got, err := reopened.GetSettings()
	if err != nil || got.MonitorRetentionPolicy != monitor.RetentionCustom || got.MonitorRetentionCustomDays != 45 || *got.MonitorBudgetDailyRequests != newRequests {
		t.Fatalf("retention preference not retained: %+v %v", got, err)
	}
	invalidRequests := int64(0)
	got.MonitorBudgetDailyRequests = &invalidRequests
	if err := reopened.SaveSettings(got); err == nil {
		t.Fatal("zero request budget was accepted")
	}
	got.MonitorBudgetDailyRequests = &newRequests
	got.MonitorRetentionPolicy = monitor.RetentionKeepAll
	got.MonitorRetentionCustomDays = 0
	if err := reopened.SaveSettings(got); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(paths.SettingsFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"unrelated_future_key": "preserve"`) || !strings.Contains(string(raw), `"monitor_retention_custom_days": 0`) {
		t.Fatalf("canonical settings did not preserve unknown fields or reset custom days: %s", raw)
	}
	if _, err := os.Stat(filepath.Join(paths.DataRoot, "history")); !os.IsNotExist(err) {
		t.Fatalf("saving retention preference must not create or prune history: %v", err)
	}

	store, err := history.NewStore(paths.HistoryDir)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewAppServiceWithPaths(store, paths, nil)
	defer svc.Close()
	lowWarning, lowHard := int64(1), int64(2)
	got.MonitorStorageWarningBytes, got.MonitorStorageHardBytes = &lowWarning, &lowHard
	if err := svc.SaveSettings(got); err != nil {
		t.Fatal(err)
	}
	usage, err := svc.GetMonitorStorageUsage()
	if err != nil || !usage.Protected || usage.TotalBytes <= lowHard {
		t.Fatalf("real SQLite files did not trigger protection: %+v %v", usage, err)
	}
	if _, err := svc.CreateMonitorJob(monitor.MonitorJob{ID: "stopped-job", Interval: time.Hour}); err != nil {
		t.Fatal(err)
	}
	if err := svc.StartMonitorJob("stopped-job"); err == nil {
		t.Fatal("protected job started against real SQLite usage")
	}
	if runs, err := svc.QueryMonitorRuns(context.Background(), "stopped-job", 10); err != nil || len(runs) != 0 {
		t.Fatalf("protected start created a run: %+v %v", runs, err)
	}

	// Raising the user-controlled threshold releases protection without changing
	// the SQLite file size or silently starting a stopped job.
	usage, err = svc.GetMonitorStorageUsage()
	if err != nil {
		t.Fatal(err)
	}
	highWarning, highHard := usage.TotalBytes+(1<<20), usage.TotalBytes+(2<<20)
	got.MonitorStorageWarningBytes, got.MonitorStorageHardBytes = &highWarning, &highHard
	if err := svc.SaveSettings(got); err != nil {
		t.Fatal(err)
	}
	usage, err = svc.GetMonitorStorageUsage()
	if err != nil || usage.Protected || usage.HardBytes != highHard {
		t.Fatalf("saved threshold did not release protection: %+v %v", usage, err)
	}
	readBack, err := svc.GetSettings()
	if err != nil || *readBack.MonitorStorageWarningBytes != highWarning || *readBack.MonitorStorageHardBytes != highHard {
		t.Fatalf("canonical threshold read-back failed: %+v %v", readBack, err)
	}
	if err := svc.SaveSettings(&AppSettings{PreferredBrowser: "chrome", MonitorRetentionPolicy: monitor.RetentionKeepAll}); err != nil {
		t.Fatal(err)
	}
	readBack, err = svc.GetSettings()
	if err != nil || *readBack.MonitorStorageWarningBytes != highWarning || *readBack.MonitorStorageHardBytes != highHard || readBack.PreferredBrowser != "chrome" {
		t.Fatalf("unrelated settings save overwrote thresholds: %+v %v", readBack, err)
	}
	if stopped, err := svc.GetMonitorJob("stopped-job"); err != nil || stopped.State != monitor.JobStateStopped {
		t.Fatalf("setting save auto-started stopped job: %+v %v", stopped, err)
	}
	if runs, err := svc.QueryMonitorRuns(context.Background(), "stopped-job", 10); err != nil || len(runs) != 0 {
		t.Fatalf("threshold save created a run: %+v %v", runs, err)
	}

	if _, err := svc.CreateMonitorJob(monitor.MonitorJob{ID: "paused-job", Interval: time.Hour}); err != nil {
		t.Fatal(err)
	}
	if err := svc.StartMonitorJob("paused-job"); err != nil {
		t.Fatal(err)
	}
	waitForIdleMonitorScheduler(t, store, "paused-job")
	if err := svc.PauseMonitorJob("paused-job"); err != nil {
		t.Fatal(err)
	}
	before, err := svc.QueryMonitorRuns(context.Background(), "paused-job", 10)
	if err != nil {
		t.Fatal(err)
	}
	got.MonitorStorageWarningBytes, got.MonitorStorageHardBytes = &lowWarning, &lowHard
	if err := svc.SaveSettings(got); err != nil {
		t.Fatal(err)
	}
	if usage, err := svc.GetMonitorStorageUsage(); err != nil || !usage.Protected {
		t.Fatalf("lowered threshold did not protect: %+v %v", usage, err)
	}
	got.MonitorStorageWarningBytes, got.MonitorStorageHardBytes = &highWarning, &highHard
	if err := svc.SaveSettings(got); err != nil {
		t.Fatal(err)
	}
	if paused, err := svc.GetMonitorJob("paused-job"); err != nil || paused.State != monitor.JobStatePaused {
		t.Fatalf("setting save resumed paused job: %+v %v", paused, err)
	}
	after, err := svc.QueryMonitorRuns(context.Background(), "paused-job", 10)
	if err != nil || len(after) != len(before) {
		t.Fatalf("threshold save triggered a paused probe: before=%d after=%d %v", len(before), len(after), err)
	}
	invalidWarning := highHard
	got.MonitorStorageWarningBytes = &invalidWarning
	if err := svc.SaveSettings(got); err == nil {
		t.Fatal("warning >= hard must be rejected")
	}
	if unchanged, err := svc.GetSettings(); err != nil || *unchanged.MonitorStorageWarningBytes != highWarning {
		t.Fatalf("invalid threshold overwrote canonical settings: %+v %v", unchanged, err)
	}
}
