package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/faceair/clash-speedtest/core/autopick"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/profiles"
	"github.com/faceair/clash-speedtest/core/publicservice"
	"gopkg.in/yaml.v3"
)

const (
	autoPickDirName        = "auto-pick"
	autoPickConfigFile     = "config.json"
	autoPickReportsDir     = "reports"
	autoPickSubscription   = "subscription.yaml"
	autoPickTokenFile      = "subscription-token"
	autoPickPortFile       = "subscription-port"
	autoPickDefaultPort    = 17891
	autoPickMaxReports     = 50
	autoPickMaxNodes       = 200
	autoPickLatencyTimeout = 10 * time.Minute
)

// AutoPickEstimate is the request estimate shown before a run.
type AutoPickEstimate struct {
	Nodes    int  `json:"nodes"`
	Projects int  `json:"projects"`
	Requests int  `json:"requests"`
	Warn     bool `json:"warn"`
}

// AutoPickSubscription describes the local subscription artifact.
type AutoPickSubscription struct {
	URL       string     `json:"url"`
	Path      string     `json:"path"`
	Exists    bool       `json:"exists"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	NodeCount int        `json:"node_count"`
}

// AutoPickStatus is the read model of the auto-pick card.
type AutoPickStatus struct {
	Config       autopick.Config       `json:"config"`
	Running      bool                  `json:"running"`
	LastError    string                `json:"last_error,omitempty"`
	Estimate     AutoPickEstimate      `json:"estimate"`
	Subscription AutoPickSubscription  `json:"subscription"`
	LastReport   *autopick.Report      `json:"last_report,omitempty"`
	Reports      []autopick.ReportMeta `json:"reports,omitempty"`
}

type autoPickScopedNode struct {
	Node        monitor.MonitoredNode
	ProfileID   string
	AirportID   string
	AirportName string
}

type autoPickLatency struct {
	LatencyMS    int64
	ResponseRate float64
	Samples      int
	Success      int
}

type autoPickHooks struct {
	resolveNodes   func(cfg autopick.Config) ([]autoPickScopedNode, error)
	measureLatency func(ctx context.Context, reportID string, nodes []autoPickScopedNode, cfg autopick.Config) (string, map[string]autoPickLatency, error)
	checkService   func(ctx context.Context, node monitor.MonitoredNode, rule publicservice.Rule) publicservice.Result
}

type autoPickState struct {
	mu        sync.Mutex
	started   bool
	running   bool
	lastError string
	stop      chan struct{}
	wg        sync.WaitGroup
	server    *http.Server
	port      int
	hooks     autoPickHooks
	runWG     sync.WaitGroup
	cancelRun context.CancelFunc
}

func (s *AppService) autoPick() *autoPickState {
	if s.autoPickState == nil {
		s.autoPickState = &autoPickState{}
	}
	return s.autoPickState
}

func (s *AppService) autoPickRoot() string {
	root := strings.TrimSpace(s.appPaths.DataRoot)
	if root == "" {
		root = strings.TrimSpace(filepath.Dir(s.profilePaths.Dir))
	}
	if root == "" {
		root = "."
	}
	return filepath.Join(root, autoPickDirName)
}

func (s *AppService) autoPickConfigPath() string {
	return filepath.Join(s.autoPickRoot(), autoPickConfigFile)
}

func (s *AppService) autoPickReportsPath() string {
	return filepath.Join(s.autoPickRoot(), autoPickReportsDir)
}

func (s *AppService) autoPickSubscriptionPath() string {
	return filepath.Join(s.autoPickRoot(), autoPickSubscription)
}

// LoadAutoPickConfig reads the durable config; a missing file yields the
// default, disabled configuration.
func (s *AppService) LoadAutoPickConfig() (autopick.Config, error) {
	cfg := autopick.Config{Schedule: autopick.Schedule6h, DailyHour: 6, MinResponseRate: autopick.DefaultMinResponseRate, MaxLatencyMS: autopick.DefaultMaxLatencyMS, SampleCount: autopick.DefaultSampleCount}
	raw, err := os.ReadFile(s.autoPickConfigPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg.Normalize()
			return cfg, nil
		}
		return cfg, fmt.Errorf("读取自动精选配置失败: %w", err)
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("解析自动精选配置失败: %w", err)
	}
	cfg.Normalize()
	return cfg, nil
}

func writeJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *AppService) saveAutoPickConfig(cfg autopick.Config) error {
	cfg.UpdatedAt = time.Now().UTC()
	return writeJSONAtomic(s.autoPickConfigPath(), cfg)
}

// GetAutoPickStatus returns the card read model.
func (s *AppService) GetAutoPickStatus() (*AutoPickStatus, error) {
	cfg, err := s.LoadAutoPickConfig()
	if err != nil {
		return nil, err
	}
	nodes, resolveErr := s.resolveAutoPickNodes(cfg)
	if resolveErr != nil && len(cfg.AirportIDs) > 0 {
		// Scope resolution failure is reported on the card, not as a hard error.
		nodes = nil
	}
	state := s.autoPick()
	state.mu.Lock()
	running, lastError := state.running, state.lastError
	state.mu.Unlock()
	status := &AutoPickStatus{Config: cfg, Running: running, LastError: lastError}
	status.Estimate = autoPickEstimate(cfg, len(nodes))
	status.Subscription = s.autoPickSubscriptionStatus(cfg)
	if status.Subscription.Exists {
		s.startAutoPickSubscriptionServer()
		status.Subscription = s.autoPickSubscriptionStatus(cfg)
	}
	status.LastReport = s.latestAutoPickReport()
	return status, nil
}

func autoPickEstimate(cfg autopick.Config, nodes int) AutoPickEstimate {
	requests := cfg.EstimateRequests(nodes)
	return AutoPickEstimate{Nodes: nodes, Projects: cfg.Projects(), Requests: requests, Warn: requests > autopick.RequestWarningThreshold}
}

// SaveAutoPickConfig validates and persists the config, then recomputes the
// next run. It never enables a schedule on its own.
func (s *AppService) SaveAutoPickConfig(cfg autopick.Config) (*AutoPickStatus, error) {
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return nil, monitor.NewValidationError(err.Error())
	}
	previous, _ := s.LoadAutoPickConfig()
	if cfg.LastRunAt == nil {
		cfg.LastRunAt = previous.LastRunAt
	}
	cfg.NextRunAt = nil
	if cfg.Enabled {
		next := autopick.NextRunAfter(time.Now(), cfg)
		cfg.NextRunAt = &next
	}
	if err := s.saveAutoPickConfig(cfg); err != nil {
		return nil, fmt.Errorf("保存自动精选配置失败: %w", err)
	}
	s.startAutoPick()
	return s.GetAutoPickStatus()
}

// RunAutoPickNow starts one round in the background and returns immediately.
func (s *AppService) RunAutoPickNow() (*AutoPickStatus, error) {
	cfg, err := s.LoadAutoPickConfig()
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, monitor.NewValidationError(err.Error())
	}
	state := s.autoPick()
	state.mu.Lock()
	if state.running {
		state.mu.Unlock()
		return nil, monitor.NewValidationError("自动精选正在运行，请等待本轮结束")
	}
	state.running = true
	state.lastError = ""
	state.mu.Unlock()
	s.startAutoPick()
	s.startAutoPickRound("manual")
	return s.GetAutoPickStatus()
}

func (s *AppService) startAutoPickRound(trigger string) {
	state := s.autoPick()
	state.runWG.Add(1)
	go func() {
		defer state.runWG.Done()
		s.runAutoPickRound(trigger)
	}()
}

func (s *AppService) runAutoPickRound(trigger string) {
	state := s.autoPick()
	defer func() {
		state.mu.Lock()
		state.running = false
		state.mu.Unlock()
	}()
	startedAt := time.Now().UTC()
	ctx, cancel := context.WithCancel(context.Background())
	state.mu.Lock()
	state.cancelRun = cancel
	state.mu.Unlock()
	defer cancel()
	reportID := "ap-" + strconv.FormatInt(startedAt.Unix(), 10) + "-" + randomToken(3)
	cfg, err := s.LoadAutoPickConfig()
	if err != nil {
		s.finishAutoPickFailure(reportID, startedAt, trigger, cfg, err)
		return
	}
	report := &autopick.Report{
		ReportID: reportID, CreatedAt: startedAt, Trigger: trigger,
		Scope: autopick.Scope{AirportIDs: cfg.AirportIDs, RegionFilter: cfg.RegionFilter},
		Rules: autopick.Rules{MinResponseRate: cfg.MinResponseRate, MaxLatencyMS: cfg.MaxLatencyMS, SampleCount: cfg.SampleCount},
	}
	if cfg.IncludeAI {
		report.Rules.Groups = append(report.Rules.Groups, autopick.PurposeAI)
	}
	if cfg.IncludeStreaming {
		report.Rules.Groups = append(report.Rules.Groups, autopick.PurposeStreaming)
	}
	if len(report.Rules.Groups) > 0 {
		report.Rules.RequiredServices = map[string][]string{}
		for _, purpose := range report.Rules.Groups {
			report.Rules.RequiredServices[purpose] = cfg.RequiredServices(purpose)
		}
	}

	nodes, err := s.autoPickResolve(cfg)
	if err != nil {
		s.finishAutoPickFailure(reportID, startedAt, trigger, cfg, err)
		return
	}
	report.Scope.AirportNames = autoPickAirportNames(nodes)
	report.Measured = len(nodes)
	if len(nodes) == 0 {
		report.Notes = append(report.Notes, "范围内没有匹配的节点；未写入订阅文件")
		s.finishAutoPickReport(report, cfg)
		return
	}
	if len(nodes) > autoPickMaxNodes {
		s.finishAutoPickFailure(reportID, startedAt, trigger, cfg, fmt.Errorf("范围内节点 %d 个，超过单轮上限 %d；请缩小机场或地区范围", len(nodes), autoPickMaxNodes))
		return
	}

	batchID, latency, err := s.autoPickMeasureLatency(ctx, reportID, nodes, cfg)
	if err != nil {
		s.finishAutoPickFailure(reportID, startedAt, trigger, cfg, err)
		return
	}
	report.BatchID = batchID

	rules := make(map[string]publicservice.Rule)
	for _, rule := range publicservice.Catalog() {
		rules[rule.ServiceID] = rule
	}
	// 第一遍只看延迟：通过即进“日常”，解锁结果只决定是否再进用途组。
	var selected []autoPickScopedNode
	groups := map[string][]autoPickScopedNode{}
	groupCounts := map[string]int{}
	plans := make([]autoPickServicePlan, 0, len(nodes))
	for _, node := range nodes {
		entry := autopick.Node{
			DisplayName: node.Node.DisplayName, ProfileID: node.ProfileID, AirportID: node.AirportID, AirportName: node.AirportName,
			NodeKey: node.Node.NodeKey, NodeIdentityKey: node.Node.NodeIdentityKey, ConfigRevisionKey: node.Node.ConfigRevisionKey,
			Services: map[string]string{},
		}
		measure, ok := latency[autoPickNodeKey(node)]
		if !ok {
			entry.ExcludedReason = "本轮没有延迟结果"
			report.Nodes = append(report.Nodes, entry)
			continue
		}
		entry.LatencyMS, entry.ResponseRate, entry.Samples, entry.SuccessSamples = measure.LatencyMS, measure.ResponseRate, measure.Samples, measure.Success
		if measure.Samples == 0 {
			entry.ExcludedReason = "没有有效延迟样本"
			report.Nodes = append(report.Nodes, entry)
			continue
		}
		if measure.ResponseRate < cfg.MinResponseRate {
			entry.ExcludedReason = fmt.Sprintf("响应率 %.0f%% 低于 %.0f%%", measure.ResponseRate, cfg.MinResponseRate)
			report.Nodes = append(report.Nodes, entry)
			continue
		}
		if measure.LatencyMS <= 0 {
			entry.ExcludedReason = "没有稳定延迟样本（去掉首次后全部失败）"
			report.Nodes = append(report.Nodes, entry)
			continue
		}
		if measure.LatencyMS > int64(cfg.MaxLatencyMS) {
			entry.ExcludedReason = fmt.Sprintf("稳定延迟 %d ms 高于上限 %d ms", measure.LatencyMS, cfg.MaxLatencyMS)
			report.Nodes = append(report.Nodes, entry)
			continue
		}
		report.LatencyPassed++
		entry.Purposes = append(entry.Purposes, autopick.PurposeDaily)
		report.Nodes = append(report.Nodes, entry)
		index := len(report.Nodes) - 1
		selected = append(selected, node)
		for _, purpose := range report.Rules.Groups {
			plans = append(plans, autoPickServicePlan{
				entry: &report.Nodes[index], node: node, purpose: purpose,
				services: cfg.RequiredServices(purpose),
			})
		}
	}

	// 解锁只测必需服务；结论“需人机验证／无法判断”算没通过，但不影响日常组。
	if len(plans) > 0 {
		serviceRound := s.autoPickStartServiceRound(report, plans)
		for i := range plans {
			plan := &plans[i]
			passed := true
			for _, serviceID := range plan.services {
				rule, ok := rules[serviceID]
				if !ok {
					plan.entry.Services[serviceID] = "rule_missing"
					passed = false
					continue
				}
				result := s.autoPickCheckService(ctx, plan.node.Node, rule)
				plan.entry.Services[serviceID] = result.Outcome
				if serviceRound != nil {
					s.autoPickSaveServiceAttempt(ctx, report, serviceRound, plan.node, serviceID, rule, result)
				}
				if !autoPickServiceUsable(result.Outcome) {
					passed = false
				}
			}
			if passed {
				plan.entry.Purposes = append(plan.entry.Purposes, plan.purpose)
				groups[plan.purpose] = append(groups[plan.purpose], plan.node)
				groupCounts[plan.purpose]++
			}
		}
		s.autoPickFinishServiceRound(serviceRound)
	}
	groups[autopick.PurposeDaily] = selected
	groupCounts[autopick.PurposeDaily] = len(selected)

	report.SelectedCount = len(selected)
	report.GroupCounts = groupCounts
	if len(selected) == 0 {
		report.Notes = append(report.Notes, "本轮没有节点满足规则；未写入订阅文件")
		s.finishAutoPickReport(report, cfg)
		return
	}

	content, count, err := buildAutoPickClashYAML(groups, selected)
	if err != nil {
		s.finishAutoPickFailure(reportID, startedAt, trigger, cfg, err)
		return
	}
	if err := writeAutoPickFileAtomic(s.autoPickSubscriptionPath(), []byte(content)); err != nil {
		s.finishAutoPickFailure(reportID, startedAt, trigger, cfg, fmt.Errorf("写入订阅文件失败: %w", err))
		return
	}
	report.Subscription = s.autoPickSubscriptionPath()
	report.Notes = append(report.Notes, fmt.Sprintf("订阅文件已写入 %d 个节点", count))
	s.finishAutoPickReport(report, cfg)
}

func (s *AppService) finishAutoPickFailure(reportID string, startedAt time.Time, trigger string, cfg autopick.Config, runErr error) {
	state := s.autoPick()
	state.mu.Lock()
	state.lastError = runErr.Error()
	state.mu.Unlock()
	report := &autopick.Report{ReportID: reportID, CreatedAt: startedAt, Trigger: trigger, Status: autopick.StatusFailed,
		Scope: autopick.Scope{AirportIDs: cfg.AirportIDs, RegionFilter: cfg.RegionFilter},
		Rules: autopick.Rules{MinResponseRate: cfg.MinResponseRate, MaxLatencyMS: cfg.MaxLatencyMS, SampleCount: cfg.SampleCount},
		Notes: []string{"本轮失败：" + runErr.Error()}}
	s.finishAutoPickReport(report, cfg)
}

// finishAutoPickReport applies the keep-previous rule, persists the report and
// advances the schedule.
func (s *AppService) finishAutoPickReport(report *autopick.Report, cfg autopick.Config) {
	if report.Status == "" {
		report.Status = autopick.StatusSelected
	}
	emptyRound := report.SelectedCount == 0 && report.Status == autopick.StatusSelected
	if emptyRound || report.Status == autopick.StatusFailed {
		if previous := s.latestSuccessfulAutoPickReport(report.ReportID); previous != nil {
			report.KeptPrevious = true
			report.PreviousReportID = previous.ReportID
			at := previous.CreatedAt
			report.PreviousAt = &at
			if emptyRound {
				report.Status = autopick.StatusKeptPrevious
			}
			report.Notes = append(report.Notes, "沿用上次结果：订阅文件保持上一轮内容，未写空文件")
		} else if emptyRound {
			report.Status = autopick.StatusFailed
			report.Notes = append(report.Notes, "没有可沿用的上次结果，未写入订阅文件")
		} else {
			report.Notes = append(report.Notes, "没有可沿用的上次结果，未写入订阅文件")
		}
	}
	if report.GroupCounts == nil {
		report.GroupCounts = map[string]int{}
	}
	report.Summary = autopick.SummaryLine(*report)
	if err := writeJSONAtomic(filepath.Join(s.autoPickReportsPath(), autoPickReportFileName(report)), report); err != nil {
		state := s.autoPick()
		state.mu.Lock()
		state.lastError = "报告保存失败：" + err.Error()
		state.mu.Unlock()
	}
	s.pruneAutoPickReports()
	now := time.Now().UTC()
	cfg.LastRunAt = &now
	if cfg.Enabled {
		next := autopick.NextRunAfter(time.Now(), cfg)
		cfg.NextRunAt = &next
	}
	_ = s.saveAutoPickConfig(cfg)
}

func autoPickReportFileName(report *autopick.Report) string {
	return fmt.Sprintf("%013d-%s.json", report.CreatedAt.UnixMilli(), report.ReportID)
}

func (s *AppService) autoPickReportFiles() ([]string, error) {
	entries, err := os.ReadDir(s.autoPickReportsPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	return names, nil
}

func (s *AppService) pruneAutoPickReports() {
	names, err := s.autoPickReportFiles()
	if err != nil || len(names) <= autoPickMaxReports {
		return
	}
	for _, name := range names[autoPickMaxReports:] {
		_ = os.Remove(filepath.Join(s.autoPickReportsPath(), name))
	}
}

// ListAutoPickReports returns newest-first report metadata.
func (s *AppService) ListAutoPickReports(limit int) ([]autopick.ReportMeta, error) {
	if limit <= 0 || limit > autoPickMaxReports {
		limit = autoPickMaxReports
	}
	names, err := s.autoPickReportFiles()
	if err != nil {
		return nil, err
	}
	result := make([]autopick.ReportMeta, 0, limit)
	for _, name := range names {
		if len(result) >= limit {
			break
		}
		var report autopick.Report
		raw, err := os.ReadFile(filepath.Join(s.autoPickReportsPath(), name))
		if err != nil || json.Unmarshal(raw, &report) != nil {
			continue
		}
		result = append(result, report.Meta())
	}
	return result, nil
}

// GetAutoPickReport loads one full report.
func (s *AppService) GetAutoPickReport(reportID string) (*autopick.Report, error) {
	reportID = strings.TrimSpace(reportID)
	if reportID == "" {
		return nil, monitor.NewValidationError("报告标识不能为空")
	}
	names, err := s.autoPickReportFiles()
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		if !strings.Contains(name, reportID) {
			continue
		}
		var report autopick.Report
		raw, err := os.ReadFile(filepath.Join(s.autoPickReportsPath(), name))
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &report); err != nil {
			return nil, err
		}
		return &report, nil
	}
	return nil, monitor.NewValidationError("报告不存在或已被清理")
}

func (s *AppService) latestAutoPickReport() *autopick.Report {
	names, err := s.autoPickReportFiles()
	if err != nil || len(names) == 0 {
		return nil
	}
	var report autopick.Report
	raw, err := os.ReadFile(filepath.Join(s.autoPickReportsPath(), names[0]))
	if err != nil || json.Unmarshal(raw, &report) != nil {
		return nil
	}
	return &report
}

func (s *AppService) latestSuccessfulAutoPickReport(excludeID string) *autopick.Report {
	names, err := s.autoPickReportFiles()
	if err != nil {
		return nil
	}
	for _, name := range names {
		var report autopick.Report
		raw, err := os.ReadFile(filepath.Join(s.autoPickReportsPath(), name))
		if err != nil || json.Unmarshal(raw, &report) != nil {
			continue
		}
		if report.ReportID == excludeID {
			continue
		}
		if report.Status == autopick.StatusSelected && report.SelectedCount > 0 {
			return &report
		}
	}
	return nil
}

// --- scope resolution ---

func (s *AppService) resolveAutoPickNodes(cfg autopick.Config) ([]autoPickScopedNode, error) {
	if s == nil {
		return nil, fmt.Errorf("app service is not initialized")
	}
	if len(cfg.AirportIDs) == 0 {
		return nil, nil
	}
	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil || store == nil {
		return nil, fmt.Errorf("读取机场列表失败: %w", err)
	}
	wanted := make(map[string]bool, len(cfg.AirportIDs))
	for _, id := range cfg.AirportIDs {
		wanted[id] = true
	}
	region := strings.ToLower(strings.TrimSpace(cfg.RegionFilter))
	var result []autoPickScopedNode
	seen := map[string]bool{}
	for _, airport := range store.Airports {
		if airport == nil || !wanted[airport.ID] {
			continue
		}
		appendNodes := func(profileID string, nodes []monitor.MonitoredNode) {
			for _, node := range nodes {
				if region != "" && !strings.Contains(strings.ToLower(node.DisplayName), region) {
					continue
				}
				key := profileID + "\x00" + node.NodeKey + "\x00" + node.NodeIdentityKey + "\x00" + node.ConfigRevisionKey
				if seen[key] {
					continue
				}
				seen[key] = true
				result = append(result, autoPickScopedNode{Node: node, ProfileID: profileID, AirportID: airport.ID, AirportName: airport.Name})
			}
		}
		loaded := false
		for _, sub := range airport.Subscriptions {
			if sub == nil {
				continue
			}
			nodes, err := s.loadMonitorNodes(sub.ID)
			if err == nil && len(nodes) > 0 {
				loaded = true
				appendNodes(sub.ID, nodes)
			}
		}
		if !loaded {
			nodes, err := s.loadMonitorNodes(airport.ID)
			if err == nil && len(nodes) > 0 {
				appendNodes(airport.ID, nodes)
			}
		}
	}
	return result, nil
}

func autoPickAirportNames(nodes []autoPickScopedNode) []string {
	seen := map[string]bool{}
	var names []string
	for _, node := range nodes {
		if node.AirportName == "" || seen[node.AirportName] {
			continue
		}
		seen[node.AirportName] = true
		names = append(names, node.AirportName)
	}
	sort.Strings(names)
	return names
}

func autoPickNodeKey(node autoPickScopedNode) string {
	return node.ProfileID + "\x00" + node.Node.NodeKey + "\x00" + node.Node.NodeIdentityKey + "\x00" + node.Node.ConfigRevisionKey
}

func autoPickServiceUsable(outcome string) bool {
	return outcome == "unlocked" || outcome == "matched"
}

func (s *AppService) autoPickResolve(cfg autopick.Config) ([]autoPickScopedNode, error) {
	if hook := s.autoPick().hooks.resolveNodes; hook != nil {
		return hook(cfg)
	}
	return s.resolveAutoPickNodes(cfg)
}

func (s *AppService) autoPickMeasureLatency(ctx context.Context, reportID string, nodes []autoPickScopedNode, cfg autopick.Config) (string, map[string]autoPickLatency, error) {
	if hook := s.autoPick().hooks.measureLatency; hook != nil {
		return hook(ctx, reportID, nodes, cfg)
	}
	return s.runAutoPickLatencyBatch(ctx, reportID, nodes, cfg)
}

func (s *AppService) autoPickCheckService(ctx context.Context, node monitor.MonitoredNode, rule publicservice.Rule) publicservice.Result {
	if hook := s.autoPick().hooks.checkService; hook != nil {
		return hook(ctx, node, rule)
	}
	checker := s.publicServiceChecker
	if s.historyStore != nil {
		checker.ReputationStore = s.historyStore.DB()
	}
	timeout := time.Duration(rule.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = publicservice.DefaultTimeout
	}
	return checker.Check(ctx, node, rule, timeout)
}

func (s *AppService) runAutoPickLatencyBatch(ctx context.Context, reportID string, nodes []autoPickScopedNode, cfg autopick.Config) (string, map[string]autoPickLatency, error) {
	selections := make([]WorkbenchLatencyBatchSelection, 0, len(nodes))
	for _, node := range nodes {
		selections = append(selections, WorkbenchLatencyBatchSelection{
			ProfileID: node.ProfileID, NodeKey: node.Node.NodeKey, NodeIdentityKey: node.Node.NodeIdentityKey,
			ConfigRevisionKey: node.Node.ConfigRevisionKey, DisplayName: node.Node.DisplayName, NodeType: node.Node.Type,
		})
	}
	batch, err := s.StartWorkbenchLatencyBatch(context.WithValue(ctx, autoPickRoundContextKey{}, autoPickRoundTrigger), WorkbenchLatencyBatchRequest{
		TargetID: "cloudflare", RequestID: "auto-pick:" + reportID, TestProject: WorkbenchLatencyProject,
		TimeoutSeconds: 5, SampleCount: cfg.SampleCount, Selections: selections,
	})
	if err != nil {
		return "", nil, fmt.Errorf("启动延迟检测失败: %w", err)
	}
	deadline := time.Now().Add(autoPickLatencyTimeout)
	for {
		current, err := s.GetWorkbenchLatencyBatch(ctx, batch.BatchID)
		if err != nil {
			return batch.BatchID, nil, fmt.Errorf("读取延迟检测进度失败: %w", err)
		}
		switch current.State {
		case "queued", "running", "cancelling", "saving":
		default:
			return batch.BatchID, autoPickLatencyFromBatch(current), nil
		}
		if time.Now().After(deadline) {
			_, _ = s.CancelWorkbenchLatencyBatch(batch.BatchID)
			return batch.BatchID, nil, fmt.Errorf("延迟检测超过 %s 未完成", autoPickLatencyTimeout)
		}
		select {
		case <-ctx.Done():
			_, _ = s.CancelWorkbenchLatencyBatch(batch.BatchID)
			return batch.BatchID, nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func autoPickLatencyFromBatch(batch *WorkbenchLatencyBatchDTO) map[string]autoPickLatency {
	result := make(map[string]autoPickLatency, len(batch.Items))
	for _, item := range batch.Items {
		if item.Result == nil {
			continue
		}
		measure := autoPickLatency{Samples: item.Result.TotalSamples, Success: item.Result.SuccessSamples, LatencyMS: item.Result.LatencyMs}
		if measure.Samples > 0 {
			measure.ResponseRate = float64(measure.Success) * 100 / float64(measure.Samples)
		}
		if stable, ok := autoPickStableLatency(item.Result.Samples); ok {
			measure.LatencyMS = stable
		} else if len(item.Result.Samples) > 0 {
			// 只有首次采样成功时没有稳定值，不拿首次当稳定延迟。
			measure.LatencyMS = 0
		}
		key := item.ProfileID + "\x00" + item.NodeKey + "\x00" + item.NodeIdentityKey + "\x00" + item.ConfigRevisionKey
		result[key] = measure
	}
	return result
}

// autoPickStableLatency mirrors the inspector's 稳定延迟: drop the first sample
// (connection warm-up), then take the median of the successful ones.
func autoPickStableLatency(samples []WorkbenchLatencySampleDTO) (int64, bool) {
	if len(samples) < 2 {
		return 0, false
	}
	ordered := append([]WorkbenchLatencySampleDTO(nil), samples...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Seq < ordered[j].Seq })
	values := make([]int64, 0, len(ordered)-1)
	for _, sample := range ordered[1:] {
		if sample.Success {
			values = append(values, sample.LatencyMs)
		}
	}
	if len(values) == 0 {
		return 0, false
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return values[len(values)/2], true
}

// --- export ---

func buildAutoPickClashYAML(groups map[string][]autoPickScopedNode, all []autoPickScopedNode) (string, int, error) {
	nodes := make([]monitor.MonitoredNode, 0, len(all))
	for _, node := range all {
		if len(node.Node.RawConfig) == 0 {
			continue
		}
		nodes = append(nodes, node.Node)
	}
	if len(nodes) == 0 {
		return "", 0, monitor.NewValidationError("所选节点缺少有效原始配置，未写入订阅文件")
	}
	proxies, names, err := prepareExportProxies(nodes, "")
	if err != nil {
		return "", 0, err
	}
	nameByKey := make(map[string]string, len(nodes))
	for i, node := range nodes {
		nameByKey[node.NodeKey+"\x00"+node.NodeIdentityKey+"\x00"+node.ConfigRevisionKey] = names[i]
	}
	groupNames := make([]string, 0, 3)
	proxyGroups := make([]map[string]any, 0, 5)
	for _, purpose := range []string{autopick.PurposeDaily, autopick.PurposeStreaming, autopick.PurposeAI} {
		members := groups[purpose]
		memberNames := make([]string, 0, len(members))
		for _, member := range members {
			if name, ok := nameByKey[member.Node.NodeKey+"\x00"+member.Node.NodeIdentityKey+"\x00"+member.Node.ConfigRevisionKey]; ok {
				memberNames = append(memberNames, name)
			}
		}
		if len(memberNames) == 0 {
			continue
		}
		groupNames = append(groupNames, purpose)
		proxyGroups = append(proxyGroups, map[string]any{"name": purpose, "type": "select", "proxies": memberNames})
	}
	if len(proxyGroups) == 0 {
		return "", 0, monitor.NewValidationError("没有可导出的分组")
	}
	head := map[string]any{
		"port": 7890, "socks-port": 7891, "allow-lan": false, "mode": "rule", "log-level": "info",
		"external-controller": "127.0.0.1:9090",
		"proxies":             proxies,
	}
	mainGroup := append(append([]string{}, groupNames...), "AUTO")
	proxyGroups = append([]map[string]any{{"name": "PROXY", "type": "select", "proxies": mainGroup}}, proxyGroups...)
	proxyGroups = append(proxyGroups, map[string]any{
		"name": "AUTO", "type": "url-test", "url": "http://www.gstatic.com/generate_204", "interval": 300, "tolerance": 50, "proxies": names,
	})
	head["proxy-groups"] = proxyGroups
	head["rules"] = []string{"MATCH,PROXY"}
	raw, err := yaml.Marshal(head)
	if err != nil {
		return "", 0, fmt.Errorf("生成 Clash YAML 失败: %w", err)
	}
	return string(raw), len(proxies), nil
}

func writeAutoPickFileAtomic(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// --- local subscription ---

// startAutoPick starts the scheduler loop and the loopback subscription
// server. It is idempotent and safe to call from startup and save paths.
func (s *AppService) startAutoPick() {
	state := s.autoPick()
	state.mu.Lock()
	if state.started {
		state.mu.Unlock()
		return
	}
	state.started = true
	state.stop = make(chan struct{})
	stop := state.stop
	state.mu.Unlock()
	if _, err := os.Stat(s.autoPickSubscriptionPath()); err == nil {
		s.startAutoPickSubscriptionServer()
	}
	state.wg.Add(1)
	go func() {
		defer state.wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				s.autoPickTick()
			}
		}
	}()
}

func (s *AppService) autoPickTick() {
	state := s.autoPick()
	state.mu.Lock()
	running := state.running
	state.mu.Unlock()
	if running {
		return
	}
	cfg, err := s.LoadAutoPickConfig()
	if err != nil || !cfg.Enabled || cfg.NextRunAt == nil {
		return
	}
	if time.Now().Before(*cfg.NextRunAt) {
		return
	}
	state.mu.Lock()
	state.running = true
	state.lastError = ""
	state.mu.Unlock()
	s.startAutoPickRound("scheduled")
}

func (s *AppService) stopAutoPick() {
	state := s.autoPick()
	state.mu.Lock()
	stop := state.stop
	server := state.server
	state.started = false
	state.server = nil
	state.stop = nil
	state.mu.Unlock()
	if stop != nil {
		close(stop)
	}
	if state.cancelRun != nil {
		state.cancelRun()
	}
	if server != nil {
		_ = server.Close()
	}
	state.wg.Wait()
	state.runWG.Wait()
}
