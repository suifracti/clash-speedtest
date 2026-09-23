package monitor

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

type fairnessLedger struct {
	mu    sync.Mutex
	usage BudgetUsage
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
