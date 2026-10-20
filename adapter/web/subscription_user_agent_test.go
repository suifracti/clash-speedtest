package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/profiles"
)

func TestSubscriptionUserAgentSettingIsLocalAndIsolated(t *testing.T) {
	var calls atomic.Int32
	uas := make(chan string, 8)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		uas <- r.UserAgent()
		w.Header().Set("Subscription-Userinfo", "upload=0;download=1;total=100")
		_, _ = w.Write([]byte(validRefreshBody))
	}))
	defer provider.Close()
	root := t.TempDir()
	paths := profiles.Paths{Dir: filepath.Join(root, "profiles")}
	oldAt := time.Now().Add(-time.Hour).UTC()
	store := &profiles.Store{Airports: []*profiles.Airport{{ID: "a", Name: "A", Subscriptions: []*profiles.Subscription{{ID: "one", Name: "One", URL: provider.URL + "/one", UpdatedAt: oldAt}, {ID: "two", Name: "Two", URL: provider.URL + "/two", UpdatedAt: oldAt}}}}}
	if err := profiles.SaveStore(paths.StoreFile(), store); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"one", "two"} {
		if err := paths.WriteCache(id, []byte(validRefreshBody)); err != nil {
			t.Fatal(err)
		}
	}
	server, err := NewServer(ServerConfig{ProfilePaths: paths, HistoryDir: filepath.Join(root, "history"), UserAgent: "global-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	handler := server.Handler()
	response := performRefreshRequest(handler, http.MethodPut, "/api/airports/a/subscriptions/one/user-agent", `{"user_agent":" custom-fixture "}`)
	if response.Code != http.StatusOK {
		t.Fatalf("setting UA: %d %s", response.Code, response.Body.String())
	}
	var dto map[string]any
	_ = json.Unmarshal(response.Body.Bytes(), &dto)
	if dto["user_agent"] != "custom-fixture" || calls.Load() != 0 {
		t.Fatalf("setting must return persisted UA without network: %v calls=%d", dto, calls.Load())
	}
	saved, err := profiles.LoadStore(paths.StoreFile())
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Get("a").GetSubscription("one").UpdatedAt.Equal(oldAt) {
		t.Fatal("setting UA fabricated a node update")
	}
	body, err := os.ReadFile(paths.CacheFile("one"))
	if err != nil || string(body) != validRefreshBody {
		t.Fatal("setting UA changed node cache")
	}
	for _, test := range []struct{ id, want string }{{"one", "custom-fixture"}, {"two", "global-fixture"}} {
		response = performRefreshRequest(handler, http.MethodPost, "/api/airports/a/subscriptions/"+test.id+"/refresh", "")
		if response.Code != http.StatusOK {
			t.Fatalf("refresh: %d %s", response.Code, response.Body.String())
		}
		if got := <-uas; got != test.want {
			t.Fatalf("UA for %s =%q want %q", test.id, got, test.want)
		}
	}
	response = performRefreshRequest(handler, http.MethodPost, "/api/subscription-usage/refresh", "")
	if response.Code != http.StatusOK {
		t.Fatalf("daily collection: %d %s", response.Code, response.Body.String())
	}
	for _, want := range []string{"custom-fixture", "global-fixture"} {
		select {
		case got := <-uas:
			if got != want {
				t.Fatalf("daily collection UA=%q want %q", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("daily collection did not fetch account")
		}
	}

	response = performRefreshRequest(handler, http.MethodPut, "/api/airports/a/subscriptions/one/user-agent", `{"user_agent":""}`)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	response = performRefreshRequest(handler, http.MethodPost, "/api/airports/a/subscriptions/one/refresh", "")
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	if got := <-uas; got != "global-fixture" {
		t.Fatalf("cleared UA did not restore fallback: %q", got)
	}
	response = performRefreshRequest(handler, http.MethodPut, "/api/airports/a/subscriptions/one/user-agent", `{"user_agent":"bad\r\nheader"}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid UA accepted: %d", response.Code)
	}
}
