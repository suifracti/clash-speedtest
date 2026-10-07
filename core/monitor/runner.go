package monitor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/constant"
)

// NodeDialer abstracts client creation for a proxy node so runner execution
// is decoupled from core dialer implementations and easily mockable in tests.
type NodeDialer interface {
	CreateClient(node MonitoredNode, timeout time.Duration) (*http.Client, error)
}

// DefaultNodeDialer builds isolated, in-memory proxy HTTP clients via mihomo adapter.
// It never alters system proxy or external controller node selections.
type DefaultNodeDialer struct{}

func NewDefaultNodeDialer() *DefaultNodeDialer {
	return &DefaultNodeDialer{}
}

func (d *DefaultNodeDialer) CreateClient(node MonitoredNode, timeout time.Duration) (*http.Client, error) {
	if len(node.RawConfig) == 0 {
		return nil, fmt.Errorf("node %s has empty raw config", node.NodeKey)
	}

	proxy, err := adapter.ParseProxy(node.RawConfig)
	if err != nil {
		return nil, fmt.Errorf("parse proxy for %s: %w", node.NodeKey, err)
	}

	transport := &http.Transport{
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, portStr, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			var u16Port uint16
			if p, err := strconv.ParseUint(portStr, 10, 16); err == nil {
				u16Port = uint16(p)
			}
			return proxy.DialContext(ctx, &constant.Metadata{
				Host:    host,
				DstPort: u16Port,
			})
		},
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}, nil
}

// RunnerConfig configures the Runner.
type RunnerConfig struct {
	ProbeActivity func(string) func() []string
	Store         SampleStore
	Dialer        NodeDialer
	WorkerCount   int
	Budget        *BudgetController
}

// Runner executes one round of probe checks across a job's nodes using an isolated dialer.
type Runner struct {
	probeActivity func(string) func() []string
	store         SampleStore
	dialer        NodeDialer
	workerCount   int
	budget        atomic.Pointer[BudgetController]
}

// NewRunner creates a new Runner instance.
func NewRunner(cfg RunnerConfig) *Runner {
	if cfg.Dialer == nil {
		cfg.Dialer = NewDefaultNodeDialer()
	}
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = 4
	}
	runner := &Runner{
		store:         cfg.Store,
		probeActivity: cfg.ProbeActivity,
		dialer:        cfg.Dialer,
		workerCount:   cfg.WorkerCount,
	}
	runner.budget.Store(cfg.Budget)
	return runner
}

func (r *Runner) SetBudget(budget *BudgetController) { r.budget.Store(budget) }

// TargetSpec defines a probe URL target and its expected probe type.
type TargetSpec struct {
	ResponseDelayOnly bool
	ProbeType         string
	URL               string
}

// GetProbeTargets returns the target probes for a given ProbeSetType.
func GetProbeTargets(pset ProbeSetType) []TargetSpec {
	switch pset {
	case ProbeSetLatencySix:
		return []TargetSpec{
			{ResponseDelayOnly: true, ProbeType: "rtt", URL: "https://speed.cloudflare.com/__down?bytes=1"},
			{ResponseDelayOnly: true, ProbeType: "rtt", URL: "https://www.gstatic.com/generate_204"},
			{ResponseDelayOnly: true, ProbeType: "rtt", URL: "https://api.github.com/zen"},
			{ResponseDelayOnly: true, ProbeType: "rtt", URL: "https://captive.apple.com/hotspot-detect.html"},
			{ResponseDelayOnly: true, ProbeType: "rtt", URL: "http://www.msftconnecttest.com/connecttest.txt"},
			{ResponseDelayOnly: true, ProbeType: "rtt", URL: "https://detectportal.firefox.com/success.txt"},
		}
	case ProbeSetService:
		return []TargetSpec{
			{ProbeType: "rtt", URL: "https://cp.cloudflare.com/generate_204"},
			{ProbeType: "service_google", URL: "https://www.google.com/generate_204"},
			{ProbeType: "service_github", URL: "https://api.github.com"},
		}
	case ProbeSetHeavy:
		return []TargetSpec{
			{ProbeType: "rtt", URL: "https://cp.cloudflare.com/generate_204"},
			{ProbeType: "ttfb_heavy", URL: "https://speed.cloudflare.com/__down?bytes=50000"},
			{ProbeType: "service_google", URL: "https://www.google.com/generate_204"},
		}
	case ProbeSetLight:
		fallthrough
	default:
		return []TargetSpec{
			{ProbeType: "rtt", URL: "https://cp.cloudflare.com/generate_204"},
		}
	}
}

// ExecuteRun executes probe targets for all nodes in the job, records raw samples,
// and persists results into the SampleStore.
func (r *Runner) ExecuteRun(ctx context.Context, job *MonitorJob, scheduledAt time.Time) (*MonitorRun, []*MonitorSample, error) {
	return r.ExecuteRunWithAdmission(ctx, job, scheduledAt, nil)
}

// ExecuteRunWithAdmission lets the scheduler distinguish a cancellable wait
// from an admitted round. Direct runner callers use ExecuteRun.
func (r *Runner) ExecuteRunWithAdmission(ctx context.Context, job *MonitorJob, scheduledAt time.Time, onAdmitted func() error) (*MonitorRun, []*MonitorSample, error) {
	if job == nil {
		return nil, nil, fmt.Errorf("job is nil")
	}
	if budget := r.budget.Load(); budget != nil {
		admittedCtx, release, err := budget.AcquireRound(ctx, job)
		if err != nil {
			return nil, nil, err
		}
		defer release()
		ctx = admittedCtx
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, budgetBlock("cancelled", "Monitor 等待已取消，本周期未探测")
	}
	if onAdmitted != nil {
		if err := onAdmitted(); err != nil {
			return nil, nil, err
		}
	}

	runID := newID("run")
	startedAt := time.Now()
	tier := job.SamplingTier
	if tier != SamplingTierFocus && tier != SamplingTierSparse {
		tier = SamplingTierRegular
	}
	trigger := job.NextRunTrigger
	if trigger == "" {
		trigger = SamplingTriggerScheduled
	}
	if trigger == SamplingTriggerManual {
		tier = SamplingTierDiagnostic
	} else {
		trigger = SamplingTriggerScheduled
	}

	run := &MonitorRun{
		RunID:                   runID,
		JobID:                   job.ID,
		SamplingTier:            tier,
		TriggerType:             trigger,
		SamplingStrategyVersion: SamplingStrategyVersion,
		ScheduledAt:             scheduledAt,
		StartedAt:               startedAt,
		Status:                  RunStatusRunning,
		TotalNodes:              len(job.Nodes),
		SuccessNodes:            0,
		FailedNodes:             0,
	}

	if r.store != nil {
		if err := r.store.SaveMonitorRun(ctx, run); err != nil {
			run.Status = RunStatusPersistenceFailed
			run.ErrorMessage = fmt.Sprintf("save initial monitor run: %v", err)
			return run, nil, fmt.Errorf("save initial monitor run: %w", err)
		}
	}

	targets := GetProbeTargets(job.ProbeSet)
	nodeTimeout := job.Timeout
	if nodeTimeout <= 0 {
		nodeTimeout = 10 * time.Second
	}
	if nodeTimeout > 30*time.Second {
		nodeTimeout = 30 * time.Second
	}

	var allSamples []*MonitorSample
	var sampleMu sync.Mutex
	var successCount int32
	var failCount int32
	var resourceErr error

	// Bounded worker pool
	nodeChan := make(chan MonitoredNode, len(job.Nodes))
	for _, n := range job.Nodes {
		PopulateNodeKeys(&n)
		nodeChan <- n
	}
	close(nodeChan)

	workerCount := r.workerCount
	if workerCount > len(job.Nodes) {
		workerCount = len(job.Nodes)
	}
	if workerCount <= 0 {
		workerCount = 1
	}

	var wg sync.WaitGroup
	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for node := range nodeChan {
				// Check context cancellation before starting probe
				if ctx.Err() != nil {
					return
				}

				nodeSamples, nodeSuccess, nodeErr := r.probeNode(ctx, job.ProfileID, node, targets, nodeTimeout, runID, job.ProbeSet == ProbeSetHeavy, admissionPriorityFor(job))

				sampleMu.Lock()
				allSamples = append(allSamples, nodeSamples...)
				if nodeErr != nil && resourceErr == nil {
					resourceErr = nodeErr
				}
				sampleMu.Unlock()
				if nodeErr != nil {
					return
				}

				if nodeSuccess {
					atomic.AddInt32(&successCount, 1)
				} else {
					atomic.AddInt32(&failCount, 1)
				}
			}
		}()
	}

	wg.Wait()

	finishedAt := time.Now()
	run.FinishedAt = &finishedAt
	run.SuccessNodes = int(successCount)
	run.FailedNodes = int(failCount)

	// Determine run completion status
	if resourceErr != nil {
		run.Status = RunStatusResourceLimited
		run.ErrorMessage = resourceErr.Error()
	} else if ctx.Err() != nil {
		run.Status = RunStatusFailed
		run.ErrorMessage = ctx.Err().Error()
	} else if run.FailedNodes == 0 {
		run.Status = RunStatusCompleted
	} else if run.SuccessNodes > 0 {
		run.Status = RunStatusPartialFailed
	} else {
		run.Status = RunStatusFailed
		run.ErrorMessage = "所有目标节点均无成功证据；未执行与探针失败见目标记录"
	}

	// Persist samples and update run in store. A probe result is not a durable
	// success until both writes report success. Keep the raw identity unchanged
	// while marking any persistence failure explicitly.
	if r.store != nil {
		var persistenceErr error
		if len(allSamples) > 0 {
			if err := r.store.SaveMonitorSamples(ctx, allSamples); err != nil {
				persistenceErr = fmt.Errorf("save monitor samples: %w", err)
			}
		}
		if persistenceErr != nil {
			run.Status = RunStatusPersistenceFailed
			run.ErrorMessage = persistenceErr.Error()
		}
		if err := r.store.UpdateMonitorRun(ctx, run); err != nil {
			updateErr := fmt.Errorf("update monitor run: %w", err)
			if persistenceErr != nil {
				persistenceErr = fmt.Errorf("%v; %w", persistenceErr, updateErr)
			} else {
				persistenceErr = updateErr
			}
			run.Status = RunStatusPersistenceFailed
			run.ErrorMessage = persistenceErr.Error()
		}
		if persistenceErr != nil {
			return run, allSamples, persistenceErr
		}
	}

	if resourceErr != nil {
		return run, allSamples, resourceErr
	}
	return run, allSamples, nil
}

func (r *Runner) probeNode(ctx context.Context, profileID string, node MonitoredNode, targets []TargetSpec, timeout time.Duration, runID string, heavy bool, priority int) ([]*MonitorSample, bool, error) {
	PopulateNodeKeys(&node)

	client, err := r.dialer.CreateClient(node, timeout)
	if err != nil {
		// No request was possible. Preserve every planned target of the new
		// scope instead of manufacturing six service failures.
		if len(targets) == 6 {
			samples := make([]*MonitorSample, 0, len(targets))
			for _, target := range targets {
				samples = append(samples, unexecutedProbe(profileID, node, target, runID, "配置不能建立客户端: "+err.Error()))
			}
			return samples, false, nil
		}
		// Client creation failure (e.g. invalid config)
		sample := &MonitorSample{
			SampleID:            newID("s"),
			RunID:               runID,
			NodeKey:             node.NodeKey,
			NodeIdentityKey:     node.NodeIdentityKey,
			ConfigRevisionKey:   node.ConfigRevisionKey,
			ProfileID:           profileID,
			DisplayNameSnapshot: node.DisplayName,
			ProbeType:           "init",
			Target:              node.Server,
			Timestamp:           time.Now(),
			Success:             false,
			ErrorClass:          "config_error",
			ErrorDetail:         err.Error(),
		}
		return []*MonitorSample{sample}, false, nil
	}
	if budget := r.budget.Load(); budget != nil {
		client = budgetedClient(client, budget, heavy, priority)
	}

	var samples []*MonitorSample
	anySuccess := false

	for _, target := range targets {
		if ctx.Err() != nil {
			if len(targets) == 6 {
				samples = append(samples, unexecutedProbe(profileID, node, target, runID, "上游已取消，本目标未执行"))
				continue
			}
			break
		}

		sample, err := r.executeSingleProbe(ctx, client, profileID, node, target, timeout, runID)
		if err != nil {
			if len(targets) == 6 {
				for _, pending := range targets[len(samples):] {
					samples = append(samples, unexecutedProbe(profileID, node, pending, runID, "资源入场未完成: "+err.Error()))
				}
			}
			return samples, anySuccess, err
		}
		samples = append(samples, sample)
		if sample.Success {
			anySuccess = true
		}
	}

	return samples, anySuccess, nil
}

func (r *Runner) executeSingleProbe(
	parentCtx context.Context,
	client *http.Client,
	profileID string,
	node MonitoredNode,
	target TargetSpec,
	timeout time.Duration,
	runID string,
) (*MonitorSample, error) {
	probeCtx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	sample := &MonitorSample{
		SampleID:            newID("s"),
		RunID:               runID,
		NodeKey:             node.NodeKey,
		NodeIdentityKey:     node.NodeIdentityKey,
		ConfigRevisionKey:   node.ConfigRevisionKey,
		ProfileID:           profileID,
		DisplayNameSnapshot: node.DisplayName,
		ProbeType:           target.ProbeType,
		Target:              target.URL,
		Timestamp:           time.Now(),
		Success:             false,
		ErrorClass:          "none",
	}

	if r.probeActivity != nil {
		finishActivity := r.probeActivity("monitor")
		defer func() {
			peers := finishActivity()
			if sample.Metadata == nil {
				sample.Metadata = map[string]any{}
			}
			sample.Metadata["competition_observation"] = "in_app_only"
			if len(peers) > 0 {
				sample.Metadata["competing_probes"] = peers
				sample.Metadata["quality_caution"] = "shared_resources_overlap_possible"
			}
		}()
	}
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, target.URL, nil)
	if err != nil {
		sample.ErrorClass = "invalid_request"
		sample.ErrorDetail = err.Error()
		return sample, nil
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	var ttfbStart time.Time
	var ttfbDuration time.Duration

	trace := &httptrace.ClientTrace{
		GotFirstResponseByte: func() {
			if !ttfbStart.IsZero() {
				ttfbDuration = time.Since(ttfbStart)
			}
		},
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	startTime := time.Now()
	ttfbStart = startTime

	resp, err := client.Do(req)
	latency := time.Since(startTime)

	if err != nil {
		if _, blocked := AsBudgetBlock(err); blocked {
			return nil, err
		}
		sample.Latency = latency
		sample.ErrorClass = classifyError(err)
		sample.ErrorDetail = err.Error()
		return sample, nil
	}
	defer resp.Body.Close()

	if sample.Metadata == nil {
		sample.Metadata = map[string]any{}
	}
	sample.Metadata["execution_status"] = "executed"
	sample.Metadata["http_status"] = resp.StatusCode
	sample.Metadata["conclusion"] = "http_response_only_not_business_or_unlock"
	// Drain small body up to 64KB
	if _, err := io.CopyN(io.Discard, resp.Body, 64*1024); err != nil {
		if _, blocked := AsBudgetBlock(err); blocked {
			return nil, err
		}
	}

	sample.Latency = latency
	sample.TTFB = ttfbDuration
	if sample.TTFB == 0 {
		sample.TTFB = latency
	}

	if target.ResponseDelayOnly {
		sample.Success = true
		sample.ErrorClass = "none"
		if resp.StatusCode >= 300 {
			sample.ErrorDetail = fmt.Sprintf("HTTP %d；已测得响应时延，业务／解锁未确认", resp.StatusCode)
		}
	} else if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		sample.Success = true
		sample.ErrorClass = "none"
	} else {
		sample.Success = false
		sample.ErrorClass = "http_status_error"
		sample.ErrorDetail = fmt.Sprintf("HTTP status %d", resp.StatusCode)
	}

	return sample, nil
}

func classifyError(err error) string {
	if err == nil {
		return "none"
	}
	errStr := strings.ToLower(err.Error())
	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded") {
		return "timeout"
	}
	if strings.Contains(errStr, "refused") {
		return "conn_refused"
	}
	if strings.Contains(errStr, "no such host") || strings.Contains(errStr, "dns") {
		return "dns_error"
	}
	if strings.Contains(errStr, "tls") || strings.Contains(errStr, "handshake") || strings.Contains(errStr, "certificate") {
		return "tls_error"
	}
	if strings.Contains(errStr, "canceled") {
		return "canceled"
	}
	return "conn_error"
}

func newID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
}

func unexecutedProbe(profile string, node MonitoredNode, target TargetSpec, runID, reason string) *MonitorSample {
	return &MonitorSample{SampleID: newID("s"), RunID: runID, ProfileID: profile, NodeKey: node.NodeKey, NodeIdentityKey: node.NodeIdentityKey, ConfigRevisionKey: node.ConfigRevisionKey, DisplayNameSnapshot: node.DisplayName, ProbeType: target.ProbeType, Target: target.URL, Timestamp: time.Now(), ErrorClass: "not_executed", ErrorDetail: reason, Metadata: map[string]any{"execution_status": "not_executed", "conclusion": "unconfirmed"}}
}
