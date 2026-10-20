package web

import (
	"encoding/json"
	"github.com/faceair/clash-speedtest/core/profiles"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSubscriptionNodeChangesAndFailedRefresh(t *testing.T) {
	var failed atomic.Bool
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failed.Load() {
			w.WriteHeader(503)
			return
		}
		w.Header().Set("Subscription-Userinfo", "upload=0;download=1;total=100")
		_, _ = w.Write([]byte(strings.ReplaceAll(validRefreshBody, "name: fixture", "name: renamed") + "\n - {name: added, type: ss, server: 192.0.2.2, port: 443, cipher: aes-128-gcm, password: fixture}"))
	}))
	defer provider.Close()
	server, paths, _ := accountUsageFixture(t, provider.URL, true)
	if err := paths.WriteCache("one", []byte(validRefreshBody+"\n - {name: offline, type: ss, server: 192.0.2.3, port: 443, cipher: aes-128-gcm, password: fixture}")); err != nil {
		t.Fatal(err)
	}
	if _, err := server.app.RefreshSubscription("a", "one", ""); err != nil {
		t.Fatal(err)
	}
	response := performRefreshRequest(server.Handler(), http.MethodGet, "/api/airports/a/subscriptions/one/changes", "")
	var changes profiles.SubscriptionNodeChanges
	if err := json.Unmarshal(response.Body.Bytes(), &changes); err != nil || response.Code != 200 || len(changes.Added) != 1 || changes.Added[0] != "added" || len(changes.Removed) != 1 || changes.Removed[0] != "offline" || len(changes.Renamed) != 1 || changes.Renamed[0].To != "renamed" {
		t.Fatalf("changes=%s err=%v", response.Body.String(), err)
	}
	store, err := profiles.LoadStore(paths.StoreFile())
	if err != nil || len(store.Get("a").GetSubscription("one").NodeVersions.Previous) != 2 {
		t.Fatal("previous node list not persisted", err)
	}
	failed.Store(true)
	if _, err := server.app.RefreshSubscription("a", "one", ""); err == nil {
		t.Fatal("expected refresh failure")
	}
	after := performRefreshRequest(server.Handler(), http.MethodGet, "/api/airports/a/subscriptions/one/changes", "")
	if after.Body.String() != response.Body.String() {
		t.Fatal("failed refresh replaced previous changes")
	}
}
