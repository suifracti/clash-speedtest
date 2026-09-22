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
	Store       SampleStore
	Dialer      NodeDialer
	WorkerCount int
}

// Runner executes one round of probe checks across a job's nodes using an isolated dialer.
type Runner struct {
	store       SampleStore
	dialer      NodeDialer
	workerCount int
}

// NewRunner creates a new Runner instance.
func NewRunner(cfg RunnerConfig) *Runner {
	if cfg.Dialer == nil {
		cfg.Dialer = NewDefaultNodeDialer()
	}
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = 4
	}
	return &Runner{
		store:       cfg.Store,
		dialer:      cfg.Dialer,
		workerCount: cfg.WorkerCount,
	}
}

// TargetSpec defines a probe URL target and its expected probe type.
type TargetSpec struct {
	ProbeType string
	URL       string
}

// GetProbeTargets returns the target probes for a given ProbeSetType.
func GetProbeTargets(pset ProbeSetType) []TargetSpec {
	switch pset {
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
	if job == nil {
		return nil, nil, fmt.Errorf("job is nil")
	}

	runID := newID("run")
	startedAt := time.Now()

	run := &MonitorRun{
		RunID:        runID,
		JobID:        job.ID,
		ScheduledAt:  scheduledAt,
		StartedAt:    startedAt,
		Status:       RunStatusRunning,
		TotalNodes:   len(job.Nodes),
		SuccessNodes: 0,
		FailedNodes:  0,
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

	var allSamples []*MonitorSample
	var sampleMu sync.Mutex
	var successCount int32
	var failCount int32

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

				nodeSamples, nodeSuccess := r.probeNode(ctx, job.ProfileID, node, targets, nodeTimeout, runID)

				sampleMu.Lock()
				allSamples = append(allSamples, nodeSamples...)
				sampleMu.Unlock()

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
	if ctx.Err() != nil {
		run.Status = RunStatusFailed
		run.ErrorMessage = ctx.Err().Error()
	} else if run.FailedNodes == 0 {
		run.Status = RunStatusCompleted
	} else if run.SuccessNodes > 0 {
		run.Status = RunStatusPartialFailed
	} else {
		run.Status = RunStatusFailed
		run.ErrorMessage = "所有目标节点探测均失败"
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

	return run, allSamples, nil
}

func (r *Runner) probeNode(ctx context.Context, profileID string, node MonitoredNode, targets []TargetSpec, timeout time.Duration, runID string) ([]*MonitorSample, bool) {
	PopulateNodeKeys(&node)

	client, err := r.dialer.CreateClient(node, timeout)
	if err != nil {
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
		return []*MonitorSample{sample}, false
	}

	var samples []*MonitorSample
	anySuccess := false

	for _, target := range targets {
		if ctx.Err() != nil {
			break
		}

		sample := r.executeSingleProbe(ctx, client, profileID, node, target, timeout, runID)
		samples = append(samples, sample)
		if sample.Success {
			anySuccess = true
		}
	}

	return samples, anySuccess
}

func (r *Runner) executeSingleProbe(
	parentCtx context.Context,
	client *http.Client,
	profileID string,
	node MonitoredNode,
	target TargetSpec,
	timeout time.Duration,
	runID string,
) *MonitorSample {
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

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, target.URL, nil)
	if err != nil {
		sample.ErrorClass = "invalid_request"
		sample.ErrorDetail = err.Error()
		return sample
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
		sample.Latency = latency
		sample.ErrorClass = classifyError(err)
		sample.ErrorDetail = err.Error()
		return sample
	}
	defer resp.Body.Close()

	// Drain small body up to 64KB
	_, _ = io.CopyN(io.Discard, resp.Body, 64*1024)

	sample.Latency = latency
	sample.TTFB = ttfbDuration
	if sample.TTFB == 0 {
		sample.TTFB = latency
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		sample.Success = true
		sample.ErrorClass = "none"
	} else {
		sample.Success = false
		sample.ErrorClass = "http_status_error"
		sample.ErrorDetail = fmt.Sprintf("HTTP status %d", resp.StatusCode)
	}

	return sample
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
