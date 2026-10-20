package gui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/faceair/clash-speedtest/core/history"
)

func TestWebHandler(t *testing.T) {
	handler := WebHandler()

	// Test root path returns index.html
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Clash SpeedTest") {
		t.Fatalf("expected body to contain 'Clash SpeedTest', got %s", body[:min(len(body), 200)])
	}
}

func TestExportClashYAML(t *testing.T) {
	proxies := []map[string]any{
		{
			"name":   "Node-01",
			"type":   "ss",
			"server": "1.2.3.4",
			"port":   8388,
		},
	}

	yamlData, err := ExportClashYAML(proxies)
	if err != nil {
		t.Fatalf("ExportClashYAML failed: %v", err)
	}

	str := string(yamlData)
	if !strings.Contains(str, "Node-01") || !strings.Contains(str, "1.2.3.4") {
		t.Fatalf("unexpected YAML output: %s", str)
	}
}

func TestExportCSV(t *testing.T) {
	results := []*history.RunNodeResult{
		{
			ProxyName:         "US-01",
			ProxyType:         "vless",
			CountryCode:       "US",
			CountryFlag:       "🇺🇸",
			LatencyMs:         150,
			DownloadSpeedMBps: 35.5,
			AntigravityStatus: "available",
		},
	}

	csvData, err := ExportCSV(results)
	if err != nil {
		t.Fatalf("ExportCSV failed: %v", err)
	}

	str := string(csvData)
	if !strings.Contains(str, "US-01") || !strings.Contains(str, "35.50") || !strings.Contains(str, "可用") {
		t.Fatalf("unexpected CSV output: %s", str)
	}
}
