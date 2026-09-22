package monitor_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
)

type budgetDialer struct {
	roundTrip func(*http.Request) (*http.Response, error)
}

type budgetTransportFunc func(*http.Request) (*http.Response, error)

func (f budgetTransportFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
func (d budgetDialer) CreateClient(monitor.MonitoredNode, time.Duration) (*http.Client, error) {
	return &http.Client{Transport: budgetTransportFunc(d.roundTrip)}, nil
}

func budgetJob(id, key string) *monitor.MonitorJob {
	return &monitor.MonitorJob{
		ID: id, ProfileID: "profile-a", ProbeSet: monitor.ProbeSetLight,
		Timeout: time.Second, Interval: time.Minute,
		Nodes: []monitor.MonitoredNode{{NodeKey: key, NodeIdentityKey: key, DisplayName: key, Server: key + ".invalid", Port: 443}},
	}
}

func budgetResponse(code int, body string, location string) *http.Response {
	header := make(http.Header)
	if location != "" {
		header.Set("Location", location)
	}
	return &http.Response{StatusCode: code, Header: header, Body: io.NopCloser(strings.NewReader(body))}
}

func TestMonitorBudgetSharedRequestsRedirectFailureAndNoFalseRun(t *testing.T) {
	store, err := history.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	limits := monitor.BudgetLimits{MaxConcurrent: 4, DailyRequests: 3, DailyBytes: 10, ResponseBytes: 10}
	budget := monitor.NewBudgetController(store, func() (monitor.BudgetLimits, error) { return limits, nil }, func() time.Time { return now })
	var hops atomic.Int32
	runner := monitor.NewRunner(monitor.RunnerConfig{Store: store, Budget: budget, Dialer: budgetDialer{roundTrip: func(req *http.Request) (*http.Response, error) {
		hops.Add(1)
		switch req.URL.Host {
		case "cp.cloudflare.com":
			return budgetResponse(302, "", "https://redirect.invalid/final"), nil
		case "redirect.invalid":
			return budgetResponse(204, "abcde", ""), nil
		default:
			return nil, errors.New("unreachable")
		}
	}}})
	first, samples, err := runner.ExecuteRun(context.Background(), budgetJob("first", "node-1"), now)
	if err != nil || first.Status != monitor.RunStatusCompleted || len(samples) != 1 {
		t.Fatalf("first run: %+v, %d samples, %v", first, len(samples), err)
	}
	if hops.Load() != 2 {
		t.Fatalf("redirect should use 2 HTTP hops, got %d", hops.Load())
	}
	usage, err := store.MonitorBudgetUsage(context.Background(), "2026-09-23")
	if err != nil || usage.RequestsUsed != 2 || usage.BytesUsed != 5 {
		t.Fatalf("usage after redirect: %+v, %v", usage, err)
	}

	failing := monitor.NewRunner(monitor.RunnerConfig{Store: store, Budget: budget, Dialer: budgetDialer{roundTrip: func(*http.Request) (*http.Response, error) {
		hops.Add(1)
		return nil, errors.New("intentional network failure")
	}}})
	second, samples, err := failing.ExecuteRun(context.Background(), budgetJob("second", "node-2"), now)
	if err != nil || second.Status != monitor.RunStatusFailed || len(samples) != 1 {
		t.Fatalf("failed transport should be saved as network failure: %+v, %d samples, %v", second, len(samples), err)
	}
	third, samples, err := runner.ExecuteRun(context.Background(), budgetJob("third", "node-3"), now)
	if _, blocked := monitor.AsBudgetBlock(err); !blocked || third != nil || len(samples) != 0 {
		t.Fatalf("exhausted budget should not make a run: %+v, %d samples, %v", third, len(samples), err)
	}
	if hops.Load() != 3 {
		t.Fatalf("exhausted budget allowed transport: %d hops", hops.Load())
	}
	runs, err := store.QueryMonitorRuns(context.Background(), "third", 10)
	if err != nil || len(runs) != 0 {
		t.Fatalf("denied round left durable run: %d, %v", len(runs), err)
	}
}

func TestMonitorBudgetBodyCapAndStoreFailure(t *testing.T) {
	store, err := history.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	limits := monitor.BudgetLimits{MaxConcurrent: 4, DailyRequests: 10, DailyBytes: 10, ResponseBytes: 3}
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	budget := monitor.NewBudgetController(store, func() (monitor.BudgetLimits, error) { return limits, nil }, func() time.Time { return now })
	var hops atomic.Int32
	runner := monitor.NewRunner(monitor.RunnerConfig{Store: store, Budget: budget, Dialer: budgetDialer{roundTrip: func(*http.Request) (*http.Response, error) {
		hops.Add(1)
		return budgetResponse(200, "abcde", ""), nil
	}}})
	run, samples, err := runner.ExecuteRun(context.Background(), budgetJob("body-cap", "node-1"), now)
	if _, blocked := monitor.AsBudgetBlock(err); !blocked || run.Status != monitor.RunStatusResourceLimited || run.FailedNodes != 0 || len(samples) != 0 {
		t.Fatalf("body cap misclassified node failure: %+v, %d samples, %v", run, len(samples), err)
	}
	usage, err := store.MonitorBudgetUsage(context.Background(), "2026-09-23")
	if err != nil || usage.BytesUsed != 3 || usage.RequestsUsed != 1 {
		t.Fatalf("body bytes not pre-reserved: %+v, %v", usage, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	_, _, err = runner.ExecuteRun(context.Background(), budgetJob("closed", "node-2"), now)
	if block, ok := monitor.AsBudgetBlock(err); !ok || block.Code != "budget_persistence_failed" {
		t.Fatalf("closed ledger did not fail closed: %v", err)
	}
	if hops.Load() != 1 {
		t.Fatalf("ledger failure allowed HTTP request: %d", hops.Load())
	}
}

func TestMonitorBudgetRedirectCeilingAndHeavyPermit(t *testing.T) {
	store, err := history.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	limits := monitor.BudgetLimits{MaxConcurrent: 4, DailyRequests: 10, DailyBytes: 10, ResponseBytes: 10}
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	budget := monitor.NewBudgetController(store, func() (monitor.BudgetLimits, error) { return limits, nil }, func() time.Time { return now })
	var hops atomic.Int32
	runner := monitor.NewRunner(monitor.RunnerConfig{Store: store, Budget: budget, Dialer: budgetDialer{roundTrip: func(*http.Request) (*http.Response, error) {
		hop := hops.Add(1)
		return budgetResponse(302, "", "https://redirect.invalid/next-"+string(rune('0'+hop))), nil
	}}})
	run, samples, err := runner.ExecuteRun(context.Background(), budgetJob("redirect-limit", "node-1"), now)
	if block, ok := monitor.AsBudgetBlock(err); !ok || block.Code != "redirect_limit" || run.Status != monitor.RunStatusResourceLimited || len(samples) != 0 {
		t.Fatalf("redirect ceiling did not stop the round honestly: %+v, %d samples, %v", run, len(samples), err)
	}
	if hops.Load() != 4 {
		t.Fatalf("3 redirects should allow exactly 4 HTTP hops, got %d", hops.Load())
	}
	usage, err := store.MonitorBudgetUsage(context.Background(), "2026-09-23")
	if err != nil || usage.RequestsUsed != 4 {
		t.Fatalf("redirect hops not individually reserved: %+v, %v", usage, err)
	}

	heavyA, heavyB := budgetJob("heavy-a", "heavy-a"), budgetJob("heavy-b", "heavy-b")
	heavyA.ProbeSet, heavyB.ProbeSet = monitor.ProbeSetHeavy, monitor.ProbeSetHeavy
	_, release, err := budget.AcquireRound(context.Background(), heavyA)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, _, err = budget.AcquireRound(ctx, heavyB)
	if block, ok := monitor.AsBudgetBlock(err); !ok || block.Code != "cancelled" {
		t.Fatalf("second heavy round bypassed single permit: %v", err)
	}
}

func TestMonitorBudgetIdentityAndCancelledWait(t *testing.T) {
	store, err := history.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	limits := monitor.BudgetLimits{MaxConcurrent: 1, DailyRequests: 10, DailyBytes: 10, ResponseBytes: 10}
	budget := monitor.NewBudgetController(store, func() (monitor.BudgetLimits, error) { return limits, nil }, time.Now)
	entered := make(chan struct{})
	unblock := make(chan struct{})
	var hops atomic.Int32
	runner := monitor.NewRunner(monitor.RunnerConfig{Store: store, Budget: budget, Dialer: budgetDialer{roundTrip: func(req *http.Request) (*http.Response, error) {
		hops.Add(1)
		close(entered)
		select {
		case <-unblock:
			return budgetResponse(204, "", ""), nil
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}}})
	firstDone := make(chan error, 1)
	go func() {
		_, _, err := runner.ExecuteRun(context.Background(), budgetJob("first", "shared-node"), time.Now())
		firstDone <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first request did not enter transport")
	}
	_, _, err = runner.ExecuteRun(context.Background(), budgetJob("same-node", "shared-node"), time.Now())
	if block, ok := monitor.AsBudgetBlock(err); !ok || block.Code != "duplicate_node" {
		t.Fatalf("same stable identity was not deduplicated: %v", err)
	}
	waitCtx, cancel := context.WithCancel(context.Background())
	waitDone := make(chan error, 1)
	go func() {
		_, _, err := runner.ExecuteRun(waitCtx, budgetJob("waiting", "other-node"), time.Now())
		waitDone <- err
	}()
	time.Sleep(300 * time.Millisecond)
	cancel()
	select {
	case err := <-waitDone:
		if _, ok := monitor.AsBudgetBlock(err); !ok {
			t.Fatalf("cancelled wait returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("wait cancellation was not bounded")
	}
	if hops.Load() != 1 {
		t.Fatalf("waiting request reached transport: %d", hops.Load())
	}
	close(unblock)
	if err := <-firstDone; err != nil {
		t.Fatalf("first run: %v", err)
	}
	status, err := budget.Status(context.Background())
	if err != nil || status.ActiveRequests != 0 {
		t.Fatalf("request permit leaked: %+v, %v", status, err)
	}
	usage, err := store.MonitorBudgetUsage(context.Background(), time.Now().UTC().Format("2006-01-02"))
	if err != nil || usage.RequestsUsed != 1 {
		t.Fatalf("cancelled wait consumed request: %+v, %v", usage, err)
	}
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if _, err := budget.Status(cancelled); err == nil {
		t.Fatal("cancelled budget read unexpectedly succeeded")
	}
	if _, err := budget.Status(context.Background()); err != nil {
		t.Fatalf("cancelled read poisoned the durable ledger: %v", err)
	}
}
