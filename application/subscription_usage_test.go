package application

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/faceair/clash-speedtest/core/profiles"
)

func TestSubscriptionUsageRefreshPersistence(t *testing.T) {
	header := "upload=1024; download=3072; total=8192; expire=2000000000"
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Subscription-Userinfo", header)
		w.WriteHeader(status)
		_, _ = w.Write([]byte("proxies:\n  - name: fixture\n    type: ss\n    server: 192.0.2.1\n    port: 8388\n    cipher: aes-128-gcm\n    password: fixture\n"))
	}))
	defer server.Close()
	s, paths := newAirportURLTestService(t)
	ap, err := s.CreateAirport("Usage fixture", server.URL+"/sub?token=FAKE_SECRET", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	sub := ap.Subscriptions[0]
	if sub.Usage == nil || sub.Usage.Upload+sub.Usage.Download != 4096 {
		t.Fatalf("usage missing on create: %+v", sub)
	}
	saved, err := profiles.LoadStore(paths.StoreFile())
	if err != nil {
		t.Fatal(err)
	}
	if saved.Get(ap.ID).Subscriptions[0].Usage == nil {
		t.Fatal("usage not persisted")
	}
	header = "upload=2048;download=4096;total=8192"
	refreshed, err := s.RefreshSubscription(ap.ID, sub.ID, "fixture")
	if err != nil || refreshed.Usage == nil || refreshed.Usage.Download != 4096 {
		t.Fatalf("refresh: %+v %v", refreshed, err)
	}
	status = http.StatusForbidden
	if _, err := s.RefreshSubscription(ap.ID, sub.ID, "fixture"); err == nil || strings.Contains(err.Error(), "FAKE_SECRET") {
		t.Fatal("failure must be explicit and safe")
	}
	list, err := s.ListAirports()
	if err != nil {
		t.Fatal(err)
	}
	if list[0].Subscriptions[0].Usage.Download != 4096 {
		t.Fatal("failure lost last known usage")
	}
	encoded, _ := json.Marshal(list)
	if strings.Contains(string(encoded), "FAKE_SECRET") {
		t.Fatal("list leaked secret")
	}
	status = http.StatusOK
	header = ""
	refreshed, err = s.RefreshSubscription(ap.ID, sub.ID, "fixture")
	if err != nil || refreshed.Usage != nil {
		t.Fatal("missing provider header must clear old snapshot, not pretend fresh")
	}
}
