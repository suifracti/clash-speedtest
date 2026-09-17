package monitor

import (
	"bytes"
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockDialer struct {
	mu        sync.RWMutex
	responses map[string]func(req *http.Request) (*http.Response, error)
}

func (m *mockDialer) CreateClient(node MonitoredNode, timeout time.Duration) (*http.Client, error) {
	m.mu.RLock()
	handler := m.responses[node.NodeKey]
	m.mu.RUnlock()
	transport := &mockRoundTripper{
		handler: handler,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}, nil
}

type mockRoundTripper struct {
	handler func(req *http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.handler != nil {
		return m.handler(req)
	}
	return &http.Response{
		StatusCode: 204,
		Body:       io.NopCloser(bytes.NewReader(nil)),
		Header:     make(http.Header),
	}, nil
}

type mockSampleStore struct {
	mu      sync.RWMutex
	runs    []*MonitorRun
	samples []*MonitorSample
}

func (m *mockSampleStore) SaveMonitorRun(ctx context.Context, run *MonitorRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs = append(m.runs, run)
	return nil
}

func (m *mockSampleStore) UpdateMonitorRun(ctx context.Context, run *MonitorRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, r := range m.runs {
		if r.RunID == run.RunID {
			m.runs[i] = run
			return nil
		}
	}
	m.runs = append(m.runs, run)
	return nil
}

func (m *mockSampleStore) SaveMonitorSamples(ctx context.Context, samples []*MonitorSample) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.samples = append(m.samples, samples...)
	return nil
}

func (m *mockSampleStore) QueryMonitorRuns(ctx context.Context, jobID string, limit int) ([]*MonitorRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*MonitorRun, len(m.runs))
	copy(res, m.runs)
	return res, nil
}

func (m *mockSampleStore) QueryMonitorSamples(ctx context.Context, filter SampleFilter) ([]*MonitorSample, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*MonitorSample, len(m.samples))
	copy(res, m.samples)
	return res, nil
}

func (m *mockSampleStore) GetNodeTimelineSamples(ctx context.Context, nodeKey string, since time.Time) ([]*MonitorSample, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var matched []*MonitorSample
	for _, s := range m.samples {
		if s.NodeKey == nodeKey && !s.Timestamp.Before(since) {
			matched = append(matched, s)
		}
	}
	return matched, nil
}

func (m *mockSampleStore) GetRuns() []*MonitorRun {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*MonitorRun, len(m.runs))
	copy(res, m.runs)
	return res
}

func TestRunner_PartialFailureResilience(t *testing.T) {
	ctx := context.Background()

	dialer := &mockDialer{
		responses: map[string]func(req *http.Request) (*http.Response, error){
			// Node 1: Healthy (204 No Content)
			"nk_node1": func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: 204,
					Body:       io.NopCloser(bytes.NewReader(nil)),
					Header:     make(http.Header),
				}, nil
			},
			// Node 2: Network Timeout
			"nk_node2": func(req *http.Request) (*http.Response, error) {
				return nil, fmt.Errorf("dial tcp 1.2.3.4:443: i/o timeout")
			},
			// Node 3: Connection Refused
			"nk_node3": func(req *http.Request) (*http.Response, error) {
				return nil, fmt.Errorf("connect: connection refused")
			},
		},
	}

	store := &mockSampleStore{}
	runner := NewRunner(RunnerConfig{
		Store:       store,
		Dialer:      dialer,
		WorkerCount: 2,
	})

	job := &MonitorJob{
		ID:        "job_test_partial",
		ProfileID: "prof_1",
		ProbeSet:  ProbeSetLight,
		Timeout:   2 * time.Second,
		Nodes: []MonitoredNode{
			{NodeKey: "nk_node1", DisplayName: "HK-01", Server: "hk1.com", Port: 443},
			{NodeKey: "nk_node2", DisplayName: "US-01", Server: "us1.com", Port: 443},
			{NodeKey: "nk_node3", DisplayName: "SG-01", Server: "sg1.com", Port: 443},
		},
	}

	run, samples, err := runner.ExecuteRun(ctx, job, time.Now())
	if err != nil {
		t.Fatalf("ExecuteRun error: %v", err)
	}

	// 1. Verify Run Status: Partial Failed (1 success, 2 failed)
	if run.Status != RunStatusPartialFailed {
		t.Errorf("expected RunStatusPartialFailed, got %s", run.Status)
	}
	if run.SuccessNodes != 1 {
		t.Errorf("expected 1 success node, got %d", run.SuccessNodes)
	}
	if run.FailedNodes != 2 {
		t.Errorf("expected 2 failed nodes, got %d", run.FailedNodes)
	}

	// 2. Verify Sample counts
	if len(samples) != 3 {
		t.Fatalf("expected 3 samples, got %d", len(samples))
	}

	sampleMap := make(map[string]*MonitorSample)
	for _, s := range samples {
		sampleMap[s.NodeKey] = s
	}

	s1 := sampleMap["nk_node1"]
	if !s1.Success || s1.ErrorClass != "none" {
		t.Errorf("node1 expected success, got: %+v", s1)
	}

	s2 := sampleMap["nk_node2"]
	if s2.Success || s2.ErrorClass != "timeout" {
		t.Errorf("node2 expected timeout error, got: %+v", s2)
	}

	s3 := sampleMap["nk_node3"]
	if s3.Success || s3.ErrorClass != "conn_refused" {
		t.Errorf("node3 expected conn_refused error, got: %+v", s3)
	}
}

func TestRunner_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	dialer := &mockDialer{
		responses: map[string]func(req *http.Request) (*http.Response, error){
			"nk_slow": func(req *http.Request) (*http.Response, error) {
				select {
				case <-req.Context().Done():
					return nil, req.Context().Err()
				case <-time.After(2 * time.Second):
					return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(nil))}, nil
				}
			},
		},
	}

	runner := NewRunner(RunnerConfig{
		Dialer: dialer,
	})

	job := &MonitorJob{
		ID:       "job_cancel",
		ProbeSet: ProbeSetLight,
		Timeout:  5 * time.Second,
		Nodes: []MonitoredNode{
			{NodeKey: "nk_slow", DisplayName: "Slow Node", Server: "slow.com", Port: 443},
		},
	}

	// Cancel context after 50ms
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	run, _, _ := runner.ExecuteRun(ctx, job, time.Now())
	elapsed := time.Since(start)

	if elapsed > 1*time.Second {
		t.Errorf("runner did not respect cancellation promptly, elapsed: %v", elapsed)
	}
	if run.Status != RunStatusFailed {
		t.Errorf("expected RunStatusFailed on cancellation, got %s", run.Status)
	}
}

func TestRunner_ZeroControllerOrSelectNodeProof(t *testing.T) {
	// Parse all Go source files in core/monitor and verify zero references to SelectNode or Controller
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob error: %v", err)
	}

	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}

		node, err := parser.ParseFile(fset, f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse file %s error: %v", f, err)
		}

		for _, imp := range node.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(path, "controller") {
				t.Errorf("SECURITY VIOLATION: %s imports controller package %q", f, path)
			}
			if strings.Contains(path, "wails") {
				t.Errorf("ARCHITECTURE VIOLATION: %s imports wails package %q", f, path)
			}
		}

		// Also check file content for SelectNode call
		content, _ := os.ReadFile(f)
		if strings.Contains(string(content), "SelectNode") {
			t.Errorf("SECURITY VIOLATION: %s contains forbidden SelectNode reference", f)
		}
	}
}
