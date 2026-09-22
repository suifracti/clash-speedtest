package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faceair/clash-speedtest/core/appdata"
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
	initial.MonitorRetentionPolicy = monitor.RetentionCustom
	initial.MonitorRetentionCustomDays = 45
	if err := service.SaveSettings(initial); err != nil {
		t.Fatal(err)
	}
	reopened := NewAppServiceWithPaths(nil, paths, nil)
	got, err := reopened.GetSettings()
	if err != nil || got.MonitorRetentionPolicy != monitor.RetentionCustom || got.MonitorRetentionCustomDays != 45 {
		t.Fatalf("retention preference not retained: %+v %v", got, err)
	}
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
}
