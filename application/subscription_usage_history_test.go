package application

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/appdata"
	"github.com/faceair/clash-speedtest/core/subscriptionusage"
)

func TestSubscriptionDailyRefreshRecordsUsageWithoutDuplicateAccounts(t *testing.T) {
	var calls atomic.Int32
	var download atomic.Int64
	download.Store(100)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Subscription-Userinfo", fmtUsage(download.Load()))
		_, _ = w.Write([]byte("proxies: []"))
	}))
	defer server.Close()
	s, _ := newAirportURLTestService(t)
	ap, err := s.CreateAirport("Fixture", server.URL+"/?token=PRIVATE_USAGE_TEST", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddSubscription(ap.ID, "重复地址", server.URL+"/?token=PRIVATE_USAGE_TEST", "", "fixture"); err != nil {
		t.Fatal(err)
	}
	calls.Store(0)
	download.Store(130)
	now := time.Now()
	if _, err := s.collectSubscriptionUsage(context.Background(), now, "fixture", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.collectSubscriptionUsage(context.Background(), now, "fixture", false); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected each imported subscription refreshed once, no second daily pass: %d", calls.Load())
	}
	day := now.In(subscriptionusage.Location).Format("2006-01-02")
	report, err := s.GetSubscriptionUsage(context.Background(), day, day, "day", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Accounts) != 1 || report.ObservedBytes == nil || *report.ObservedBytes != 30 {
		t.Fatalf("same provider account counted twice: %+v", report)
	}
	raw, _ := json.Marshal(report)
	if strings.Contains(string(raw), "PRIVATE_USAGE_TEST") || strings.Contains(string(raw), server.URL) {
		t.Fatal("usage API leaked source URL")
	}
	settings, _ := s.GetSettings()
	disabled := false
	settings.SubscriptionDailyUpdateEnabled = &disabled
	if err := s.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	if _, err := s.collectSubscriptionUsage(context.Background(), now.AddDate(0, 0, 2), "fixture", false); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("disabled daily update issued requests")
	}
}

func fmtUsage(download int64) string {
	return fmt.Sprintf("upload=0; download=%d; total=1000", download)
}

func TestSubscriptionUsageNilDefenses(t *testing.T) {
	// 1. fillDefaultSubscriptionUsage with nil settings should not panic
	fillDefaultSubscriptionUsage(nil)

	// 2. fillDefaultSubscriptionUsage with nil SubscriptionDailyUpdateEnabled should initialize it to true
	var s AppSettings
	fillDefaultSubscriptionUsage(&s)
	if s.SubscriptionDailyUpdateEnabled == nil || !*s.SubscriptionDailyUpdateEnabled {
		t.Fatalf("expected SubscriptionDailyUpdateEnabled to default to true, got: %v", s.SubscriptionDailyUpdateEnabled)
	}

	// 3. usageSnapshot with nil airport and nil subscription should not panic
	snap := usageSnapshot(nil, nil, "missing", "test", time.Now())
	if snap.Status != "missing" || snap.AirportID != "" || snap.SubscriptionID != "" {
		t.Fatalf("unexpected snapshot with nil parameters: %+v", snap)
	}

	// 4. validateMonitorStorageThresholds & validateMonitorBudget with nil settings should not panic
	if err := validateMonitorStorageThresholds(nil); err != nil {
		t.Fatalf("validateMonitorStorageThresholds(nil) returned unexpected error: %v", err)
	}
	if err := validateMonitorBudget(nil); err != nil {
		t.Fatalf("validateMonitorBudget(nil) returned unexpected error: %v", err)
	}
}

func TestSaveSettingsAtomicPersistence(t *testing.T) {
	tempDir := t.TempDir()
	paths, err := appdata.Resolve(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewAppServiceWithPaths(nil, paths, nil)

	s, err := svc.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	s.PreferredBrowser = "firefox"
	if err := svc.SaveSettings(s); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	info, err := os.Stat(paths.SettingsFile)
	if err != nil {
		t.Fatalf("settings.json does not exist: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected permissions 0600, got: %v", info.Mode().Perm())
	}

	entries, err := os.ReadDir(paths.DataRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("leftover temporary file found: %s", entry.Name())
		}
	}

	reopened := NewAppServiceWithPaths(nil, paths, nil)
	got, err := reopened.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings failed: %v", err)
	}
	if got.PreferredBrowser != "firefox" {
		t.Fatalf("expected PreferredBrowser firefox, got: %s", got.PreferredBrowser)
	}
}

