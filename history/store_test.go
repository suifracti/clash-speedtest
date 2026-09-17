package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHistoryStore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "history-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := NewStore(tempDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// 1. Test Save & Get
	run1 := &TestRun{
		AirportID:   "ap1",
		AirportName: "Airport One",
		CreatedAt:   time.Now().Add(-1 * time.Hour),
		TotalNodes:  2,
		PassedNodes: 2,
		Results: []*RunNodeResult{
			{
				ProxyName:         "HK-01",
				CountryCode:       "HK",
				CountryFlag:       "🇭🇰",
				LatencyMs:         100,
				DownloadSpeedMBps: 20.5,
				AntigravityStatus: "blocked",
			},
			{
				ProxyName:         "US-01",
				CountryCode:       "US",
				CountryFlag:       "🇺🇸",
				LatencyMs:         180,
				DownloadSpeedMBps: 30.0,
				AntigravityStatus: "available",
			},
		},
	}

	id1, err := store.Save(run1)
	if err != nil {
		t.Fatalf("Save run1 failed: %v", err)
	}
	if id1 == "" {
		t.Fatal("expected non-empty run ID")
	}

	loaded1, err := store.Get(id1)
	if err != nil {
		t.Fatalf("Get run1 failed: %v", err)
	}
	if loaded1.AirportName != "Airport One" || len(loaded1.Results) != 2 {
		t.Fatalf("loaded1 mismatch: %+v", loaded1)
	}

	// 2. Test Save second run
	run2 := &TestRun{
		AirportID:   "ap1",
		AirportName: "Airport One",
		CreatedAt:   time.Now(),
		TotalNodes:  3,
		PassedNodes: 3,
		Results: []*RunNodeResult{
			{
				ProxyName:         "HK-01",
				CountryCode:       "HK",
				CountryFlag:       "🇭🇰",
				LatencyMs:         60, // Improved (-40ms)
				DownloadSpeedMBps: 45.0, // Improved (+24.5 MB/s)
				AntigravityStatus: "available", // Improved (blocked -> available)
			},
			{
				ProxyName:         "US-01",
				CountryCode:       "US",
				CountryFlag:       "🇺🇸",
				LatencyMs:         240, // Degraded (+60ms)
				DownloadSpeedMBps: 10.0, // Degraded (-20 MB/s)
				AntigravityStatus: "blocked", // Degraded
			},
			{
				ProxyName:         "SG-01",
				CountryCode:       "SG",
				CountryFlag:       "🇸🇬",
				LatencyMs:         75,
				DownloadSpeedMBps: 50.0,
				AntigravityStatus: "available",
			},
		},
	}

	id2, err := store.Save(run2)
	if err != nil {
		t.Fatalf("Save run2 failed: %v", err)
	}

	// 3. Test List
	list, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 runs in list, got %d", len(list))
	}
	// Verify sorting (run2 is newer than run1)
	if list[0].ID != id2 || list[1].ID != id1 {
		t.Fatalf("expected list[0]=%s, list[1]=%s, got %s, %s", id2, id1, list[0].ID, list[1].ID)
	}

	// 4. Test Compare (id1 as base, id2 as target)
	comp, err := store.Compare(id1, id2)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}
	if comp.Summary.TotalCompared != 2 {
		t.Fatalf("expected 2 nodes compared, got %d", comp.Summary.TotalCompared)
	}
	if comp.Summary.NewCount != 1 {
		t.Fatalf("expected 1 new node (SG-01), got %d", comp.Summary.NewCount)
	}
	if comp.Summary.ImprovedCount != 1 {
		t.Fatalf("expected 1 improved node (HK-01), got %d", comp.Summary.ImprovedCount)
	}
	if comp.Summary.DegradedCount != 1 {
		t.Fatalf("expected 1 degraded node (US-01), got %d", comp.Summary.DegradedCount)
	}

	// 5. Test Delete
	if err := store.Delete(id1); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	listAfter, err := store.List()
	if err != nil {
		t.Fatalf("List after delete failed: %v", err)
	}
	if len(listAfter) != 1 {
		t.Fatalf("expected 1 run remaining, got %d", len(listAfter))
	}

	// Ensure file is removed
	if _, err := os.Stat(filepath.Join(tempDir, id1+".json")); !os.IsNotExist(err) {
		t.Fatalf("expected file to be gone, got err: %v", err)
	}
}

func TestGenerateAndSaveHTMLReport(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "report-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	now := time.Now()
	run := &TestRun{
		AirportID:   "ap-test",
		AirportName: "Test Airport",
		CreatedAt:   now,
		TotalNodes:  1,
		PassedNodes: 1,
		Results: []*RunNodeResult{
			{
				ProxyName:         "US-Premium-01",
				ProxyType:         "hysteria2",
				Server:            "1.2.3.4",
				Port:              443,
				CountryCode:       "US",
				CountryFlag:       "🇺🇸",
				LatencyMs:         85,
				JitterMs:          4,
				PacketLoss:        16.7,
				TimeoutCount:      1,
				DownloadSpeedMBps: 45.2,
				AntigravityStatus: "available",
				LatencySamples: []LatencySample{
					{Seq: 1, Timestamp: now, LatencyMs: 82, Success: true},
					{Seq: 2, Timestamp: now.Add(200 * time.Millisecond), LatencyMs: 88, Success: true},
					{Seq: 3, Timestamp: now.Add(400 * time.Millisecond), LatencyMs: 0, Success: false, Error: "Client.Timeout exceeded"},
				},
			},
		},
	}

	html, err := GenerateHTMLReport([]*TestRun{run})
	if err != nil {
		t.Fatalf("GenerateHTMLReport failed: %v", err)
	}

	if len(html) < 100 {
		t.Fatalf("expected substantial html output, got len %d", len(html))
	}
	if !strings.Contains(html, "US-Premium-01") {
		t.Fatalf("expected report to contain proxy name")
	}
	if !strings.Contains(html, "Client.Timeout exceeded") {
		t.Fatalf("expected report to contain timeout sample error")
	}

	reportFile := filepath.Join(tempDir, "report.html")
	if err := SaveHTMLReport([]*TestRun{run}, reportFile); err != nil {
		t.Fatalf("SaveHTMLReport failed: %v", err)
	}

	content, err := os.ReadFile(reportFile)
	if err != nil {
		t.Fatalf("failed to read report file: %v", err)
	}
	if len(content) == 0 {
		t.Fatal("saved report file is empty")
	}
}

func TestGetNodeTimelineAndListNodes(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "timeline-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := NewStore(tempDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	t1 := time.Now().Add(-2 * time.Hour)
	run1 := &TestRun{
		AirportID:   "ap-1",
		AirportName: "Airport One",
		CreatedAt:   t1,
		TotalNodes:  2,
		PassedNodes: 2,
		Results: []*RunNodeResult{
			{
				ProxyName:         "Node-A",
				LatencyMs:         100,
				DownloadSpeedMBps: 20.0,
				AntigravityStatus: "blocked",
				IPInfo: &IPInfo{
					IP:        "1.1.1.1",
					ASN:       "AS13335",
					IPType:    "机房IDC",
					RiskScore: 30,
				},
			},
			{
				ProxyName:         "Node-B",
				LatencyMs:         150,
				DownloadSpeedMBps: 10.0,
			},
		},
	}
	if _, err := store.Save(run1); err != nil {
		t.Fatalf("Save run1 failed: %v", err)
	}

	t2 := time.Now().Add(-1 * time.Hour)
	run2 := &TestRun{
		AirportID:   "ap-1",
		AirportName: "Airport One",
		CreatedAt:   t2,
		TotalNodes:  2,
		PassedNodes: 2,
		Results: []*RunNodeResult{
			{
				ProxyName:         "Node-A",
				LatencyMs:         80,
				DownloadSpeedMBps: 35.0,
				AntigravityStatus: "available",
				Stability: &StabilityInfo{
					TotalProbes:   3,
					SuccessProbes: 3,
					StabilityRate: 100.0,
					Flapping:      false,
					ExitIPs:       []string{"1.1.1.2"},
				},
			},
		},
	}
	if _, err := store.Save(run2); err != nil {
		t.Fatalf("Save run2 failed: %v", err)
	}

	// Test ListAllDistinctNodes
	nodes, err := store.ListAllDistinctNodes()
	if err != nil {
		t.Fatalf("ListAllDistinctNodes failed: %v", err)
	}
	if len(nodes) != 2 || nodes[0] != "Node-A" || nodes[1] != "Node-B" {
		t.Fatalf("unexpected distinct nodes: %v", nodes)
	}

	// Test GetNodeTimeline
	timeline, err := store.GetNodeTimeline("Node-A")
	if err != nil {
		t.Fatalf("GetNodeTimeline failed: %v", err)
	}
	if len(timeline) != 2 {
		t.Fatalf("expected 2 timeline entries for Node-A, got %d", len(timeline))
	}
	if timeline[0].LatencyMs != 100 || timeline[1].LatencyMs != 80 {
		t.Fatalf("expected chronological order (100ms then 80ms), got %d then %d", timeline[0].LatencyMs, timeline[1].LatencyMs)
	}
	if timeline[0].AntigravityStatus != "blocked" || timeline[1].AntigravityStatus != "available" {
		t.Fatalf("unexpected antigravity status progression: %s -> %s", timeline[0].AntigravityStatus, timeline[1].AntigravityStatus)
	}
}

func TestGetAirportHistoryAndListAirports(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "airport-history-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := NewStore(tempDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	t1 := time.Now().Add(-3 * time.Hour)
	t2 := time.Now().Add(-2 * time.Hour)
	t3 := time.Now().Add(-1 * time.Hour)

	run1 := &TestRun{
		AirportID:   "ap-liangxin",
		AirportName: "良心云",
		CreatedAt:   t1,
		TotalNodes:  2,
		PassedNodes: 2,
		Results: []*RunNodeResult{
			{
				ProxyName:         "HK-01",
				ProxyType:         "ss",
				CountryCode:       "HK",
				CountryFlag:       "🇭🇰",
				LatencyMs:         150,
				DownloadSpeedMBps: 20.0,
				AntigravityStatus: "available",
				IPInfo: &IPInfo{
					IP: "1.1.1.1",
				},
			},
			{
				ProxyName:         "TW-01",
				ProxyType:         "vless",
				CountryCode:       "TW",
				CountryFlag:       "🇹🇼",
				LatencyMs:         200,
				DownloadSpeedMBps: 15.0,
				AntigravityStatus: "blocked",
			},
		},
	}
	run2 := &TestRun{
		AirportID:   "ap-liangxin",
		AirportName: "良心云",
		CreatedAt:   t2,
		TotalNodes:  1,
		PassedNodes: 1,
		Results: []*RunNodeResult{
			{
				ProxyName:         "HK-01",
				ProxyType:         "ss",
				CountryCode:       "HK",
				CountryFlag:       "🇭🇰",
				LatencyMs:         120,
				DownloadSpeedMBps: 35.0,
				AntigravityStatus: "available",
				IPInfo: &IPInfo{
					IP: "1.1.1.2",
				},
			},
		},
	}
	run3 := &TestRun{
		AirportID:   "ap-other",
		AirportName: "其他机场",
		CreatedAt:   t3,
		TotalNodes:  1,
		PassedNodes: 1,
		Results: []*RunNodeResult{
			{
				ProxyName:         "SG-01",
				ProxyType:         "trojan",
				CountryCode:       "SG",
				CountryFlag:       "🇸🇬",
				LatencyMs:         80,
				DownloadSpeedMBps: 50.0,
				AntigravityStatus: "available",
			},
		},
	}

	for _, r := range []*TestRun{run1, run2, run3} {
		if _, err := store.Save(r); err != nil {
			t.Fatalf("Save run failed: %v", err)
		}
	}

	// 1. Test ListHistoryAirports
	airports, err := store.ListHistoryAirports()
	if err != nil {
		t.Fatalf("ListHistoryAirports failed: %v", err)
	}
	if len(airports) != 2 {
		t.Fatalf("expected 2 airports, got %d", len(airports))
	}
	var lx *AirportSummary
	for _, a := range airports {
		if a.AirportID == "ap-liangxin" {
			lx = a
			break
		}
	}
	if lx == nil {
		t.Fatal("ap-liangxin airport summary not found")
	}
	if lx.TotalRuns != 2 {
		t.Fatalf("expected 2 runs for ap-liangxin, got %d", lx.TotalRuns)
	}
	if lx.DistinctNodeCount != 2 {
		t.Fatalf("expected 2 distinct nodes for ap-liangxin, got %d", lx.DistinctNodeCount)
	}
	if lx.MaxSpeedMBps != 35.0 {
		t.Fatalf("expected max speed 35.0 for ap-liangxin, got %f", lx.MaxSpeedMBps)
	}

	// 2. Test GetAirportHistory
	hist, err := store.GetAirportHistory("ap-liangxin")
	if err != nil {
		t.Fatalf("GetAirportHistory failed: %v", err)
	}
	if hist.TotalRuns != 2 {
		t.Fatalf("expected 2 runs in airport history, got %d", hist.TotalRuns)
	}
	if len(hist.Moments) != 2 {
		t.Fatalf("expected 2 moments, got %d", len(hist.Moments))
	}
	if len(hist.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(hist.Nodes))
	}

	var hkNode *AirportNodeHistory
	for _, n := range hist.Nodes {
		if n.ProxyName == "HK-01" {
			hkNode = n
			break
		}
	}
	if hkNode == nil {
		t.Fatal("HK-01 node history not found")
	}
	if hkNode.TotalTests != 2 {
		t.Fatalf("expected HK-01 total tests 2, got %d", hkNode.TotalTests)
	}
	if len(hkNode.Points) != 2 {
		t.Fatalf("expected HK-01 points 2, got %d", len(hkNode.Points))
	}
	if hkNode.MinLatencyMs != 120 || hkNode.MaxLatencyMs != 150 {
		t.Fatalf("expected HK-01 min=120, max=150, got min=%d max=%d", hkNode.MinLatencyMs, hkNode.MaxLatencyMs)
	}
	if hkNode.AvgLatencyMs != 135 {
		t.Fatalf("expected HK-01 avg latency 135, got %d", hkNode.AvgLatencyMs)
	}
	if hkNode.LatencyChangeMs != -30 {
		t.Fatalf("expected HK-01 latency change -30, got %d", hkNode.LatencyChangeMs)
	}
	if hkNode.AvailableRate != 100.0 {
		t.Fatalf("expected HK-01 available rate 100.0, got %f", hkNode.AvailableRate)
	}
	if len(hkNode.UniqueIPs) != 2 {
		t.Fatalf("expected HK-01 2 unique IPs, got %d (%v)", len(hkNode.UniqueIPs), hkNode.UniqueIPs)
	}
}

