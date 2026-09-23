package monitor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// BudgetLimits are product limits for this AppService's Monitor jobs only.
type BudgetLimits struct {
	MaxConcurrent int   `json:"max_concurrent"`
	DailyRequests int64 `json:"daily_requests"`
	DailyBytes    int64 `json:"daily_bytes"`
	ResponseBytes int64 `json:"response_bytes"`
}

type BudgetUsage struct {
	UTCDay       string `json:"utc_day"`
	RequestsUsed int64  `json:"requests_used"`
	BytesUsed    int64  `json:"bytes_used"`
}

type BudgetStatus struct {
	Limits         BudgetLimits `json:"limits"`
	Usage          BudgetUsage  `json:"usage"`
	ResetAt        time.Time    `json:"reset_at"`
	ActiveRequests int          `json:"active_requests"`
	BlockedReason  string       `json:"blocked_reason,omitempty"`
	BlockedCode    string       `json:"blocked_code,omitempty"`
}

// BudgetLedger must commit a reservation before a request or body read proceeds.
// A failed refund may conservatively overcount, never grant unrecorded quota.
type BudgetLedger interface {
	MonitorBudgetUsage(context.Context, string) (BudgetUsage, error)
	ReserveMonitorRequest(context.Context, string, int64) (BudgetUsage, error)
	RefundMonitorRequest(context.Context, string) error
	ReserveMonitorBytes(context.Context, string, int64, int64) (string, int64, error)
	RefundMonitorBytes(context.Context, string, int64) error
}

type BudgetBlockError struct {
	Code   string
	Reason string
}

func (e *BudgetBlockError) Error() string { return e.Reason }

func AsBudgetBlock(err error) (*BudgetBlockError, bool) {
	var block *BudgetBlockError
	return block, errors.As(err, &block)
}

func budgetBlock(code, reason string) error { return &BudgetBlockError{Code: code, Reason: reason} }

func budgetContextError(ctx context.Context, err error) error {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return budgetBlock("cancelled", "Monitor 请求等待已取消")
	}
	return nil
}

const (
	budgetStagger                = 100 * time.Millisecond
	budgetMaxRoundWait           = 5 * time.Second
	budgetMaxPermitWait          = 5 * time.Second
	sparseFairnessGrantLimit     = 4
	diagnosticFairnessGrantLimit = 3
)

type admissionWaiter struct {
	priority int
	heavy    bool
	ready    chan struct{}
	granted  bool
}

// BudgetController is shared by every Monitor runner/scheduler in one AppService.
// SQLite remains the durable authority for usage across restarts.
type BudgetController struct {
	ledger                BudgetLedger
	limits                func() (BudgetLimits, error)
	now                   func() time.Time
	mu                    sync.Mutex
	active                int
	heavy                 int
	identity              map[string]bool
	nextRound             time.Time
	waiters               []*admissionWaiter
	nonSparseGrantStreak  int
	diagnosticGrantStreak int
	storeFailure          error
}

func NewBudgetController(ledger BudgetLedger, limits func() (BudgetLimits, error), now func() time.Time) *BudgetController {
	if now == nil {
		now = time.Now
	}
	return &BudgetController{ledger: ledger, limits: limits, now: now, identity: make(map[string]bool)}
}

func (b *BudgetController) failStore(err error) error {
	b.mu.Lock()
	if b.storeFailure == nil {
		b.storeFailure = err
	}
	b.mu.Unlock()
	return budgetBlock("budget_persistence_failed", fmt.Sprintf("Monitor 预算记录不可用，已停止新增请求：%v", err))
}

func (b *BudgetController) failure() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.storeFailure != nil {
		return budgetBlock("budget_persistence_failed", fmt.Sprintf("Monitor 预算记录不可用，已停止新增请求：%v", b.storeFailure))
	}
	return nil
}

func (b *BudgetController) config() (BudgetLimits, error) {
	if b == nil || b.ledger == nil || b.limits == nil {
		return BudgetLimits{}, budgetBlock("budget_unavailable", "Monitor 全局预算未初始化")
	}
	if err := b.failure(); err != nil {
		return BudgetLimits{}, err
	}
	limits, err := b.limits()
	if err != nil {
		return BudgetLimits{}, budgetBlock("budget_settings_invalid", fmt.Sprintf("无法读取 Monitor 预算设置：%v", err))
	}
	if limits.MaxConcurrent <= 0 || limits.DailyRequests <= 0 || limits.DailyBytes <= 0 || limits.ResponseBytes <= 0 || limits.ResponseBytes > limits.DailyBytes {
		return BudgetLimits{}, budgetBlock("budget_settings_invalid", "Monitor 预算设置无效")
	}
	return limits, nil
}

func budgetDay(now time.Time) string { return now.UTC().Format("2006-01-02") }

func (b *BudgetController) Status(ctx context.Context) (BudgetStatus, error) {
	limits, err := b.config()
	if err != nil {
		return BudgetStatus{}, err
	}
	usage, err := b.ledger.MonitorBudgetUsage(ctx, budgetDay(b.now()))
	if err != nil {
		if cancelled := budgetContextError(ctx, err); cancelled != nil {
			return BudgetStatus{}, cancelled
		}
		return BudgetStatus{}, b.failStore(err)
	}
	day, err := time.Parse("2006-01-02", usage.UTCDay)
	if err != nil {
		return BudgetStatus{}, b.failStore(err)
	}
	b.mu.Lock()
	active := b.active
	b.mu.Unlock()
	status := BudgetStatus{Limits: limits, Usage: usage, ActiveRequests: active, ResetAt: day.AddDate(0, 0, 1)}
	if usage.RequestsUsed >= limits.DailyRequests {
		status.BlockedCode, status.BlockedReason = "requests_exhausted", "Monitor 今日请求额度已用尽，等待 UTC 次日重置或明确提高上限"
	} else if usage.BytesUsed >= limits.DailyBytes {
		status.BlockedCode, status.BlockedReason = "bytes_exhausted", "Monitor 今日响应体读取额度已用尽，等待 UTC 次日重置或明确提高上限"
	}
	return status, nil
}

func roundIdentities(job *MonitorJob) []string {
	seen := make(map[string]bool)
	keys := make([]string, 0, len(job.Nodes))
	for _, node := range job.Nodes {
		PopulateNodeKeys(&node)
		key := node.NodeIdentityKey
		if key == "" {
			key = node.NodeKey
		}
		if key == "" {
			continue
		}
		key = job.ProfileID + "\x00" + key
		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	return keys
}

// AcquireRound gives each job at most one staggered admission and rejects
// duplicate stable identities. It never creates a durable run on rejection.
func (b *BudgetController) AcquireRound(ctx context.Context, job *MonitorJob) (context.Context, func(), error) {
	status, err := b.Status(ctx)
	if err != nil {
		return nil, nil, err
	}
	if status.BlockedCode != "" {
		return nil, nil, budgetBlock(status.BlockedCode, status.BlockedReason)
	}
	deadline := b.now().Add(budgetMaxRoundWait)
	b.mu.Lock()
	start := b.now()
	if b.nextRound.After(start) {
		start = b.nextRound
	}
	if start.After(deadline) {
		b.mu.Unlock()
		return nil, nil, budgetBlock("wait_expired", "Monitor 全局错峰等待已过期，本周期已跳过")
	}
	b.nextRound = start.Add(budgetStagger)
	b.mu.Unlock()
	if wait := start.Sub(b.now()); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, nil, budgetBlock("cancelled", "Monitor 等待已取消，本周期未探测")
		case <-timer.C:
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, budgetBlock("cancelled", "Monitor 等待已取消，本周期未探测")
	}
	keys := roundIdentities(job)
	b.mu.Lock()
	for _, key := range keys {
		if b.identity[key] {
			b.mu.Unlock()
			return nil, nil, budgetBlock("duplicate_node", "同一稳定节点正在其它 Monitor 任务中探测，本周期已跳过")
		}
	}
	for _, key := range keys {
		b.identity[key] = true
	}
	b.mu.Unlock()
	releaseIdentity := func() {
		b.mu.Lock()
		for _, key := range keys {
			delete(b.identity, key)
		}
		b.mu.Unlock()
	}
	// Reserve the first HTTP hop before the run is recorded. A denied or
	// expired wait must not leave an empty durable run that looks like a probe.
	priority := admissionPriorityFor(job)
	releaseRequest, responseMax, reservedDay, err := b.acquireRequest(ctx, job.ProbeSet == ProbeSetHeavy, priority)
	if err != nil {
		releaseIdentity()
		return nil, nil, err
	}
	credit := &roundCredit{release: releaseRequest, responseMax: responseMax}
	return context.WithValue(ctx, roundCreditKey{}, credit), func() {
		if !credit.used.Load() {
			refundCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			if err := b.ledger.RefundMonitorRequest(refundCtx, reservedDay); err != nil {
				_ = b.failStore(err)
			}
			cancel()
		}
		releaseRequest()
		releaseIdentity()
	}, nil
}

type roundCreditKey struct{}

type roundCredit struct {
	used        atomic.Bool
	release     func()
	responseMax int64
}

func admissionPriorityFor(job *MonitorJob) int {
	if job == nil || job.NextRunTrigger == SamplingTriggerManual || job.SamplingTier == SamplingTierDiagnostic {
		return 2
	}
	if job.SamplingTier == SamplingTierSparse {
		return 0
	}
	return 1
}

func (b *BudgetController) chooseWaiterLocked() int {
	contains := func(priority int) bool {
		for _, waiter := range b.waiters {
			if waiter.priority == priority {
				return true
			}
		}
		return false
	}
	findEligible := func(priority int) int {
		for i, waiter := range b.waiters {
			if waiter.priority == priority && (!waiter.heavy || b.heavy == 0) {
				return i
			}
		}
		return -1
	}

	if contains(0) && b.nonSparseGrantStreak >= sparseFairnessGrantLimit {
		if index := findEligible(0); index >= 0 {
			return index
		}
	}
	if contains(1) && contains(2) && b.diagnosticGrantStreak >= diagnosticFairnessGrantLimit {
		if index := findEligible(1); index >= 0 {
			return index
		}
	}
	for priority := 2; priority >= 0; priority-- {
		if index := findEligible(priority); index >= 0 {
			return index
		}
	}
	return -1
}

func (b *BudgetController) dispatchWaitersLocked(limits BudgetLimits) {
	for b.active < limits.MaxConcurrent && len(b.waiters) > 0 {
		index := b.chooseWaiterLocked()
		if index < 0 {
			return
		}
		waiter := b.waiters[index]
		waitingSparse, waitingNormal, waitingDiagnostic := false, false, false
		for _, pending := range b.waiters {
			switch pending.priority {
			case 0:
				waitingSparse = true
			case 1:
				waitingNormal = true
			case 2:
				waitingDiagnostic = true
			}
		}
		b.waiters = append(b.waiters[:index], b.waiters[index+1:]...)
		b.active++
		if waiter.heavy {
			b.heavy++
		}
		waiter.granted = true
		close(waiter.ready)

		if waitingSparse {
			if waiter.priority == 0 {
				b.nonSparseGrantStreak = 0
			} else {
				b.nonSparseGrantStreak++
			}
		} else {
			b.nonSparseGrantStreak = 0
		}
		if waitingNormal && waitingDiagnostic {
			if waiter.priority == 2 {
				b.diagnosticGrantStreak++
			} else if waiter.priority == 1 {
				b.diagnosticGrantStreak = 0
			}
		} else {
			b.diagnosticGrantStreak = 0
		}
	}
}

func (b *BudgetController) removeWaiter(waiter *admissionWaiter, limits BudgetLimits) {
	b.mu.Lock()
	if waiter.granted {
		b.active--
		if waiter.heavy {
			b.heavy--
		}
		waiter.granted = false
	} else {
		for i, pending := range b.waiters {
			if pending == waiter {
				b.waiters = append(b.waiters[:i], b.waiters[i+1:]...)
				break
			}
		}
	}
	b.dispatchWaitersLocked(limits)
	b.mu.Unlock()
}

func (b *BudgetController) acquireRequest(ctx context.Context, heavy bool, priority int) (func(), int64, string, error) {
	deadline := time.NewTimer(budgetMaxPermitWait)
	defer deadline.Stop()
	if err := ctx.Err(); err != nil {
		return nil, 0, "", budgetBlock("cancelled", "Monitor 请求等待已取消")
	}
	limits, err := b.config()
	if err != nil {
		return nil, 0, "", err
	}
	waiter := &admissionWaiter{priority: priority, heavy: heavy, ready: make(chan struct{})}
	b.mu.Lock()
	b.waiters = append(b.waiters, waiter)
	b.dispatchWaitersLocked(limits)
	b.mu.Unlock()
	select {
	case <-waiter.ready:
	case <-ctx.Done():
		b.removeWaiter(waiter, limits)
		return nil, 0, "", budgetBlock("cancelled", "Monitor 请求等待已取消")
	case <-deadline.C:
		b.removeWaiter(waiter, limits)
		return nil, 0, "", budgetBlock("wait_expired", "Monitor 全局并发等待已过期，本请求未发出")
	}
	if err := ctx.Err(); err != nil {
		b.removeWaiter(waiter, limits)
		return nil, 0, "", budgetBlock("cancelled", "Monitor 请求等待已取消")
	}
	release := sync.OnceFunc(func() {
		b.mu.Lock()
		b.active--
		if heavy {
			b.heavy--
		}
		b.dispatchWaitersLocked(limits)
		b.mu.Unlock()
	})
	if err := b.failure(); err != nil {
		release()
		return nil, 0, "", err
	}
	usage, err := b.ledger.ReserveMonitorRequest(ctx, budgetDay(b.now()), limits.DailyRequests)
	if err != nil {
		release()
		if cancelled := budgetContextError(ctx, err); cancelled != nil {
			return nil, 0, "", cancelled
		}
		if block, ok := AsBudgetBlock(err); ok {
			return nil, 0, "", block
		}
		return nil, 0, "", b.failStore(err)
	}
	return release, limits.ResponseBytes, usage.UTCDay, nil
}

func (b *BudgetController) reserveBytes(ctx context.Context, n int64) (string, int64, error) {
	limits, err := b.config()
	if err != nil {
		return "", 0, err
	}
	day, granted, err := b.ledger.ReserveMonitorBytes(ctx, budgetDay(b.now()), n, limits.DailyBytes)
	if err != nil {
		if cancelled := budgetContextError(ctx, err); cancelled != nil {
			return "", 0, cancelled
		}
		if block, ok := AsBudgetBlock(err); ok {
			return "", 0, block
		}
		return "", 0, b.failStore(err)
	}
	return day, granted, nil
}

type budgetTransport struct {
	base     http.RoundTripper
	budget   *BudgetController
	heavy    bool
	priority int
}

func (t *budgetTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var release func()
	var responseMax int64
	credit, ok := req.Context().Value(roundCreditKey{}).(*roundCredit)
	if ok && !credit.used.Swap(true) {
		release, responseMax = credit.release, credit.responseMax
	} else {
		var err error
		release, responseMax, _, err = t.budget.acquireRequest(req.Context(), t.heavy, t.priority)
		if err != nil {
			return nil, err
		}
	}
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		release()
		return nil, err
	}
	if resp == nil || resp.Body == nil {
		release()
		return resp, nil
	}
	resp.Body = &budgetBody{ReadCloser: resp.Body, budget: t.budget, ctx: req.Context(), release: release, remaining: responseMax}
	return resp, nil
}

type budgetBody struct {
	io.ReadCloser
	budget    *BudgetController
	ctx       context.Context
	release   func()
	remaining int64
	closed    sync.Once
}

func (b *budgetBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if b.remaining <= 0 {
		return 0, budgetBlock("response_limit", "Monitor 单响应体读取上限已达到")
	}
	want := int64(len(p))
	if want > 4096 {
		want = 4096
	}
	if want > b.remaining {
		want = b.remaining
	}
	day, reserved, err := b.budget.reserveBytes(b.ctx, want)
	if err != nil {
		return 0, err
	}
	n, readErr := b.ReadCloser.Read(p[:reserved])
	b.remaining -= int64(n)
	if int64(n) < reserved {
		refundCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := b.budget.ledger.RefundMonitorBytes(refundCtx, day, reserved-int64(n)); err != nil {
			return 0, b.budget.failStore(err)
		}
	}
	return n, readErr
}

func (b *budgetBody) Close() error {
	b.closed.Do(b.release)
	return b.ReadCloser.Close()
}

func budgetedClient(client *http.Client, budget *BudgetController, heavy bool, priority int) *http.Client {
	copy := *client
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	copy.Transport = &budgetTransport{base: base, budget: budget, heavy: heavy, priority: priority}
	previousRedirect := client.CheckRedirect
	copy.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 {
			return budgetBlock("redirect_limit", "Monitor HTTP 重定向超过 3 次")
		}
		if previousRedirect != nil {
			return previousRedirect(req, via)
		}
		return nil
	}
	if copy.Timeout <= 0 || copy.Timeout > 30*time.Second {
		copy.Timeout = 30 * time.Second
	}
	return &copy
}
