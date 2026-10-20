package application

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/profiles"
	"github.com/faceair/clash-speedtest/core/subscriptionusage"
)

func TestPartialUsageRefreshPreservesUnknownCounters(t *testing.T) {
	const header = "download=3.072e3;total=8192;expire=2000000000"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Subscription-Userinfo", header)
		_, _ = w.Write([]byte("proxies: []"))
	}))
	defer server.Close()
	s, paths := newAirportURLTestService(t)
	airport, err := s.CreateAirport("Partial fixture", server.URL, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	sub, err := s.RefreshSubscription(airport.ID, airport.Subscriptions[0].ID, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if sub.Usage == nil || sub.Usage.Upload != nil || sub.Usage.Download == nil || *sub.Usage.Download != 3072 || sub.Usage.Total == nil || *sub.Usage.Total != 8192 || sub.Usage.RawHeader != header {
		t.Fatalf("partial DTO lost fields: %+v", sub.Usage)
	}
	saved, err := profiles.LoadStore(paths.StoreFile())
	if err != nil {
		t.Fatal(err)
	}
	if saved.Get(airport.ID).Subscriptions[0].Usage.Upload != nil {
		t.Fatal("unknown persisted as zero")
	}
	snaps, err := s.historyStore.QuerySubscriptionUsage(context.Background(), time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) == 0 {
		t.Fatal("no history observation")
	}
	for _, snap := range snaps {
		if snap.Status != "partial" || snap.Upload != nil || snap.Download == nil || *snap.Download != 3072 {
			t.Fatalf("partial observation became full accounting: %+v", snap)
		}
	}
	day := time.Now().In(subscriptionusage.Location).Format("2006-01-02")
	report, err := s.GetSubscriptionUsage(context.Background(), day, day, "day", "")
	if err != nil {
		t.Fatal(err)
	}
	if report.ObservedBytes != nil || report.UnknownIntervals == 0 {
		t.Fatalf("partial counters became a known daily increment: %+v", report)
	}
}
