package application

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/profiles"
)

func TestAirportMaintenanceSchedule(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Subscription-Userinfo", "upload=0; download=0; total=1000")
		_, _ = w.Write([]byte("proxies: []"))
	}))
	defer server.Close()
	s, paths := newAirportURLTestService(t)
	ap, err := s.CreateAirport("fixture", server.URL, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	requests.Store(0)
	settings := profiles.AirportMaintenance{RefreshHours: 6, Links: []profiles.MaintenanceLink{{Label: "月初活动", URL: "https://example.com/activity", MonthlyDay: 1, DoneMonth: "2026-09"}}}
	if err := s.SaveAirportMaintenance(ap.ID, settings); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 0 {
		t.Fatal("saving settings issued requests")
	}
	saved, _ := profiles.LoadStore(paths.StoreFile())
	next := saved.Get(ap.ID).Maintenance.NextRefresh
	if saved.Get(ap.ID).Maintenance.Links[0].DoneMonth != "2026-09" {
		t.Fatal("monthly completion not persisted")
	}
	if err := s.refreshDueAirport(context.Background(), next.Add(-time.Second), "fixture"); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 0 {
		t.Fatal("refreshed before due")
	}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.refreshDueAirport(context.Background(), next, "fixture"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if requests.Load() != 1 {
		t.Fatalf("duplicate scheduled refresh: %d", requests.Load())
	}
	// Reopening the service uses persisted next due; no new quota or catch-up burst.
	reopened := &AppService{profilePaths: paths}
	if err := reopened.refreshDueAirport(context.Background(), next, "fixture"); err != nil {
		t.Fatal(err)
	}
	if err := reopened.refreshDueAirport(context.Background(), next.Add(90*24*time.Hour), "fixture"); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatal("missed intervals were replayed")
	}
	settings.RefreshHours = 0
	if err := s.SaveAirportMaintenance(ap.ID, settings); err != nil {
		t.Fatal(err)
	}
	if err := s.refreshDueAirport(context.Background(), next.Add(365*24*time.Hour), "fixture"); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatal("disabled schedule issued request")
	}
	settings.Links[0].URL = "javascript:alert(1)"
	if err := s.SaveAirportMaintenance(ap.ID, settings); err == nil {
		t.Fatal("accepted unsafe link")
	}
}
