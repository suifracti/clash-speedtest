package application

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/appdata"
	"github.com/faceair/clash-speedtest/core/autopick"
	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/profiles"
	"github.com/faceair/clash-speedtest/core/publicservice"
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
		{Node: monitor.MonitoredNode{DisplayName: "新加坡 01", NodeKey: "k1", NodeIdentityKey: "i1", ConfigRevisionKey: "r1", Type: "ss", RawConfig: map[string]any{"name": "新加坡 01", "type": "ss", "server": "1.1.1.1", "port": 8388, "cipher": "aes-128-gcm", "password": "pw"}}, ProfileID: "p1", AirportID: "airport-1", AirportName: "机场A"},
		{Node: monitor.MonitoredNode{DisplayName: "新加坡 02", NodeKey: "k2", NodeIdentityKey: "i2", ConfigRevisionKey: "r2", Type: "ss", RawConfig: map[string]any{"name": "新加坡 02", "type": "ss", "server": "1.1.1.2", "port": 8388, "cipher": "aes-128-gcm", "password": "pw"}}, ProfileID: "p1", AirportID: "airport-1", AirportName: "机场A"},
	}
}

func autoPickFixtureWithHistory(t *testing.T) *AppService {
	t.Helper()
	svc := autoPickFixture(t)
	store, err := history.NewStore(filepath.Join(t.TempDir(), "history"))
	if err != nil {
		t.Fatalf("history store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	svc.historyStore = store
	return svc
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

	// Report names use milliseconds plus a random ID; keep the rounds in distinct milliseconds.
	time.Sleep(2 * time.Millisecond)

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

// 解锁没通过只影响用途组：节点仍然留在“日常”。
func TestAutoPickKeepsUnlockFailuresInDailyGroup(t *testing.T) {
	svc := autoPickFixture(t)
	cfg, err := svc.LoadAutoPickConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.IncludeAI = true
	cfg.Normalize()
	if err := svc.saveAutoPickConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	nodes := autoPickFakeNodes()
	svc.autoPick().hooks.resolveNodes = func(autopick.Config) ([]autoPickScopedNode, error) { return nodes, nil }
	svc.autoPick().hooks.measureLatency = func(_ context.Context, _ string, _ []autoPickScopedNode, _ autopick.Config) (string, map[string]autoPickLatency, error) {
		return "batch-1", map[string]autoPickLatency{
			autoPickNodeKey(nodes[0]): {LatencyMS: 120, ResponseRate: 100, Samples: 6, Success: 6},
			autoPickNodeKey(nodes[1]): {LatencyMS: 240, ResponseRate: 100, Samples: 6, Success: 6},
		}, nil
	}
	// 01 两项都通；02 的 Claude 结论是“需人机验证”，按新规则算没通过。
	svc.autoPick().hooks.checkService = func(_ context.Context, node monitor.MonitoredNode, rule publicservice.Rule) publicservice.Result {
		if node.NodeKey == "k1" {
			return publicservice.Result{Outcome: "unlocked", RequestCount: 1, Summary: "可用"}
		}
		if rule.ServiceID == "claude_unlock" {
			return publicservice.Result{Outcome: "verification_required", RequestCount: 1, Summary: "需人机验证"}
		}
		return publicservice.Result{Outcome: "unlocked", RequestCount: 1, Summary: "可用"}
	}

	svc.runAutoPickRound("manual")

	report := svc.latestAutoPickReport()
	if report == nil {
		t.Fatal("no report")
	}
	if report.SelectedCount != 2 || report.GroupCounts[autopick.PurposeDaily] != 2 {
		t.Fatalf("daily must keep both nodes: %+v", report.GroupCounts)
	}
	if report.GroupCounts[autopick.PurposeAI] != 1 {
		t.Fatalf("ai group must only take the fully passing node: %+v", report.GroupCounts)
	}
	for _, node := range report.Nodes {
		if node.NodeKey == "k2" && len(node.Purposes) != 1 {
			t.Fatalf("failed unlock must not exclude the node: %+v", node)
		}
	}
	// 落盘报告里的用途必须和分组一致：检测计划持有 report.Nodes 元素的指针，
	// 追加节点若发生扩容，前面节点的 AI 用途会写进旧数组而丢失。
	rawReport, err := os.ReadFile(filepath.Join(svc.autoPickReportsPath(), autoPickReportFileName(report)))
	if err != nil {
		t.Fatalf("report file: %v", err)
	}
	var saved autopick.Report
	if err := json.Unmarshal(rawReport, &saved); err != nil {
		t.Fatalf("report json: %v", err)
	}
	if len(saved.Nodes) != 2 || saved.Nodes[0].NodeKey != "k1" {
		t.Fatalf("report nodes: %+v", saved.Nodes)
	}
	if !slices.Contains(saved.Nodes[0].Purposes, autopick.PurposeAI) {
		t.Fatalf("first node must carry the AI purpose on disk: %+v", saved.Nodes[0])
	}
	raw, err := os.ReadFile(svc.autoPickSubscriptionPath())
	if err != nil {
		t.Fatalf("subscription: %v", err)
	}
	if !strings.Contains(string(raw), "name: "+autopick.PurposeAI) {
		t.Fatalf("ai group missing from subscription:\n%s", raw)
	}
}

// 运行期间保存的新设置不能被结束时的旧配置覆盖；本轮报告仍按启动时的规则记录。
func TestAutoPickKeepsSettingsSavedDuringRun(t *testing.T) {
	svc := autoPickFixture(t)
	cfg, err := svc.LoadAutoPickConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Enabled = true
	cfg.RegionFilter = "新加坡"
	cfg.Normalize()
	if err := svc.saveAutoPickConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	nodes := autoPickFakeNodes()
	svc.autoPick().hooks.resolveNodes = func(autopick.Config) ([]autoPickScopedNode, error) { return nodes, nil }
	svc.autoPick().hooks.measureLatency = func(_ context.Context, _ string, _ []autoPickScopedNode, _ autopick.Config) (string, map[string]autoPickLatency, error) {
		// 模拟用户在运行中保存：关闭自动运行、换地区、提高阈值。
		next, err := svc.LoadAutoPickConfig()
		if err != nil {
			t.Fatalf("reload config: %v", err)
		}
		next.Enabled = false
		next.RegionFilter = "日本"
		next.MinResponseRate = 95
		next.MaxLatencyMS = 500
		next.NextRunAt = nil
		if err := svc.saveAutoPickConfig(next); err != nil {
			t.Fatalf("save config during run: %v", err)
		}
		return "batch-1", map[string]autoPickLatency{
			autoPickNodeKey(nodes[0]): {LatencyMS: 120, ResponseRate: 100, Samples: 6, Success: 6},
			autoPickNodeKey(nodes[1]): {LatencyMS: 240, ResponseRate: 100, Samples: 6, Success: 6},
		}, nil
	}

	svc.runAutoPickRound("manual")

	latest, err := svc.LoadAutoPickConfig()
	if err != nil {
		t.Fatalf("load latest config: %v", err)
	}
	if latest.Enabled || latest.NextRunAt != nil {
		t.Fatalf("turning auto pick off during the run must stick: %+v", latest)
	}
	if latest.RegionFilter != "日本" || latest.MinResponseRate != 95 || latest.MaxLatencyMS != 500 {
		t.Fatalf("settings saved during the run were overwritten: %+v", latest)
	}
	if latest.LastRunAt == nil {
		t.Fatal("last run time must still be recorded")
	}
	report := svc.latestAutoPickReport()
	if report == nil {
		t.Fatal("no report")
	}
	if report.Rules.MinResponseRate != 90 || report.Rules.MaxLatencyMS != 800 || report.Scope.RegionFilter != "新加坡" {
		t.Fatalf("report must keep the rules of the running round: rules=%+v scope=%+v", report.Rules, report.Scope)
	}
}

func TestAutoPickStableLatencyDropsTheFirstSample(t *testing.T) {
	samples := []WorkbenchLatencySampleDTO{
		{Seq: 0, LatencyMs: 900, Success: true},
		{Seq: 1, LatencyMs: 100, Success: true},
		{Seq: 2, LatencyMs: 120, Success: true},
		{Seq: 3, LatencyMs: 110, Success: true},
	}
	value, ok := autoPickStableLatency(samples)
	if !ok || value != 110 {
		t.Fatalf("stable latency: %d ok=%v, want 110", value, ok)
	}
	if _, ok := autoPickStableLatency(samples[:1]); ok {
		t.Fatal("a single sample must not produce a stable latency")
	}
	onlyFirst := []WorkbenchLatencySampleDTO{{Seq: 0, LatencyMs: 80, Success: true}, {Seq: 1, LatencyMs: 0, Success: false}}
	if _, ok := autoPickStableLatency(onlyFirst); ok {
		t.Fatal("when only the first sample succeeded there is no stable value")
	}
	measure := autoPickLatencyFromBatch(&WorkbenchLatencyBatchDTO{Items: []WorkbenchLatencyBatchItemDTO{{
		ProfileID: "p1", NodeKey: "k1", NodeIdentityKey: "i1", ConfigRevisionKey: "r1",
		Result: &WorkbenchLatencyTestDTO{TotalSamples: 4, SuccessSamples: 4, LatencyMs: 900, Samples: samples},
	}}})
	if got := measure["p1\x00k1\x00i1\x00r1"]; got.LatencyMS != 110 || got.ResponseRate != 100 {
		t.Fatalf("batch projection: %+v", got)
	}
}

// 每轮的延迟与解锁结果都进历史库，轮次来源是“自动精选”。
func TestAutoPickWritesRoundsWithAutoPickTrigger(t *testing.T) {
	svc := autoPickFixtureWithHistory(t)
	cfg, err := svc.LoadAutoPickConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.IncludeAI = true
	cfg.Normalize()
	if err := svc.saveAutoPickConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	nodes := autoPickFakeNodes()
	svc.autoPick().hooks.resolveNodes = func(autopick.Config) ([]autoPickScopedNode, error) { return nodes, nil }
	svc.autoPick().hooks.measureLatency = func(_ context.Context, _ string, _ []autoPickScopedNode, _ autopick.Config) (string, map[string]autoPickLatency, error) {
		return "batch-1", map[string]autoPickLatency{
			autoPickNodeKey(nodes[0]): {LatencyMS: 120, ResponseRate: 100, Samples: 6, Success: 6},
		}, nil
	}
	checks := map[string]int{}
	svc.autoPick().hooks.checkService = func(_ context.Context, _ monitor.MonitoredNode, rule publicservice.Rule) publicservice.Result {
		checks[rule.ServiceID]++
		return publicservice.Result{Outcome: "unlocked", RequestCount: 1, Summary: "可用", StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC()}
	}

	svc.runAutoPickRound("manual")

	report := svc.latestAutoPickReport()
	if report == nil {
		t.Fatal("no report")
	}
	rounds, err := svc.ListMeasurementRoundNodes(context.Background(), nodes[0].ProfileID, nodes[0].Node.NodeKey, nodes[0].Node.NodeIdentityKey, nodes[0].Node.ConfigRevisionKey, 10)
	if err != nil {
		t.Fatalf("rounds: %v", err)
	}
	var serviceRound *history.MeasurementRound
	for i := range rounds {
		if rounds[i].TriggerType != autoPickRoundTrigger {
			t.Fatalf("round trigger: %q", rounds[i].TriggerType)
		}
		for _, item := range rounds[i].Items {
			if item.Project == "service" {
				serviceRound = &rounds[i]
			}
		}
	}
	if serviceRound == nil {
		t.Fatalf("no service round in history: %+v", rounds)
	}
	if serviceRound.State != "finished" {
		t.Fatalf("round state: %q", serviceRound.State)
	}
	for _, item := range serviceRound.Items {
		if item.Service == nil || item.Service.Result == nil || item.Service.Result.Outcome != "unlocked" {
			t.Fatalf("service result not committed: %+v", item.Service)
		}
		if item.Service.PersistenceState != "saved" {
			t.Fatalf("service persistence: %q", item.Service.PersistenceState)
		}
	}

	// 信誉结果与解锁共用历史；24小时内重跑精选不再查询这三家。
	svc.runAutoPickRound("manual")
	for _, id := range []string{"ippure_ip_quality", "ping0_ipv4", "ipok_ipv4"} {
		if checks[id] != 1 {
			t.Fatalf("%s checks=%d, want one query across both rounds", id, checks[id])
		}
	}

	// 延迟批次走的是冻结计划：带自动精选标记时才允许执行自己的项。
	ctx := context.WithValue(context.Background(), autoPickRoundContextKey{}, autoPickRoundTrigger)
	req := WorkbenchLatencyBatchRequest{RequestID: "auto-pick:round-1", TestProject: WorkbenchLatencyProject}
	selections := []WorkbenchLatencyBatchSelection{{ProfileID: nodes[0].ProfileID, NodeKey: nodes[0].Node.NodeKey, NodeIdentityKey: nodes[0].Node.NodeIdentityKey, ConfigRevisionKey: nodes[0].Node.ConfigRevisionKey}}
	if err := svc.ensureManualLatencyRound(ctx, req, selections); err != nil {
		t.Fatalf("ensure round: %v", err)
	}
	trigger, err := svc.historyStore.DB().MeasurementRequestTrigger(context.Background(), req.RequestID, nodes[0].ProfileID, nodes[0].Node.NodeIdentityKey, nodes[0].Node.ConfigRevisionKey, "latency", "")
	if err != nil || trigger != autoPickRoundTrigger {
		t.Fatalf("latency round trigger: %q err=%v", trigger, err)
	}
	if err := svc.validateMeasurementRequest(ctx, req.RequestID, nodes[0].ProfileID, nodes[0].Node.NodeKey, nodes[0].Node.NodeIdentityKey, nodes[0].Node.ConfigRevisionKey, "latency", ""); err != nil {
		t.Fatalf("auto-pick ctx must execute its own plan: %v", err)
	}
	if err := svc.validateMeasurementRequest(context.Background(), req.RequestID, nodes[0].ProfileID, nodes[0].Node.NodeKey, nodes[0].Node.NodeIdentityKey, nodes[0].Node.ConfigRevisionKey, "latency", ""); err == nil {
		t.Fatal("a plain context must not execute an auto-pick item")
	}
}
