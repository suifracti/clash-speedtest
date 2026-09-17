package gui

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/profiles"
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

func TestBroadcaster(t *testing.T) {
	b := NewBroadcaster()
	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	event := Event{
		Type: "test_event",
		Payload: map[string]string{
			"hello": "world",
		},
	}

	go b.Broadcast(event)

	select {
	case msg := <-ch:
		var received Event
		if err := json.Unmarshal(msg, &received); err != nil {
			t.Fatalf("unmarshal broadcast message: %v", err)
		}
		if received.Type != "test_event" {
			t.Fatalf("expected type test_event, got %s", received.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for broadcast event")
	}
}

func TestServerLifecycle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "server-test-*")
	if err != nil {
		t.Fatalf("temp dir failed: %v", err)
	}
	defer os.RemoveAll(tempDir)

	server, err := NewServer(ServerConfig{
		Port:         0, // Ephemeral port
		ProfilePaths: profiles.Paths{Dir: tempDir},
		HistoryDir:   tempDir,
	})
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if err := server.Start(); err != nil {
		t.Fatalf("Start server failed: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Stop(ctx)
	}()

	url := server.URL()

	// 1. Check /api/antigravity/status
	resp, err := http.Get(url + "/api/antigravity/status")
	if err != nil {
		t.Fatalf("GET status failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	// 2. Check /api/airports
	respAirports, err := http.Get(url + "/api/airports")
	if err != nil {
		t.Fatalf("GET airports failed: %v", err)
	}
	defer respAirports.Body.Close()
	body, _ := io.ReadAll(respAirports.Body)
	if respAirports.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", respAirports.StatusCode, string(body))
	}

	// 3. Set token via API
	setTokenReq, _ := json.Marshal(map[string]string{"token": "ya29.test-mock-token-for-testing"})
	respToken, err := http.Post(url+"/api/antigravity/token", "application/json", strings.NewReader(string(setTokenReq)))
	if err != nil {
		t.Fatalf("POST token failed: %v", err)
	}
	defer respToken.Body.Close()
	if respToken.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", respToken.StatusCode)
	}
	defer func() {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			_ = os.Remove(filepath.Join(home, ".antigravity_token"))
		}
	}()

	tok, _ := server.GetAntigravityToken()
	if tok != "ya29.test-mock-token-for-testing" {
		t.Fatalf("expected token set, got %s", tok)
	}

	// 4. Test /api/settings
	respSettings, err := http.Get(url + "/api/settings")
	if err != nil {
		t.Fatalf("GET settings failed: %v", err)
	}
	defer respSettings.Body.Close()
	if respSettings.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", respSettings.StatusCode)
	}

	var sResp SettingsResp
	if err := json.NewDecoder(respSettings.Body).Decode(&sResp); err != nil {
		t.Fatalf("decode settings failed: %v", err)
	}
	if len(sResp.AvailableBrowsers) == 0 {
		t.Fatal("expected at least 1 available browser")
	}

	// 5. Test saving browser preference
	saveReq, _ := json.Marshal(map[string]string{"preferred_browser": "chrome"})
	respSave, err := http.Post(url+"/api/settings", "application/json", strings.NewReader(string(saveReq)))
	if err != nil {
		t.Fatalf("POST settings failed: %v", err)
	}
	defer respSave.Body.Close()
	if respSave.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", respSave.StatusCode)
	}

	// 6. Test airport node parsing from cache
	yamlSample := `proxies:
  - name: "🇭🇰 香港专线 01"
    type: ss
    server: 1.1.1.1
    port: 8388
    cipher: aes-128-gcm
    password: test
  - name: "🇯🇵 日本高速 01"
    type: ss
    server: 2.2.2.2
    port: 8388
    cipher: aes-128-gcm
    password: test
`
	paths := profiles.Paths{Dir: tempDir}
	_ = paths.WriteCache("test-ap", []byte(yamlSample))

	cnt := server.countCachedNodes("test-ap")
	if cnt != 2 {
		t.Fatalf("expected 2 cached nodes, got %d", cnt)
	}

	respNodes, err := http.Get(url + "/api/airports/test-ap/nodes")
	if err != nil {
		t.Fatalf("GET airport nodes failed: %v", err)
	}
	defer respNodes.Body.Close()
	if respNodes.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(respNodes.Body)
		t.Fatalf("expected status 200, got %d: %s", respNodes.StatusCode, string(b))
	}

	var nodesResp AirportNodesResp
	if err := json.NewDecoder(respNodes.Body).Decode(&nodesResp); err != nil {
		t.Fatalf("decode nodes resp: %v", err)
	}
	if nodesResp.TotalNodes != 2 {
		t.Fatalf("expected 2 nodes, got %d", nodesResp.TotalNodes)
	}

	// 7. Test History Nodes, Timeline, and Compare APIs
	r1 := &history.TestRun{
		ID:          "run-1",
		AirportID:   "ap-1",
		AirportName: "Airport 1",
		CreatedAt:   time.Now().Add(-1 * time.Hour),
		PassedNodes: 1,
		TotalNodes:  1,
		Results: []*history.RunNodeResult{
			{
				ProxyName:         "US-01",
				CountryCode:       "US",
				LatencyMs:         160,
				DownloadSpeedMBps: 20.0,
				AntigravityStatus: "available",
				IPInfo: &history.IPInfo{
					IP:         "198.51.100.1",
					ASN:        "AS12345",
					IPType:     "住宅家宽",
					OriginType: "原生IP",
					RiskScore:  5,
					FraudScore: 10,
				},
			},
		},
	}
	r2 := &history.TestRun{
		ID:          "run-2",
		AirportID:   "ap-1",
		AirportName: "Airport 1",
		CreatedAt:   time.Now(),
		PassedNodes: 1,
		TotalNodes:  1,
		Results: []*history.RunNodeResult{
			{
				ProxyName:         "US-01",
				CountryCode:       "US",
				LatencyMs:         140,
				DownloadSpeedMBps: 35.0,
				AntigravityStatus: "available",
				IPInfo: &history.IPInfo{
					IP:         "198.51.100.2",
					ASN:        "AS12345",
					IPType:     "住宅家宽",
					OriginType: "原生IP",
					RiskScore:  5,
					FraudScore: 10,
				},
			},
		},
	}
	_, _ = server.historyStore.Save(r1)
	_, _ = server.historyStore.Save(r2)

	// 7.1 GET /api/history/nodes
	respHistNodes, err := http.Get(url + "/api/history/nodes")
	if err != nil {
		t.Fatalf("GET /api/history/nodes failed: %v", err)
	}
	defer respHistNodes.Body.Close()
	var distinctNodes []string
	if err := json.NewDecoder(respHistNodes.Body).Decode(&distinctNodes); err != nil {
		t.Fatalf("decode distinct nodes: %v", err)
	}
	if len(distinctNodes) != 1 || distinctNodes[0] != "US-01" {
		t.Fatalf("expected ['US-01'], got %v", distinctNodes)
	}

	// 7.2 GET /api/history/node-timeline?name=US-01
	respTimeline, err := http.Get(url + "/api/history/node-timeline?name=US-01")
	if err != nil {
		t.Fatalf("GET timeline failed: %v", err)
	}
	defer respTimeline.Body.Close()
	var timelineItems []*history.NodeTimelineItem
	if err := json.NewDecoder(respTimeline.Body).Decode(&timelineItems); err != nil {
		t.Fatalf("decode timeline items: %v", err)
	}
	if len(timelineItems) != 2 {
		t.Fatalf("expected 2 timeline items, got %d", len(timelineItems))
	}

	// 7.3 GET /api/history/compare?base_id=run-1&target_id=run-2
	respCompare, err := http.Get(url + "/api/history/compare?base_id=run-1&target_id=run-2")
	if err != nil {
		t.Fatalf("GET compare failed: %v", err)
	}
	defer respCompare.Body.Close()
	var compResult history.RunComparison
	if err := json.NewDecoder(respCompare.Body).Decode(&compResult); err != nil {
		t.Fatalf("decode comparison result: %v", err)
	}
	if compResult.Summary.TotalCompared != 1 || compResult.Summary.ImprovedCount != 1 {
		t.Fatalf("unexpected comparison summary: %+v", compResult.Summary)
	}

	// 7.4 GET /api/history/airports
	respAirportsHist, err := http.Get(url + "/api/history/airports")
	if err != nil {
		t.Fatalf("GET /api/history/airports failed: %v", err)
	}
	defer respAirportsHist.Body.Close()
	var airportsList []*history.AirportSummary
	if err := json.NewDecoder(respAirportsHist.Body).Decode(&airportsList); err != nil {
		t.Fatalf("decode airports list: %v", err)
	}
	if len(airportsList) != 1 || airportsList[0].AirportID != "ap-1" {
		t.Fatalf("expected ap-1 in airport list, got %v", airportsList[0])
	}

	// 7.5 GET /api/history/airport?airport=ap-1
	respAirportHist, err := http.Get(url + "/api/history/airport?airport=ap-1")
	if err != nil {
		t.Fatalf("GET /api/history/airport failed: %v", err)
	}
	defer respAirportHist.Body.Close()
	var airportHist history.AirportHistory
	if err := json.NewDecoder(respAirportHist.Body).Decode(&airportHist); err != nil {
		t.Fatalf("decode airport history: %v", err)
	}
	if airportHist.TotalRuns != 2 || len(airportHist.Moments) != 2 || len(airportHist.Nodes) != 1 {
		t.Fatalf("unexpected airport history: runs=%d, moments=%d, nodes=%d", airportHist.TotalRuns, len(airportHist.Moments), len(airportHist.Nodes))
	}
	if airportHist.Nodes[0].TotalTests != 2 || airportHist.Nodes[0].MinLatencyMs != 140 {
		t.Fatalf("unexpected node history metrics: tests=%d, minLat=%d", airportHist.Nodes[0].TotalTests, airportHist.Nodes[0].MinLatencyMs)
	}
}
