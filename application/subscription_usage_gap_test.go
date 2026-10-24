package application

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestUsageRefreshKeepsThirtyMinuteGapPerAirport(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Subscription-Userinfo", fmtUsage(int64(100+calls.Load())))
		_, _ = w.Write([]byte("proxies: []"))
	}))
	defer server.Close()
	s, _ := newAirportURLTestService(t)
	if _, err := s.CreateAirport("Gap fixture", server.URL, "fixture"); err != nil {
		t.Fatal(err)
	}
	calls.Store(0)
	now := time.Now()
	if _, err := s.collectSubscriptionUsage(context.Background(), now, "fixture", true); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("first refresh: got %d calls", calls.Load())
	}
	state, err := s.collectSubscriptionUsage(context.Background(), now.Add(time.Minute), "fixture", true)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || state == nil || !strings.Contains(state.Message, "30 分钟内已刷新") {
		t.Fatalf("second refresh ignored the gap: calls=%d state=%+v err=%v", calls.Load(), state, err)
	}
	if _, err := s.collectSubscriptionUsage(context.Background(), now.Add(31*time.Minute), "fixture", true); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("refresh after the gap: got %d calls", calls.Load())
	}
}

func TestRefreshJobMarksAirportsInsideGapAsNotExecuted(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Subscription-Userinfo", fmtUsage(120))
		_, _ = w.Write([]byte("proxies: []"))
	}))
	defer server.Close()
	s, _ := newAirportURLTestService(t)
	ap, err := s.CreateAirport("Job gap fixture", server.URL, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.collectSubscriptionUsage(context.Background(), time.Now(), "fixture", true); err != nil {
		t.Fatal(err)
	}
	calls.Store(0)
	job, err := s.StartSubscriptionRefreshJob(SubscriptionRefreshRequest{RequestID: "gap-job", All: true}, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if len(job.Items) != 1 || job.Items[0].State != "not_executed" || job.Items[0].ErrorCode != "recently_refreshed" {
		t.Fatalf("job did not reuse the cache: %+v", job)
	}
	if calls.Load() != 0 {
		t.Fatalf("job inside the gap issued %d requests", calls.Load())
	}
	if ap.ID == "" {
		t.Fatal("fixture airport was not created")
	}
}
