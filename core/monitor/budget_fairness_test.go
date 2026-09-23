package monitor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

type fairnessLedger struct {
	mu    sync.Mutex
	usage BudgetUsage
}

type mutableBudgetConfig struct {
	mu     sync.RWMutex
	limits BudgetLimits
	err    error
}

func (c *mutableBudgetConfig) read() (BudgetLimits, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.limits, c.err
}

func (c *mutableBudgetConfig) set(limits BudgetLimits, err error) {
	c.mu.Lock()
	c.limits = limits
	c.err = err
	c.mu.Unlock()
}

func (l *fairnessLedger) MonitorBudgetUsage(_ context.Context, day string) (BudgetUsage, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.usage.UTCDay != day {
		return BudgetUsage{UTCDay: day}, nil
	}
	return l.usage, nil
}

func (l *fairnessLedger) ReserveMonitorRequest(_ context.Context, day string, limit int64) (BudgetUsage, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.usage.UTCDay != day {
		l.usage = BudgetUsage{UTCDay: day}
	}
	if l.usage.RequestsUsed >= limit {
		return l.usage, budgetBlock("requests_exhausted", "test daily limit exhausted")
	}
	l.usage.RequestsUsed++
	return l.usage, nil
}

func (l *fairnessLedger) RefundMonitorRequest(_ context.Context, day string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.usage.UTCDay == day && l.usage.RequestsUsed > 0 {
		l.usage.RequestsUsed--
	}
	return nil
}

func (l *fairnessLedger) ReserveMonitorBytes(_ context.Context, day string, want, limit int64) (string, int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.usage.UTCDay != day {
		l.usage = BudgetUsage{UTCDay: day}
	}
	if remaining := limit - l.usage.BytesUsed; want > remaining {
		want = remaining
	}
	if want < 0 {
		want = 0
	}
	l.usage.BytesUsed += want
	return day, want, nil
}

func (l *fairnessLedger) RefundMonitorBytes(_ context.Context, day string, n int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.usage.UTCDay == day {
		l.usage.BytesUsed -= n
		if l.usage.BytesUsed < 0 {
			l.usage.BytesUsed = 0
		}
	}
	return nil
}

func TestBudgetAdmissionPriorityAndBoundedSparseFairness(t *testing.T) {
	limits := BudgetLimits{MaxConcurrent: 1, DailyRequests: 50, DailyBytes: 1000, ResponseBytes: 100}
	ledger := &fairnessLedger{}
	b := NewBudgetController(ledger, func() (BudgetLimits, error) { return limits, nil }, func() time.Time {
		return time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	})
	initialRelease, _, _, err := b.acquireRequest(context.Background(), false, 1)
	if err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		label string
		err   error
	}
	order := make(chan outcome, 10)
	addWaiter := func(label string, priority int, expected int) {
		t.Helper()
		go func() {
			release, _, _, err := b.acquireRequest(context.Background(), false, priority)
			order <- outcome{label: label, err: err}
			if err == nil {
				release()
			}
		}()
		waitForBudgetWaiters(t, b, expected)
	}
	addWaiter("sparse", 0, 1)
	addWaiter("focus", 1, 2)
	for i := 0; i < 8; i++ {
		addWaiter(fmt.Sprintf("diagnostic-%d", i), 2, i+3)
	}
	initialRelease()

	got := make([]string, 0, 10)
	for range 10 {
		select {
		case result := <-order:
			if result.err != nil {
				t.Fatalf("queued %s request failed: %v", result.label, result.err)
			}
			got = append(got, result.label)
		case <-time.After(2 * time.Second):
			t.Fatalf("admissions stalled with remaining waiters: %v", got)
		}
	}
	if len(got) < 5 || got[0] != "diagnostic-0" || got[1] != "diagnostic-1" || got[2] != "diagnostic-2" {
		t.Fatalf("manual diagnostics did not receive real priority: %v", got)
	}
	firstFive := map[string]bool{}
	for _, label := range got[:5] {
		firstFive[label] = true
	}
	if !firstFive["focus"] || !firstFive["sparse"] {
		t.Fatalf("lower tiers starved despite continuing diagnostic arrivals: %v", got)
	}
	usage, err := ledger.MonitorBudgetUsage(context.Background(), "2026-09-23")
	if err != nil || usage.RequestsUsed != 11 {
		t.Fatalf("requests did not share one budget ledger: usage=%+v err=%v", usage, err)
	}
	status, err := b.Status(context.Background())
	if err != nil || status.ActiveRequests != 0 {
		t.Fatalf("admission permit leaked: status=%+v err=%v", status, err)
	}
}

func TestBudgetAdmissionCancellationRemovesWaiterWithoutRequest(t *testing.T) {
	limits := BudgetLimits{MaxConcurrent: 1, DailyRequests: 10, DailyBytes: 100, ResponseBytes: 10}
	ledger := &fairnessLedger{}
	b := NewBudgetController(ledger, func() (BudgetLimits, error) { return limits, nil }, time.Now)
	hold, _, _, err := b.acquireRequest(context.Background(), false, 1)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, _, _, err := b.acquireRequest(ctx, false, 0)
		done <- err
	}()
	waitForBudgetWaiters(t, b, 1)
	cancel()
	select {
	case err := <-done:
		if block, ok := AsBudgetBlock(err); !ok || block.Code != "cancelled" {
			t.Fatalf("cancelled admission returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled admission did not return promptly")
	}
	hold()
	usage, err := ledger.MonitorBudgetUsage(context.Background(), budgetDay(time.Now()))
	if err != nil || usage.RequestsUsed != 1 {
		t.Fatalf("cancelled waiter consumed a request: usage=%+v err=%v", usage, err)
	}
	status, err := b.Status(context.Background())
	if err != nil || status.ActiveRequests != 0 {
		t.Fatalf("cancelled admission leaked permit: status=%+v err=%v", status, err)
	}
}

func TestBudgetAdmissionUsesReducedConcurrencyForQueuedWaiters(t *testing.T) {
	limits := BudgetLimits{MaxConcurrent: 4, DailyRequests: 20, DailyBytes: 100, ResponseBytes: 10}
	config := &mutableBudgetConfig{limits: limits}
	ledger := &fairnessLedger{}
	b := NewBudgetController(ledger, config.read, func() time.Time {
		return time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	})
	var held []func()
	for i := 0; i < 4; i++ {
		release, _, _, err := b.acquireRequest(context.Background(), false, 1)
		if err != nil {
			t.Fatalf("initial request %d failed: %v", i, err)
		}
		held = append(held, release)
	}
	type outcome struct {
		release func()
		err     error
	}
	result := make(chan outcome, 1)
	go func() {
		release, _, _, err := b.acquireRequest(context.Background(), false, 1)
		result <- outcome{release: release, err: err}
	}()
	waitForBudgetWaiters(t, b, 1)

	limits.MaxConcurrent = 1
	config.set(limits, nil)
	for i := 0; i < 3; i++ {
		held[i]()
	}
	assertBudgetAdmissionState(t, b, 1, 1)
	select {
	case got := <-result:
		if got.release != nil {
			got.release()
		}
		t.Fatalf("waiter was admitted while one request still met the reduced limit: err=%v", got.err)
	default:
	}

	held[3]()
	select {
	case got := <-result:
		if got.err != nil {
			t.Fatalf("waiter failed after concurrency became available: %v", got.err)
		}
		got.release()
	case <-time.After(time.Second):
		t.Fatal("waiter was not admitted after active requests fell below the reduced limit")
	}
	assertBudgetAdmissionState(t, b, 0, 0)
}

func TestBudgetAdmissionUsesReducedDailyLimitForQueuedWaiter(t *testing.T) {
	limits := BudgetLimits{MaxConcurrent: 1, DailyRequests: 10, DailyBytes: 100, ResponseBytes: 10}
	config := &mutableBudgetConfig{limits: limits}
	ledger := &fairnessLedger{}
	b := NewBudgetController(ledger, config.read, func() time.Time {
		return time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	})
	hold, _, _, err := b.acquireRequest(context.Background(), false, 1)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		release, _, _, err := b.acquireRequest(context.Background(), false, 1)
		if err == nil {
			release()
		}
		result <- err
	}()
	waitForBudgetWaiters(t, b, 1)

	limits.DailyRequests = 1
	config.set(limits, nil)
	hold()
	select {
	case err := <-result:
		block, ok := AsBudgetBlock(err)
		if !ok || block.Code != "requests_exhausted" {
			t.Fatalf("queued request ignored the reduced daily limit: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued request did not finish after acquiring the available concurrency permit")
	}
	usage, err := ledger.MonitorBudgetUsage(context.Background(), "2026-09-23")
	if err != nil || usage.RequestsUsed != 1 {
		t.Fatalf("rejected waiter consumed request quota: usage=%+v err=%v", usage, err)
	}
	assertBudgetAdmissionState(t, b, 0, 0)
}

func TestBudgetAdmissionDoesNotUseStaleLimitsAfterConfigReadFailure(t *testing.T) {
	limits := BudgetLimits{MaxConcurrent: 1, DailyRequests: 10, DailyBytes: 100, ResponseBytes: 10}
	config := &mutableBudgetConfig{limits: limits}
	ledger := &fairnessLedger{}
	b := NewBudgetController(ledger, config.read, func() time.Time {
		return time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	})
	hold, _, _, err := b.acquireRequest(context.Background(), false, 1)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, _, _, err := b.acquireRequest(ctx, false, 1)
		result <- err
	}()
	waitForBudgetWaiters(t, b, 1)

	config.set(limits, errors.New("settings unavailable"))
	hold()
	assertBudgetAdmissionState(t, b, 0, 1)
	cancel()
	select {
	case err := <-result:
		if block, ok := AsBudgetBlock(err); !ok || block.Code != "cancelled" {
			t.Fatalf("queued waiter returned %v after cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued waiter did not cancel promptly")
	}
	usage, err := ledger.MonitorBudgetUsage(context.Background(), "2026-09-23")
	if err != nil || usage.RequestsUsed != 1 {
		t.Fatalf("config read failure allowed a stale request reservation: usage=%+v err=%v", usage, err)
	}
	assertBudgetAdmissionState(t, b, 0, 0)
}

func assertBudgetAdmissionState(t *testing.T, b *BudgetController, active, waiters int) {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.active != active || len(b.waiters) != waiters {
		t.Fatalf("unexpected admission state: active=%d waiters=%d, want active=%d waiters=%d", b.active, len(b.waiters), active, waiters)
	}
}

func waitForBudgetWaiters(t *testing.T, b *BudgetController, count int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		b.mu.Lock()
		got := len(b.waiters)
		b.mu.Unlock()
		if got == count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("waiter count did not reach %d", count)
}
