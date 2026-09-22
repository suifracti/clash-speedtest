package application

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/profiles"
)

const fakeAirportSecret = "VERY_SECRET_TEST_TOKEN_12345"

func TestAirportURLBoundaryKeepsOrdinaryDTOsSafeAndExplicitEditUsable(t *testing.T) {
	var responseStatus = http.StatusOK
	subscription := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(responseStatus)
		if responseStatus == http.StatusOK {
			_, _ = w.Write([]byte("proxies:\n  - name: fixture\n    type: ss\n    server: 192.0.2.1\n    port: 8388\n    cipher: aes-128-gcm\n    password: fixture-password\n"))
		}
	}))
	defer subscription.Close()
	fakeURL := subscription.URL + "/sub?token=" + fakeAirportSecret

	service, paths := newAirportURLTestService(t)
	if err := profiles.SaveStore(paths.StoreFile(), &profiles.Store{Airports: []*profiles.Airport{{
		ID:   "airport-a",
		Name: "Fixture airport",
		URL:  fakeURL,
	}}}); err != nil {
		t.Fatal(err)
	}
	if err := paths.WriteCache("airport-a", []byte("proxies:\n  - name: fixture\n    type: ss\n    server: 192.0.2.1\n    port: 8388\n    cipher: aes-128-gcm\n    password: fixture-password\n")); err != nil {
		t.Fatal(err)
	}

	list, err := service.ListAirports()
	if err != nil {
		t.Fatalf("ListAirports: %v", err)
	}
	if len(list) != 1 || list[0].URLDisplay == "" || !list[0].URLConfigured {
		t.Fatalf("unexpected safe airport DTO: %+v", list)
	}
	encoded, err := json.Marshal(list[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), fakeURL) || strings.Contains(string(encoded), fakeAirportSecret) {
		t.Fatalf("ordinary AirportDTO leaked subscription secret: %s", encoded)
	}
	if strings.Contains(string(encoded), `"url"`) {
		t.Fatalf("ordinary AirportDTO still carries a complete url field: %s", encoded)
	}

	revealed, err := service.GetAirportURL("airport-a")
	if err != nil || revealed != fakeURL {
		t.Fatalf("explicit URL read failed: value=%q err=%v", revealed, err)
	}
	created, err := service.CreateAirport("Added fixture", fakeURL, "test-agent")
	if err != nil {
		t.Fatalf("adding a subscription through the explicit input failed: %v", err)
	}
	createdJSON, err := json.Marshal(created)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(createdJSON), fakeURL) || strings.Contains(string(createdJSON), fakeAirportSecret) {
		t.Fatalf("create response leaked subscription secret: %s", createdJSON)
	}
	persisted, err := profiles.LoadStore(paths.StoreFile())
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Get(created.ID).URL != fakeURL {
		t.Fatalf("explicit create did not persist the backend URL: %+v", persisted.Get(created.ID))
	}

	updated, err := service.UpdateAirport("airport-a", "Renamed", revealed, "test-agent")
	if err != nil {
		t.Fatalf("unchanged URL edit failed: %v", err)
	}
	if updated.Name != "Renamed" || strings.Contains(updated.URLDisplay, fakeAirportSecret) {
		t.Fatalf("unexpected safe update result: %+v", updated)
	}
	persisted, err = profiles.LoadStore(paths.StoreFile())
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Get("airport-a").URL != fakeURL {
		t.Fatalf("unchanged edit did not preserve backend URL: %+v", persisted.Get("airport-a"))
	}

	if _, err := service.UpdateAirport("airport-a", "Masked", safeAirportURLDisplay(fakeURL), "test-agent"); err == nil {
		t.Fatal("masked display was accepted as an update authority")
	}
	persisted, err = profiles.LoadStore(paths.StoreFile())
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Get("airport-a").URL != fakeURL {
		t.Fatalf("masked update changed backend URL: %+v", persisted.Get("airport-a"))
	}

	responseStatus = http.StatusInternalServerError
	if _, err := service.RefreshAirport("airport-a", "test-agent"); err == nil {
		t.Fatal("expected refresh failure from fixture server")
	} else if strings.Contains(err.Error(), fakeURL) || strings.Contains(err.Error(), fakeAirportSecret) {
		t.Fatalf("refresh error leaked subscription secret: %v", err)
	}

	responseStatus = http.StatusOK
	refreshed, err := service.RefreshAirport("airport-a", "test-agent")
	if err != nil {
		t.Fatalf("refresh from backend URL failed: %v", err)
	}
	if refreshed.URLConfigured != true || strings.Contains(refreshed.URLDisplay, fakeAirportSecret) {
		t.Fatalf("refresh returned unsafe DTO: %+v", refreshed)
	}
}

func newAirportURLTestService(t *testing.T) (*AppService, profiles.Paths) {
	t.Helper()
	root := t.TempDir()
	paths := profiles.Paths{Dir: filepath.Join(root, "profiles")}
	if err := paths.InitializeEmpty(); err != nil {
		t.Fatalf("InitializeEmpty: %v", err)
	}
	historyStore, err := history.NewStore(filepath.Join(root, "history"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	service := NewAppService(historyStore, paths, nil)
	t.Cleanup(func() { _ = service.Close() })
	return service, paths
}
