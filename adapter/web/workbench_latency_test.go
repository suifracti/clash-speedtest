package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/faceair/clash-speedtest/application"
	"github.com/faceair/clash-speedtest/core/profiles"
	"gopkg.in/yaml.v2"
)

func TestWebWorkbenchLatencyContractAndReopen(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(proxy.Close)
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatalf("parse proxy URL: %v", err)
	}
	host, portText, _ := strings.Cut(proxyURL.Host, ":")
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("proxy port: %v", err)
	}

	tmpDir := t.TempDir()
	profileDir := filepath.Join(tmpDir, "profiles")
	paths := profiles.Paths{Dir: profileDir}
	if err := profiles.SaveStore(paths.StoreFile(), &profiles.Store{Airports: []*profiles.Airport{{ID: "profile-web", Name: "Web 订阅"}}}); err != nil {
		t.Fatalf("SaveStore: %v", err)
	}
	body, err := yaml.Marshal(map[string]any{"proxies": []map[string]any{{
		"name":   "Web 节点",
		"type":   "http",
		"server": host,
		"port":   port,
	}}})
	if err != nil {
		t.Fatalf("marshal cache: %v", err)
	}
	if err := paths.WriteCache("profile-web", body); err != nil {
		t.Fatalf("WriteCache: %v", err)
	}

	historyDir := filepath.Join(tmpDir, "history")
	server, err := NewServer(ServerConfig{Port: 0, ProfilePaths: paths, HistoryDir: historyDir})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	optionsReq := httptest.NewRequest(http.MethodGet, "/api/monitor/nodes", nil)
	optionsReq.Host = "127.0.0.1:8080"
	optionsRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(optionsRec, optionsReq)
	if optionsRec.Code != http.StatusOK {
		t.Fatalf("node options status: %d body=%s", optionsRec.Code, optionsRec.Body.String())
	}
	var options []application.MonitorNodeOptionDTO
	if err := json.Unmarshal(optionsRec.Body.Bytes(), &options); err != nil || len(options) != 1 {
		t.Fatalf("node options: %v %+v", err, options)
	}

	payload, _ := json.Marshal(application.WorkbenchLatencyTestRequest{
		ProfileID:      "profile-web",
		NodeKey:        options[0].NodeKey,
		TestProject:    application.WorkbenchLatencyProject,
		TimeoutSeconds: 1,
	})
	testReq := httptest.NewRequest(http.MethodPost, "/api/workbench/latency-tests", bytes.NewReader(payload))
	testReq.Host = "127.0.0.1:8080"
	testReq.Header.Set("Content-Type", "application/json")
	testRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(testRec, testReq)
	if testRec.Code != http.StatusOK {
		t.Fatalf("latency test status: %d body=%s", testRec.Code, testRec.Body.String())
	}
	var result application.WorkbenchLatencyTestDTO
	if err := json.Unmarshal(testRec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.PersistenceState != "saved" || len(result.Samples) == 0 {
		t.Fatalf("unexpected Web result: %+v", result)
	}
	if err := server.Close(); err != nil {
		t.Fatalf("close first server: %v", err)
	}

	server, err = NewServer(ServerConfig{Port: 0, ProfilePaths: paths, HistoryDir: historyDir})
	if err != nil {
		t.Fatalf("reopen NewServer: %v", err)
	}
	defer server.Close()
	historyReq := httptest.NewRequest(http.MethodGet, "/api/workbench/latency-tests?profile_id=profile-web&node_key="+url.QueryEscape(options[0].NodeKey), nil)
	historyReq.Host = "127.0.0.1:8080"
	historyRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(historyRec, historyReq)
	if historyRec.Code != http.StatusOK {
		t.Fatalf("history status: %d body=%s", historyRec.Code, historyRec.Body.String())
	}
	var rows []application.WorkbenchLatencyTestDTO
	if err := json.Unmarshal(historyRec.Body.Bytes(), &rows); err != nil || len(rows) != 1 {
		t.Fatalf("reopened history: %v %+v", err, rows)
	}
	if rows[0].AttemptID != result.AttemptID || len(rows[0].Samples) != len(result.Samples) {
		t.Fatalf("reopened Web history mismatch: %+v", rows)
	}
}
