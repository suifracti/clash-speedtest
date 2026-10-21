package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faceair/clash-speedtest/core/appdata"
	"github.com/faceair/clash-speedtest/core/autopick"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/profiles"
	"gopkg.in/yaml.v3"
)

func autoPickFixture(t *testing.T) *AppService {
	t.Helper()
	root := t.TempDir()
	svc := &AppService{
		appPaths:     appdata.AppPaths{DataRoot: root, ProfileDir: filepath.Join(root, "profiles"), SettingsFile: filepath.Join(root, "settings.json")},
		profilePaths: profiles.Paths{Dir: filepath.Join(root, "profiles")},
	}
	cfg := autopick.Config{AirportIDs: []string{"airport-1"}, Schedule: autopick.Schedule6h, DailyHour: 6, MinResponseRate: 90, MaxLatencyMS: 800, SampleCount: 6}
	cfg.Normalize()
	if err := svc.saveAutoPickConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	return svc
}

func autoPickFakeNodes() []autoPickScopedNode {
	return []autoPickScopedNode{
		{Node: monitor.MonitoredNode{DisplayName: "新加坡 01", NodeKey: "k1", NodeIdentityKey: "i1", ConfigRevisionKey: "r1", Type: "ss", RawConfig: map[string]any{"name": "新加坡 01", "type": "ss", "server": "1.1.1.1", "port": 8388, "cipher": "aes-128-gcm", "password": "pw"}}, ProfileID: "p1", AirportID: "airport-1", AirportName: "良心云"},
		{Node: monitor.MonitoredNode{DisplayName: "新加坡 02", NodeKey: "k2", NodeIdentityKey: "i2", ConfigRevisionKey: "r2", Type: "ss", RawConfig: map[string]any{"name": "新加坡 02", "type": "ss", "server": "1.1.1.2", "port": 8388, "cipher": "aes-128-gcm", "password": "pw"}}, ProfileID: "p1", AirportID: "airport-1", AirportName: "良心云"},
	}
}

func TestAutoPickWritesSelectedNodesAndSubscription(t *testing.T) {
	svc := autoPickFixture(t)
	nodes := autoPickFakeNodes()
	svc.autoPick().hooks.resolveNodes = func(autopick.Config) ([]autoPickScopedNode, error) { return nodes, nil }
	svc.autoPick().hooks.measureLatency = func(_ context.Context, _ string, _ []autoPickScopedNode, _ autopick.Config) (string, map[string]autoPickLatency, error) {
		return "batch-1", map[string]autoPickLatency{
			autoPickNodeKey(nodes[0]): {LatencyMS: 120, ResponseRate: 100, Samples: 6, Success: 6},
			autoPickNodeKey(nodes[1]): {LatencyMS: 240, ResponseRate: 100, Samples: 6, Success: 6},
		}, nil
	}

	svc.runAutoPickRound("manual")

	report := svc.latestAutoPickReport()
	if report == nil || report.Status != autopick.StatusSelected || report.SelectedCount != 2 || report.GroupCounts[autopick.PurposeDaily] != 2 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if !strings.Contains(report.Summary, "选中 2 个") {
		t.Fatalf("summary: %q", report.Summary)
	}
	raw, err := os.ReadFile(svc.autoPickSubscriptionPath())
	if err != nil {
		t.Fatalf("subscription file: %v", err)
	}
	var config struct {
		Proxies     []map[string]any `yaml:"proxies"`
		ProxyGroups []struct {
			Name    string   `yaml:"name"`
			Proxies []string `yaml:"proxies"`
		} `yaml:"proxy-groups"`
	}
	if err := yaml.Unmarshal(raw, &config); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	if len(config.Proxies) != 2 {
		t.Fatalf("proxies: %d", len(config.Proxies))
	}
	var daily []string
	for _, group := range config.ProxyGroups {
		if group.Name == autopick.PurposeDaily {
			daily = group.Proxies
		}
	}
	if len(daily) != 2 {
		t.Fatalf("daily group: %+v", config.ProxyGroups)
	}
	reports, err := svc.ListAutoPickReports(10)
	if err != nil || len(reports) != 1 || reports[0].ReportID != report.ReportID {
		t.Fatalf("reports: %+v err=%v", reports, err)
	}
}

func TestAutoPickKeepsPreviousSubscriptionWhenNothingSelected(t *testing.T) {
	svc := autoPickFixture(t)
	nodes := autoPickFakeNodes()
	svc.autoPick().hooks.resolveNodes = func(autopick.Config) ([]autoPickScopedNode, error) { return nodes, nil }
	svc.autoPick().hooks.measureLatency = func(_ context.Context, _ string, _ []autoPickScopedNode, _ autopick.Config) (string, map[string]autoPickLatency, error) {
		return "batch-1", map[string]autoPickLatency{
			autoPickNodeKey(nodes[0]): {LatencyMS: 120, ResponseRate: 100, Samples: 6, Success: 6},
			autoPickNodeKey(nodes[1]): {LatencyMS: 240, ResponseRate: 100, Samples: 6, Success: 6},
		}, nil
	}
	svc.runAutoPickRound("manual")
	first := svc.latestAutoPickReport()
	before, err := os.ReadFile(svc.autoPickSubscriptionPath())
	if err != nil {
		t.Fatalf("first subscription: %v", err)
	}

	// Second round: every node loses too many samples.
	svc.autoPick().hooks.measureLatency = func(_ context.Context, _ string, _ []autoPickScopedNode, _ autopick.Config) (string, map[string]autoPickLatency, error) {
		return "batch-2", map[string]autoPickLatency{
			autoPickNodeKey(nodes[0]): {LatencyMS: 120, ResponseRate: 50, Samples: 6, Success: 3},
			autoPickNodeKey(nodes[1]): {LatencyMS: 240, ResponseRate: 50, Samples: 6, Success: 3},
		}, nil
	}
	svc.runAutoPickRound("manual")

	report := svc.latestAutoPickReport()
	if report == nil || report.SelectedCount != 0 || !report.KeptPrevious || report.PreviousReportID != first.ReportID {
		t.Fatalf("unexpected kept-previous report: %+v", report)
	}
	if report.Status != autopick.StatusKeptPrevious || !strings.Contains(report.Summary, "沿用上次结果") {
		t.Fatalf("status/summary: %+v", report)
	}
	after, err := os.ReadFile(svc.autoPickSubscriptionPath())
	if err != nil {
		t.Fatalf("subscription after failed round: %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("failed round must not rewrite the subscription file")
	}
}
