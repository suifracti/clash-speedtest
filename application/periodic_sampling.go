package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/publicservice"
)

type PeriodicSamplingConfig struct {
	LatencyProbeSet         monitor.ProbeSetType   `json:"latency_probe_set"`
	Enabled                 bool                   `json:"enabled"`
	LatencyIntervalSeconds  int64                  `json:"latency_interval_seconds"`
	ServiceIntervalSeconds  int64                  `json:"service_interval_seconds"`
	DownloadIntervalSeconds int64                  `json:"download_interval_seconds"`
	DownloadMiB             int64                  `json:"download_mib"`
	ServiceConcurrency      int                    `json:"service_concurrency"`
	IncludeAntigravity      bool                   `json:"include_antigravity"`
	Selections              []MonitorNodeOptionDTO `json:"selections,omitempty"`
}

func defaultPeriodicSamplingConfig() PeriodicSamplingConfig {
	return PeriodicSamplingConfig{LatencyProbeSet: monitor.ProbeSetLatencySix, LatencyIntervalSeconds: 120, ServiceIntervalSeconds: 300, DownloadIntervalSeconds: 3600, DownloadMiB: 10, ServiceConcurrency: 16, IncludeAntigravity: true}
}

func validatePeriodicSamplingConfig(cfg PeriodicSamplingConfig) error {
	if value := periodicServiceConcurrency(cfg); value < 1 || value > periodicMaximumServiceConcurrency {
		return monitor.NewValidationError("服务巡检并发必须在1到64之间")
	}
	for _, value := range []int64{cfg.LatencyIntervalSeconds, cfg.ServiceIntervalSeconds, cfg.DownloadIntervalSeconds} {
		if value < 10 || value > 86400 {
			return monitor.NewValidationError("定时周期必须在10秒到24小时之间")
		}
	}
	if cfg.DownloadMiB < 1 || cfg.DownloadMiB > 100 {
		return monitor.NewValidationError("每节点下载上限必须为1到100 MiB")
	}
	for _, n := range cfg.Selections {
		if n.ProfileID == "" || n.NodeKey == "" || n.NodeIdentityKey == "" || n.ConfigRevisionKey == "" {
			return monitor.NewValidationError("所选节点必须有完整稳定身份与配置版本")
		}
	}
	return nil
}

func periodicNextDue(scheduled time.Time, interval time.Duration, finished time.Time) (time.Time, int64) {
	steps := int64(finished.Sub(scheduled)/interval) + 1
	if steps < 1 {
		steps = 1
	}
	return scheduled.Add(time.Duration(steps) * interval), steps - 1
}

type PeriodicCycle struct {
	TimingVersion             int            `json:"timing_version,omitempty"`
	QueueWaitSeconds          float64        `json:"queue_wait_seconds,omitempty"`
	NetworkCleanupWaitSeconds float64        `json:"network_cleanup_wait_seconds,omitempty"`
	ServiceTurnWaitSeconds    float64        `json:"service_turn_wait_seconds,omitempty"`
	ExclusionWaitSeconds      float64        `json:"download_exclusion_wait_seconds,omitempty"`
	AdmissionWaitSeconds      float64        `json:"admission_wait_seconds,omitempty"`
	RoundID                   string         `json:"round_id,omitempty"`
	NetworkActive             int            `json:"network_active"`
	LegacyObservation         bool           `json:"legacy_observation"`
	Started                   int            `json:"started"`
	Active                    int            `json:"active"`
	Waiting                   int            `json:"waiting"`
	NotExecuted               int            `json:"not_executed"`
	ExecutionSeconds          float64        `json:"execution_seconds"`
	EstimatedTotalSeconds     *float64       `json:"estimated_total_seconds"`
	RemainingSeconds          *float64       `json:"remaining_seconds"`
	Running                   bool           `json:"running"`
	Concurrency               int            `json:"concurrency"`
	ElapsedSeconds            float64        `json:"elapsed_seconds"`
	WaitSeconds               float64        `json:"wait_seconds"`
	CoveredNodes              int            `json:"covered_nodes"`
	CoveredServices           int            `json:"covered_services"`
	CompletedNodes            int            `json:"completed_nodes"`
	FullCoverage              bool           `json:"full_coverage"`
	Interrupted               bool           `json:"interrupted"`
	ScheduledAt               time.Time      `json:"scheduled_at"`
	StartedAt                 time.Time      `json:"started_at"`
	FinishedAt                time.Time      `json:"finished_at"`
	NextDue                   time.Time      `json:"next_due"`
	NodeCount                 int            `json:"node_count"`
	ServiceCount              int            `json:"service_count"`
	Total                     int            `json:"total"`
	Processed                 int            `json:"processed"`
	Saved                     int            `json:"saved"`
	Unsaved                   int            `json:"unsaved"`
	BytesRead                 int64          `json:"bytes_read"`
	SkippedSlots              int64          `json:"skipped_slots"`
	Error                     string         `json:"error,omitempty"`
	Outcomes                  map[string]int `json:"outcomes"`
}

type PeriodicSamplingStatus struct {
	Config        PeriodicSamplingConfig   `json:"config"`
	Running       bool                     `json:"running"`
	NodeCount     int                      `json:"node_count"`
	ServiceCount  int                      `json:"service_count"`
	MonitorJobIDs []string                 `json:"monitor_job_ids"`
	PauseReason   string                   `json:"pause_reason,omitempty"`
	Error         string                   `json:"error,omitempty"`
	Cycles        map[string]PeriodicCycle `json:"cycles"`
	RecentCycles  []PeriodicCycleRecord    `json:"recent_cycles"`
}

type periodicJob struct {
	ID        string `json:"id"`
	Signature string `json:"signature"`
}
type periodicSampling struct {
	app              *AppService
	lifecycle        sync.Mutex
	mu               sync.Mutex
	gate             sync.RWMutex
	serviceTurnUntil time.Time
	cfg              PeriodicSamplingConfig
	jobs             map[string]periodicJob
	cycles           map[string]PeriodicCycle
	recent           []PeriodicCycleRecord
	cancel           context.CancelFunc
	wg               sync.WaitGroup
	closed           bool
	pauseReason      string
	lastError        string
	// Test seams keep timing/coverage verification offline.
	readErrorHook func() error
	servicesHook  func(PeriodicSamplingConfig) []publicservice.Rule
	nodesHook     func() ([]MonitorNodeOptionDTO, error)
	measureHook   func(context.Context, string, MonitorNodeOptionDTO, string) (string, bool, int64, error)
}

func newPeriodicSampling(app *AppService) *periodicSampling {
	p := &periodicSampling{app: app, cfg: defaultPeriodicSamplingConfig(), jobs: map[string]periodicJob{}, cycles: map[string]PeriodicCycle{}}
	if data, err := os.ReadFile(p.path()); err == nil {
		var saved struct {
			Config PeriodicSamplingConfig `json:"config"`
			Jobs   map[string]periodicJob `json:"jobs"`
		}
		if err = json.Unmarshal(data, &saved); err == nil {
			err = validatePeriodicSamplingConfig(saved.Config)
		}
		if err != nil {
			p.lastError = "定时配置读取失败，未自动启动：" + err.Error()
		} else {
			saved.Config.LatencyProbeSet = monitor.ProbeSetLatencySix
			saved.Config.ServiceConcurrency = periodicServiceConcurrency(saved.Config)
			p.cfg = saved.Config
			if saved.Jobs != nil {
				p.jobs = saved.Jobs
			}
		}
	} else if !os.IsNotExist(err) {
		p.lastError = err.Error()
	}
	p.loadPeriodicCycles()
	return p
}

func (p *periodicSampling) path() string {
	return filepath.Join(p.app.appPaths.DataRoot, "periodic_sampling.json")
}
func (p *periodicSampling) persistLocked(cfg PeriodicSamplingConfig) error {
	data, err := json.MarshalIndent(struct {
		Config PeriodicSamplingConfig `json:"config"`
		Jobs   map[string]periodicJob `json:"jobs"`
	}{cfg, p.jobs}, "", "  ")
	if err != nil {
		return err
	}
	if p.app.appPaths.DataRoot == "" {
		return fmt.Errorf("定时采样必须使用明确的数据根目录")
	}
	if err = os.MkdirAll(p.app.appPaths.DataRoot, 0700); err != nil {
		return err
	}
	tmp := p.path() + ".tmp"
	if err = os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, p.path())
}

func (s *AppService) GetPeriodicSampling() (*PeriodicSamplingStatus, error) {
	p := s.periodic
	p.mu.Lock()
	status := &PeriodicSamplingStatus{Config: p.cfg, Running: p.cancel != nil, PauseReason: p.pauseReason, Error: p.lastError, Cycles: map[string]PeriodicCycle{}}
	status.Config.LatencyProbeSet = monitor.ProbeSetLatencySix
	status.Config.Selections = append([]MonitorNodeOptionDTO(nil), p.cfg.Selections...)
	for _, job := range p.jobs {
		status.MonitorJobIDs = append(status.MonitorJobIDs, job.ID)
	}
	for kind, cycle := range p.cycles {
		if cycle.Running {
			cycle.ElapsedSeconds = time.Since(cycle.StartedAt).Seconds()
		}
		cycle.Outcomes = clonePeriodicOutcomes(cycle.Outcomes)
		periodicEstimate(&cycle)
		if p.app != nil {
			cycle.NetworkActive = p.app.probeActivity.count(kind)
		}
		status.Cycles[kind] = cycle
	}
	for _, record := range p.recent {
		record.Cycle = periodicObserveLegacy(record.Cycle)
		record.Cycle.Outcomes = clonePeriodicOutcomes(record.Cycle.Outcomes)
		status.RecentCycles = append(status.RecentCycles, record)
	}
	p.mu.Unlock()
	nodes, err := p.nodes(status.Config)
	if err == nil {
		status.NodeCount = len(nodes)
	} else if status.Error == "" {
		status.Error = err.Error()
	}
	status.ServiceCount = len(periodicServices(status.Config))
	return status, nil
}

func clonePeriodicOutcomes(source map[string]int) map[string]int {
	result := map[string]int{}
	for k, v := range source {
		result[k] = v
	}
	return result
}
func periodicServices(cfg PeriodicSamplingConfig) []publicservice.Rule {
	result := []publicservice.Rule{}
	for _, r := range publicservice.Catalog() {
		if cfg.IncludeAntigravity || r.ServiceID != "antigravity" {
			result = append(result, r)
		}
	}
	return result
}
func (p *periodicSampling) nodes(cfg PeriodicSamplingConfig) ([]MonitorNodeOptionDTO, error) {
	var nodes []MonitorNodeOptionDTO
	var err error
	if p.nodesHook != nil {
		nodes, err = p.nodesHook()
	} else {
		nodes, err = p.app.ListMonitorNodeOptions()
	}
	if err != nil {
		return nil, err
	}
	if len(cfg.Selections) == 0 {
		targets := make([]MonitorNodeOptionDTO, 0, len(nodes))
		for _, n := range nodes {
			if !periodicNotice(n) {
				targets = append(targets, n)
			}
		}
		return targets, nil
	}
	result := []MonitorNodeOptionDTO{}
	for _, n := range nodes {
		for _, chosen := range cfg.Selections {
			if n.ProfileID == chosen.ProfileID && n.NodeKey == chosen.NodeKey && n.NodeIdentityKey == chosen.NodeIdentityKey && n.ConfigRevisionKey == chosen.ConfigRevisionKey {
				result = append(result, n)
				break
			}
		}
	}
	if len(result) != len(cfg.Selections) {
		return nil, monitor.NewValidationError("所选节点配置已变化，请重新选择；未使用旧配置采样")
	}
	return result, nil
}

func (s *AppService) ConfigurePeriodicSampling(cfg PeriodicSamplingConfig) (*PeriodicSamplingStatus, error) {
	cfg.LatencyProbeSet = monitor.ProbeSetLatencySix
	cfg.ServiceConcurrency = periodicServiceConcurrency(cfg)
	if err := validatePeriodicSamplingConfig(cfg); err != nil {
		return nil, err
	}
	p := s.periodic
	p.lifecycle.Lock()
	defer p.lifecycle.Unlock()
	if p.closed {
		return nil, fmt.Errorf("服务正在退出")
	}
	if cfg.Enabled {
		if err := s.canonicalMonitorHistory(); err != nil {
			return nil, err
		}
		if _, err := p.nodes(cfg); err != nil {
			return nil, err
		}
	}
	p.mu.Lock()
	err := p.persistLocked(cfg)
	if err == nil {
		p.cfg = cfg
	}
	p.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("定时配置未保存，未改变运行状态：%w", err)
	}
	if cfg.Enabled {
		p.stopMeasurementsLocked()
	} else {
		p.stopLocked()
	}
	p.mu.Lock()
	p.cfg = cfg
	p.lastError = ""
	p.mu.Unlock()
	if cfg.Enabled {
		p.startLocked()
	}
	return s.GetPeriodicSampling()
}

func (p *periodicSampling) startLocked() {
	p.mu.Lock()
	if p.cancel != nil || !p.cfg.Enabled || p.closed {
		p.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	cfg := p.cfg
	p.mu.Unlock()
	p.wg.Add(3)
	go func() { defer p.wg.Done(); p.manageMonitors(ctx, cfg) }()
	go func() {
		defer p.wg.Done()
		p.loop(ctx, "service", time.Duration(cfg.ServiceIntervalSeconds)*time.Second, 0, cfg)
	}()
	go func() {
		defer p.wg.Done()
		p.loop(ctx, "download", time.Duration(cfg.DownloadIntervalSeconds)*time.Second, 30*time.Second, cfg)
	}()
}
func (p *periodicSampling) stopMeasurementsLocked() {
	p.mu.Lock()
	cancel := p.cancel
	p.mu.Unlock()
	if cancel != nil {
		cancel()
		p.wg.Wait()
	}
	p.mu.Lock()
	p.cancel = nil
	p.mu.Unlock()
}
func (p *periodicSampling) stopLocked() {
	p.stopMeasurementsLocked()
	p.mu.Lock()
	jobs := make([]string, 0, len(p.jobs))
	for _, j := range p.jobs {
		jobs = append(jobs, j.ID)
	}
	p.mu.Unlock()
	for _, id := range jobs {
		_ = p.app.StopMonitorJob(id)
	}
}
func (p *periodicSampling) stop() { p.lifecycle.Lock(); defer p.lifecycle.Unlock(); p.stopLocked() }
func (p *periodicSampling) close() {
	p.lifecycle.Lock()
	defer p.lifecycle.Unlock()
	p.closed = true
	p.stopLocked()
}

func (p *periodicSampling) manageMonitors(ctx context.Context, cfg PeriodicSamplingConfig) {
	for {
		if ctx.Err() != nil {
			return
		}
		reason := p.app.monitorStorageGuard()
		p.mu.Lock()
		p.pauseReason = reason
		p.mu.Unlock()
		var err error
		if reason == "" {
			err = p.syncMonitors(ctx, cfg)
		} else {
			p.mu.Lock()
			ids := []string{}
			for _, j := range p.jobs {
				ids = append(ids, j.ID)
			}
			p.mu.Unlock()
			for _, id := range ids {
				_ = p.app.StopMonitorJob(id)
			}
		}
		p.mu.Lock()
		if err != nil {
			p.lastError = err.Error()
		} else {
			p.lastError = ""
		}
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-time.After(20 * time.Second):
		}
	}
}

func (p *periodicSampling) syncMonitors(ctx context.Context, cfg PeriodicSamplingConfig) error {
	nodes, err := p.nodes(cfg)
	if err != nil {
		return err
	}
	groups := map[string][]MonitorNodeOptionDTO{}
	for _, n := range nodes {
		groups[n.ProfileID] = append(groups[n.ProfileID], n)
	}
	for profile, selected := range groups {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		data, _ := json.Marshal(struct {
			ProbeSet monitor.ProbeSetType
			Interval int64
			Nodes    []MonitorNodeOptionDTO
		}{monitor.ProbeSetLatencySix, cfg.LatencyIntervalSeconds, selected})
		signature := fmt.Sprintf("%x", sha256.Sum256(data))
		p.mu.Lock()
		entry := p.jobs[profile]
		p.mu.Unlock()
		if entry.ID != "" && entry.Signature != signature {
			_ = p.app.StopMonitorJob(entry.ID)
			entry.ID = ""
		}
		if entry.ID != "" {
			if _, err = p.app.GetMonitorJob(entry.ID); err != nil {
				entry.ID = ""
			}
		}
		if entry.ID == "" {
			req := MonitorJobCreateRequest{Name: "定时采样·延迟·" + selected[0].ProfileName, ProfileID: profile, ProbeSet: monitor.ProbeSetLatencySix, SamplingTier: monitor.SamplingTierRegular, IntervalSeconds: cfg.LatencyIntervalSeconds, TimeoutSeconds: 5}
			for _, n := range selected {
				req.NodeKeys = append(req.NodeKeys, n.NodeKey)
				req.NodeContexts = append(req.NodeContexts, MonitorNodeSelectionContext{NodeKey: n.NodeKey, NodeIdentityKey: n.NodeIdentityKey, ConfigRevisionKey: n.ConfigRevisionKey})
			}
			job, createErr := p.app.CreateMonitorJobFromRequest(req)
			if createErr != nil {
				return createErr
			}
			entry = periodicJob{job.ID, signature}
			p.mu.Lock()
			p.jobs[profile] = entry
			err = p.persistLocked(p.cfg)
			p.mu.Unlock()
			if err != nil {
				return err
			}
		}
		job, getErr := p.app.GetMonitorJob(entry.ID)
		if getErr != nil {
			return getErr
		}
		if job.State == monitor.JobStateRunning {
			continue
		}
		if err = p.app.StartMonitorJob(entry.ID); err != nil {
			return err
		}
	}
	p.mu.Lock()
	removed := []string{}
	for profile, j := range p.jobs {
		if len(groups[profile]) == 0 {
			removed = append(removed, j.ID)
		}
	}
	p.mu.Unlock()
	for _, id := range removed {
		_ = p.app.StopMonitorJob(id)
	}
	return nil
}

func (p *periodicSampling) loop(ctx context.Context, kind string, interval, delay time.Duration, cfg PeriodicSamplingConfig) {
	due := time.Now().Add(delay)
	for {
		timer := time.NewTimer(time.Until(due))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		p.runCycle(ctx, kind, due, cfg)
		next, missed := periodicNextDue(due, interval, time.Now())
		p.mu.Lock()
		cycle := p.cycles[kind]
		cycle.NextDue = next
		cycle.SkippedSlots += missed
		p.cycles[kind] = cycle
		p.archivePeriodicCycleLocked(kind, cycle)
		p.mu.Unlock()
		due = next
	}
}

func (p *periodicSampling) runCycle(ctx context.Context, kind string, due time.Time, cfg PeriodicSamplingConfig) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if p.app != nil {
		reason := p.app.monitorStorageGuard()
		p.mu.Lock()
		p.pauseReason = reason
		p.mu.Unlock()
	}
	nodes, err := p.nodes(cfg)
	services := periodicServices(cfg)
	if p.servicesHook != nil {
		services = p.servicesHook(cfg)
	}
	workers := periodicServiceConcurrency(cfg)
	total := len(nodes) * len(services)
	if kind == "download" {
		workers = 1
		total = len(nodes)
	}
	roundID := newWorkbenchLatencyID("patrol_")
	if p.app != nil && p.app.historyStore != nil && err == nil && total > 0 {
		trigger := "scheduled"
		if ctx.Value(scheduledRoundContextKey{}) == "diagnostic" {
			trigger = "diagnostic"
		}
		plan := history.MeasurementRound{RoundID: roundID, TriggerType: trigger, StartedAt: time.Now().UTC()}
		for _, n := range nodes {
			if kind == "download" {
				plan.Items = append(plan.Items, history.MeasurementRoundItem{RequestID: periodicRoundRequestID(roundID, n, ""), Project: kind, ProfileID: n.ProfileID, NodeKey: n.NodeKey, NodeIdentityKey: n.NodeIdentityKey, ConfigRevisionKey: n.ConfigRevisionKey, DisplayName: n.DisplayName})
			} else {
				for _, r := range services {
					plan.Items = append(plan.Items, history.MeasurementRoundItem{RequestID: periodicRoundRequestID(roundID, n, r.ServiceID), Project: kind, ServiceID: r.ServiceID, ProfileID: n.ProfileID, NodeKey: n.NodeKey, NodeIdentityKey: n.NodeIdentityKey, ConfigRevisionKey: n.ConfigRevisionKey, DisplayName: n.DisplayName})
				}
			}
		}
		if e := p.app.historyStore.DB().CreateMeasurementRound(ctx, plan); e != nil {
			err = e
		} else {
			ctx = context.WithValue(ctx, scheduledRoundContextKey{}, trigger)
			ctx = context.WithValue(ctx, periodicRoundIDContextKey{}, roundID)
			defer func() {
				state := "finished"
				if ctx.Err() != nil {
					state = "interrupted"
				}
				if e := p.app.historyStore.DB().FinishMeasurementRound(context.Background(), roundID, state); e != nil {
					p.cycleError(kind, e)
				}
			}()
		}
	}
	p.mu.Lock()
	previous := p.cycles[kind]
	p.cycles[kind] = PeriodicCycle{TimingVersion: 2, RoundID: roundID, Running: true, ScheduledAt: due, StartedAt: time.Now(), NodeCount: len(nodes), ServiceCount: len(services), Concurrency: workers, Total: total, SkippedSlots: previous.SkippedSlots, Outcomes: map[string]int{}}
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		cycle := p.cycles[kind]
		cycle.Running = false
		cycle.FinishedAt = time.Now()
		cycle.ElapsedSeconds = cycle.FinishedAt.Sub(cycle.StartedAt).Seconds()
		cycle.Interrupted = ctx.Err() != nil
		cycle.FullCoverage = !cycle.Interrupted && cycle.Total > 0 && cycle.CompletedNodes == cycle.NodeCount && (kind == "download" || cycle.CoveredServices == cycle.ServiceCount)
		p.cycles[kind] = cycle
	}()
	if err != nil {
		p.cycleError(kind, err)
		return
	}
	queue := make(chan periodicTask)
	seenNodes := map[string]int{}
	seenServices := map[string]bool{}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			downloadHeld := false
			batch := 0
			var batchStarted time.Time
			defer p.releaseDownloadGate(&downloadHeld)
			for task := range queue {
				if ctx.Err() != nil {
					return
				}
				p.mu.Lock()
				paused := p.pauseReason
				p.mu.Unlock()
				if paused != "" {
					p.cycleError(kind, fmt.Errorf("%s", paused))
					cancel()
					return
				}
				p.adjustActivity(kind, 1, 0, 0)
				var exclusionNS, turnNS int64
				queueNS := int64(0)
				if !task.queuedAt.IsZero() {
					queueNS = time.Since(task.queuedAt).Nanoseconds()
				}
				p.observeWaitPart(kind, "queue", float64(queueNS)/1e9)
				if kind == "download" {
					if !downloadHeld {
						turnWait, waitErr := p.waitServiceTurn(ctx)
						turnNS = turnWait.Nanoseconds()
						p.addPeriodicWait(kind, turnWait.Seconds())
						p.observeWaitPart(kind, "service_turn", turnWait.Seconds())
						if waitErr != nil {
							p.adjustActivity(kind, -1, 0, 0)
							return
						}
						waiting := time.Now()
						p.gate.Lock()
						exclusionNS = time.Since(waiting).Nanoseconds()
						downloadHeld = true
						batch = 0
						batchStarted = time.Now()
						p.addPeriodicWait(kind, float64(exclusionNS)/1e9)
						p.observeWaitPart(kind, "exclusion", float64(exclusionNS)/1e9)
					}
				} else {
					waiting := time.Now()
					p.gate.RLock()
					exclusionNS = time.Since(waiting).Nanoseconds()
					p.addPeriodicWait(kind, float64(exclusionNS)/1e9)
					p.observeWaitPart(kind, "exclusion", float64(exclusionNS)/1e9)
				}
				var outcome string
				var saved bool
				var bytes int64
				var measureErr error
				releaseNetwork := func() { p.releaseDownloadGate(&downloadHeld) }
				if kind == "service" {
					var once sync.Once
					releaseNetwork = func() { once.Do(p.gate.RUnlock) }
				}
				if ctx.Err() == nil {
					outcome, saved, bytes, measureErr = p.measure(withProbeTiming(withProbeTiming(withProbeTiming(ctx, "queue_wait_ns", queueNS), "service_turn_wait_ns", turnNS), "download_exclusion_wait_ns", exclusionNS), kind, task.node, task.service, cfg, releaseNetwork)
				} else {
					p.adjustActivity(kind, -1, 0, 0)
				}
				if kind == "download" {
					batch++
					if periodicDownloadYield(batch, time.Since(batchStarted)) {
						releaseNetwork()
					}
				} else {
					releaseNetwork()
				}
				if ctx.Err() != nil {
					return
				}
				if measureErr != nil && outcome == "" && p.app != nil && p.app.historyStore != nil {
					_ = p.app.historyStore.DB().MarkMeasurementNotExecuted(context.Background(), periodicRoundRequestID(roundID, task.node, task.service), measureErr.Error())
				}
				if measureErr != nil && outcome == "" {
					outcome = "not_executed"
				}
				if outcome == "" {
					outcome = "unknown"
				}
				p.mu.Lock()
				cycle := p.cycles[kind]
				cycle.Processed++
				if outcome == "not_executed" {
					cycle.NotExecuted++
				} else if saved {
					cycle.Saved++
				} else {
					cycle.Unsaved++
				}
				cycle.BytesRead += bytes
				cycle.Outcomes[outcome]++
				if measureErr != nil {
					cycle.Error = measureErr.Error()
				} else if outcome != "not_executed" {
					key := task.node.ProfileID + "\x00" + task.node.NodeIdentityKey + "\x00" + task.node.ConfigRevisionKey
					seenNodes[key]++
					if kind == "service" {
						seenServices[task.service] = true
						if seenNodes[key] == len(services) {
							cycle.CompletedNodes++
						}
					} else {
						cycle.CompletedNodes++
					}
					cycle.CoveredNodes = len(seenNodes)
					cycle.CoveredServices = len(seenServices)
				}
				p.cycles[kind] = cycle
				p.mu.Unlock()
			}
		}()
	}
	produce := func(task periodicTask) bool {
		task.queuedAt = time.Now()
		select {
		case <-ctx.Done():
			return false
		case queue <- task:
			return true
		}
	}
	if kind == "download" {
		for _, n := range periodicFairDownloadNodes(nodes) {
			if !produce(periodicTask{node: n}) {
				break
			}
		}
	} else {
		for _, task := range periodicFairServiceTasks(nodes, services) {
			if !produce(task) {
				break
			}
		}
	}
	close(queue)
	wg.Wait()
}
func (p *periodicSampling) cycleError(kind string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	cycle := p.cycles[kind]
	cycle.Error = err.Error()
	p.cycles[kind] = cycle
}

func periodicFinished(execution, persistence string) bool {
	return (execution != "" && execution != "queued" && execution != "running") && (persistence == "saved" || persistence == "failed" || persistence == "not_applicable")
}
func (p *periodicSampling) measure(ctx context.Context, kind string, node MonitorNodeOptionDTO, service string, cfg PeriodicSamplingConfig, releaseNetwork ...func()) (outcome string, saved bool, readBytes int64, measureErr error) {
	release := func() {}
	if len(releaseNetwork) > 0 {
		release = releaseNetwork[0]
		// Service readers end per probe. Downloads retain a bounded batch,
		// except when network has ended and persistence is unusually slow.
		if kind == "service" {
			defer release()
		}
	}
	if p.measureHook != nil {
		p.adjustActivity(kind, -1, 1, 1)
		defer p.adjustActivity(kind, 0, -1, 0)
		return p.measureHook(ctx, kind, node, service)
	}
	started := false
	defer func() {
		if started && measureErr != nil && outcome == "" {
			outcome = "execution_observation_error"
		}
	}()
	defer func() {
		if !started {
			p.adjustActivity(kind, -1, 0, 0)
		} else {
			p.adjustActivity(kind, 0, -1, 0)
		}
	}()
	requestID := newWorkbenchLatencyAttemptID()
	if roundID, _ := ctx.Value(periodicRoundIDContextKey{}).(string); roundID != "" {
		requestID = periodicRoundRequestID(roundID, node, service)
	}
	if kind == "service" {
		q := WorkbenchPublicServiceHistoryQuery{ProfileID: node.ProfileID, NodeKey: node.NodeKey, NodeIdentityKey: node.NodeIdentityKey, ConfigRevisionKey: node.ConfigRevisionKey, ServiceID: service}
		// One original 10s lease includes admission/setup and the request. A
		// worker waiting inside the gate cannot start a fresh 10s tail after
		// the refill window has closed. Persistence uses the observer context.
		requestCtx, cancelRequest := context.WithTimeout(ctx, 10*time.Second)
		defer cancelRequest()
		attempt, waited, err := p.startPeriodicPublicService(requestCtx, WorkbenchPublicServiceTestRequest{RequestID: requestID, ProfileID: node.ProfileID, NodeKey: node.NodeKey, NodeIdentityKey: node.NodeIdentityKey, ConfigRevisionKey: node.ConfigRevisionKey, ServiceID: service, TimeoutSeconds: 10}, periodicServiceConcurrency(cfg))
		p.addPeriodicWait(kind, waited)
		p.observeWaitPart(kind, "admission", waited)
		if err != nil {
			return "", false, 0, err
		}
		started = true
		p.adjustActivity(kind, -1, 1, 1)
		p.app.publicServiceMu.Lock()
		runtime := p.app.publicServiceActive[attempt.AttemptID]
		var networkDone <-chan struct{}
		if runtime != nil {
			networkDone = runtime.networkDone
		}
		p.app.publicServiceMu.Unlock()
		if runtime == nil || !probeNetworkRunning(networkDone) {
			release()
			networkDone = nil
		}
		id := attempt.AttemptID
		// Cancellation must not release a reader while its transport is still
		// unwinding. Keep the original checker's deadline, never a fresh drain
		// budget, and let persistence run after the network gate is released.
		originalNetworkDone := networkDone
		defer func() {
			if ctx.Err() != nil || measureErr != nil {
				if runtime != nil {
					runtime.cancel()
				}
				p.drainPeriodicNetwork(kind, originalNetworkDone)
				release()
				_, _ = p.app.CancelWorkbenchPublicServiceTest(context.Background(), id, q)
			}
		}()
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		deadline := time.NewTimer(45 * time.Second)
		defer deadline.Stop()
		for !periodicFinished(attempt.ExecutionState, attempt.PersistenceState) {
			select {
			case <-networkDone:
				release()
				networkDone = nil
			case <-ctx.Done():
				return "cancelled", false, 0, ctx.Err()
			case <-deadline.C:
				return "execution_wait_timeout", false, 0, fmt.Errorf("检测完成状态等待超时")
			case <-ticker.C:
			}
			if p.readErrorHook != nil {
				if err = p.readErrorHook(); err != nil {
					return "", false, 0, err
				}
			}
			attempt, err = p.app.GetWorkbenchPublicServiceAttempt(context.Background(), attempt.AttemptID, q)
			if err != nil {
				return "", false, 0, err
			}
		}
		if attempt.PersistenceState == "failed" && attempt.Result != nil {
			for i := 0; i < 3; i++ {
				retry, retryErr := p.app.RetrySaveWorkbenchPublicServiceTest(context.Background(), attempt.AttemptID, q)
				if retryErr == nil {
					attempt = retry
					break
				}
				if ctx.Err() != nil {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
		if attempt.Result == nil {
			return attempt.ExecutionState, false, 0, nil
		}
		if attempt.ExecutionState == "not_executed" || attempt.Result.Details["execution_status"] == "not_executed" {
			return "not_executed", attempt.PersistenceState == "saved", 0, nil
		}
		p.observeExecution(kind, float64(attempt.Result.DurationMs)/1000)
		return attempt.Result.Outcome, attempt.PersistenceState == "saved", attempt.Result.BytesRead, nil
	}
	q := WorkbenchDownloadHistoryQuery{ProfileID: node.ProfileID, NodeKey: node.NodeKey, NodeIdentityKey: node.NodeIdentityKey, ConfigRevisionKey: node.ConfigRevisionKey}
	attempt, waited, err := p.startPeriodicDownload(ctx, WorkbenchDownloadTestRequest{RequestID: requestID, ProfileID: node.ProfileID, NodeKey: node.NodeKey, NodeIdentityKey: node.NodeIdentityKey, ConfigRevisionKey: node.ConfigRevisionKey, MaximumBytes: cfg.DownloadMiB * 1048576, TimeoutSeconds: 10})
	p.addPeriodicWait(kind, waited)
	p.observeWaitPart(kind, "admission", waited)
	if err != nil {
		return "", false, 0, err
	}
	started = true
	p.adjustActivity(kind, -1, 1, 1)
	id := attempt.AttemptID
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(45 * time.Second)
	defer deadline.Stop()
	p.app.workbenchMu.Lock()
	runtime := p.app.workbenchActiveDownload
	var networkDone <-chan struct{}
	if runtime != nil && runtime.attemptID == attempt.AttemptID {
		networkDone = runtime.networkDone
	}
	p.app.workbenchMu.Unlock()
	originalNetworkDone := networkDone
	defer func() {
		if ctx.Err() != nil || measureErr != nil {
			if runtime != nil && runtime.attemptID == id {
				runtime.cancel()
			}
			p.drainPeriodicNetwork(kind, originalNetworkDone)
			release()
			_, _ = p.app.CancelWorkbenchDownloadTest(context.Background(), id, q)
		}
	}()
	var slowSave *time.Timer
	var slowSaveC <-chan time.Time
	defer func() {
		if slowSave != nil {
			slowSave.Stop()
		}
	}()
	for !periodicFinished(attempt.ExecutionState, attempt.PersistenceState) {
		select {
		case <-ctx.Done():
			return "cancelled", false, 0, ctx.Err()
		case <-deadline.C:
			return "execution_wait_timeout", false, 0, fmt.Errorf("下载完成状态等待超时")
		case <-networkDone:
			networkDone = nil
			slowSave = time.NewTimer(50 * time.Millisecond)
			slowSaveC = slowSave.C
		case <-slowSaveC:
			// The bandwidth measurement is over. Do not let a slow save hold patrol.
			release()
			slowSaveC = nil
		case <-ticker.C:
		}
		if p.readErrorHook != nil {
			if err = p.readErrorHook(); err != nil {
				return "", false, 0, err
			}
		}
		attempt, err = p.app.GetWorkbenchDownloadAttempt(context.Background(), attempt.AttemptID, q)
		if err != nil {
			return "", false, 0, err
		}
	}
	if attempt.PersistenceState == "failed" && attempt.Result != nil {
		for i := 0; i < 3; i++ {
			retry, retryErr := p.app.RetrySaveWorkbenchDownloadTest(context.Background(), attempt.AttemptID, q)
			if retryErr == nil {
				attempt = retry
				break
			}
			if ctx.Err() != nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	if attempt.Result == nil {
		return attempt.ExecutionState, false, 0, nil
	}
	p.observeExecution(kind, float64(attempt.Result.DurationNS)/1e9)
	return attempt.Result.Outcome, attempt.PersistenceState == "saved", attempt.Result.BytesRead, nil
}
