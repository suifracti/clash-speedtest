package application

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/faceair/clash-speedtest/core/subscriptionusage"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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
