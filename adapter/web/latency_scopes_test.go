package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/profiles"
)

func TestWebLatencyHistoryScopesUsesSavedEvidenceAndValidatesWindow(t *testing.T) {
	dir := t.TempDir()
	historyDir := filepath.Join(dir, "history")
	store, err := history.NewStore(historyDir)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	if err := store.SaveLatencyTest(context.Background(), &history.LatencyTest{AttemptID: "past", ProfileID: "account", NodeKey: "node", NodeIdentityKey: "identity", ConfigRevisionKey: "revision", DisplayName: "Node 名称", TestProject: "latency_stability", Status: "completed", RequestedAt: at, StartedAt: at, FinishedAt: at, Samples: []history.LatencyTestSample{{Seq: 1, Timestamp: at, Success: true}}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(ServerConfig{Port: 0, ProfilePaths: profiles.Paths{Dir: filepath.Join(dir, "profiles")}, HistoryDir: historyDir})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	handler := server.buildHandler()
	query := url.Values{"profile_id": {"account"}, "display_name": {"Node 名称"}, "since": {at.Add(-time.Hour).Format(time.RFC3339)}, "until": {at.Add(time.Hour).Format(time.RFC3339)}}
	get := func(q url.Values) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/api/history/latency-scopes?"+q.Encode(), nil)
		r.Host = "127.0.0.1:8080"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	w := get(query)
	var scopes []history.LatencyHistoryScope
	if err := json.Unmarshal(w.Body.Bytes(), &scopes); err != nil || w.Code != 200 || len(scopes) != 1 || scopes[0].NodeIdentityKey != "identity" {
		t.Fatalf("scope response %d %s, err=%v", w.Code, w.Body, err)
	}
	query.Set("profile_id", "other-account")
	w = get(query)
	if w.Code != 200 || w.Body.String() != "[]\n" {
		t.Fatalf("other account response: %d %s", w.Code, w.Body)
	}
	for _, field := range []string{"profile_id", "display_name", "since", "until"} {
		value := query.Get(field)
		query.Del(field)
		if w := get(query); w.Code != http.StatusBadRequest {
			t.Errorf("missing %s: %d %s", field, w.Code, w.Body)
		}
		query.Set(field, value)
	}
	query.Set("since", query.Get("until"))
	if w := get(query); w.Code != http.StatusBadRequest {
		t.Fatalf("empty time window: %d %s", w.Code, w.Body)
	}
}
