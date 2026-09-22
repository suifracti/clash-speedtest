package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faceair/clash-speedtest/core/appdata"
)

func TestProfileSetupWebChainImportsThenReadsCanonicalProfile(t *testing.T) {
	root := t.TempDir()
	paths, err := appdata.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "legacy")
	if err := os.MkdirAll(filepath.Join(source, "airports-cache"), 0o700); err != nil {
		t.Fatal(err)
	}
	fakeURL := "https://fixture.invalid/sub?token=VERY_SECRET_TEST_TOKEN_12345"
	if err := os.WriteFile(filepath.Join(source, "airports.json"), []byte(`{"airports":[{"id":"web-fixture","name":"Web fixture","url":"`+fakeURL+`"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "airports-cache", "web-fixture.yaml"), []byte("proxies:\n  - name: web-node\n    type: ss\n    server: 192.0.2.20\n    port: 8388\n    cipher: aes-128-gcm\n    password: fixture-password\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	server, err := NewServer(ServerConfig{AppPaths: paths, Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	handler := server.Handler()

	setup := requestJSON(t, handler, http.MethodGet, "/api/profile/setup", nil)
	if setup["state"] != "needs_choice" {
		t.Fatalf("fresh web profile should require explicit choice: %+v", setup)
	}

	inspect := requestJSON(t, handler, http.MethodPost, "/api/profile/source/inspect", map[string]string{"path": source})
	if inspect["available"] != true || inspect["profile_count"] != float64(1) || inspect["cache_count"] != float64(1) {
		t.Fatalf("unexpected source summary: %+v", inspect)
	}

	requestJSON(t, handler, http.MethodPost, "/api/profile/import", map[string]string{"path": source})
	ready := requestJSON(t, handler, http.MethodGet, "/api/profile/setup", nil)
	if ready["state"] != "ready" || ready["initialized"] != true {
		t.Fatalf("canonical web profile was not ready: %+v", ready)
	}

	airports := requestJSON(t, handler, http.MethodGet, "/api/airports", nil)
	items, ok := airportsAsList(airports)
	if !ok || len(items) != 1 || items[0]["id"] != "web-fixture" {
		t.Fatalf("Web airport list did not use canonical profile: %+v", airports)
	}
	encodedAirports, err := json.Marshal(airports)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedAirports), fakeURL) || strings.Contains(string(encodedAirports), "VERY_SECRET_TEST_TOKEN_12345") {
		t.Fatalf("Web airport list leaked subscription URL: %s", encodedAirports)
	}
	if _, present := items[0]["url"]; present {
		t.Fatalf("Web airport list still exposes the complete url field: %+v", items[0])
	}
	revealed := requestJSON(t, handler, http.MethodGet, "/api/airports/web-fixture/url", nil)
	if revealed["url"] != fakeURL {
		t.Fatalf("explicit URL read returned %v, want %q", revealed["url"], fakeURL)
	}
	nodes := requestJSON(t, handler, http.MethodGet, "/api/airports/web-fixture/nodes", nil)
	nodeItems, ok := airportsAsList(nodes)
	if !ok || len(nodeItems) != 1 || nodeItems[0]["name"] != "web-node" {
		t.Fatalf("Web node options did not use imported cache: %+v", nodes)
	}
}

func requestJSON(t *testing.T, handler http.Handler, method, path string, body any) map[string]any {
	t.Helper()
	var payload *bytes.Reader
	if body == nil {
		payload = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		payload = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, payload)
	req.Host = "127.0.0.1:8080"
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code < 200 || rec.Code >= 300 {
		t.Fatalf("%s %s failed: status=%d body=%s", method, path, rec.Code, rec.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err == nil {
		return result
	}
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode response %s %s: %v body=%s", method, path, err, rec.Body.String())
	}
	return map[string]any{"items": list}
}

func airportsAsList(payload map[string]any) ([]map[string]any, bool) {
	items, ok := payload["items"].([]map[string]any)
	return items, ok
}
