package monitor

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// SchedulerConfig configures a MonitorJob Scheduler.
type SchedulerConfig struct {
	Job    *MonitorJob
	Runner *Runner
	Store  SampleStore
	// StorageGuard returns a reason while new background collection is unsafe.
	StorageGuard func() string
	BudgetGuard  func() (string, string)
}

// Scheduler manages the periodic execution and lifecycle of a MonitorJob.
type Scheduler struct {
	mu                    sync.Mutex
	job                   *MonitorJob
	runner                *Runner
	store                 SampleStore
	storageGuard          func() string
	budgetGuard           func() (string, string)
	pendingCancel         context.CancelFunc
	pendingID             uint64
	recoveryPending       bool
	state                 JobState
	stopping              bool
	ctx                   context.Context
	cancel                context.CancelFunc
	loopWg                sync.WaitGroup
	runWg                 sync.WaitGroup
	isExecuting           int32 // atomic flag: 1 if runner is active, 0 otherwise
	skippedRounds         int64 // atomic counter for overlapped ticks skipped
	resourceSkippedRounds int64
	completedRuns         int64 // atomic counter for finished runs
}

// NewScheduler creates a Scheduler for the specified job.
func NewScheduler(cfg SchedulerConfig) (*Scheduler, error) {
	if cfg.Job == nil {
		return nil, fmt.Errorf("job is nil")
	}
	if cfg.Runner == nil {
		return nil, fmt.Errorf("runner is nil")
	}
	if cfg.Job.Interval <= 0 {
		cfg.Job.Interval = 5 * time.Minute
	}
	if cfg.Job.SamplingTier == "" {
		cfg.Job.SamplingTier = SamplingTierRegular
	}

	s := &Scheduler{
		job:          cfg.Job,
		runner:       cfg.Runner,
		store:        cfg.Store,
		storageGuard: cfg.StorageGuard,
		budgetGuard:  cfg.BudgetGuard,
		state:        cfg.Job.State,
	}
	if s.job.PersistenceState == "" {
		s.job.PersistenceState = PersistenceStateHealthy
	}
	if s.state != JobStateBlocked {
		s.state = JobStateStopped
		s.job.State = JobStateStopped
	}
	return s, nil
}

// State returns the current lifecycle state of the scheduler.
func (s *Scheduler) State() JobState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Job returns a copy of the underlying MonitorJob.
func (s *Scheduler) Job() MonitorJob {
	s.checkStorage()
	s.checkBudget()
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *s.job
	copy.SkippedRounds = atomic.LoadInt64(&s.skippedRounds)
	copy.ResourceSkippedRounds = atomic.LoadInt64(&s.resourceSkippedRounds)
	return copy
}

// UpdateSamplingTier changes only the periodic sampling tier. It does not
// change lifecycle state or start a stopped/paused scheduler.
func (s *Scheduler) UpdateSamplingTier(tier SamplingTier, updatedAt time.Time) error {
	if tier != SamplingTierRegular && tier != SamplingTierFocus && tier != SamplingTierSparse {
		return fmt.Errorf("unsupported periodic monitor sampling tier %q", tier)
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.job.SamplingTier = tier
	s.job.UpdatedAt = updatedAt
	return nil
}

func (s *Scheduler) SetRunner(runner *Runner) error {
	if runner == nil {
		return fmt.Errorf("runner is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == JobStateRunning || s.stopping || atomic.LoadInt32(&s.isExecuting) != 0 {
		return fmt.Errorf("cannot replace runner while monitor job is active")
	}
	s.runner = runner
	return nil
}

// SetLaunchIntent updates the durable user intent as reflected by the current
// process. Callers persist these values before reporting success.
func (s *Scheduler) SetLaunchIntent(resumeOnLaunch bool, desired JobState, recoveryState RecoveryState, recoveryReason, persistenceError string) {
	s.mu.Lock()
	s.job.ResumeOnLaunch = resumeOnLaunch
	s.job.DesiredState = desired
	s.job.RecoveryState = recoveryState
	s.job.RecoveryReason = recoveryReason
	s.job.IntentPersistenceError = persistenceError
	s.mu.Unlock()
}

// UpdateRecoveryResolution refreshes nodes from the current profile/cache
// before startup recovery. It never remaps by display name.
func (s *Scheduler) UpdateRecoveryResolution(nodes []MonitoredNode, blockedReason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == JobStateRunning || s.stopping || atomic.LoadInt32(&s.isExecuting) != 0 {
		return fmt.Errorf("cannot refresh a monitor job while it is active")
	}
	if blockedReason != "" {
		s.state = JobStateBlocked
		s.job.State = JobStateBlocked
		s.job.BlockedReason = blockedReason
		s.job.RecoveryState = RecoveryStateBlocked
		s.job.RecoveryReason = blockedReason
		return nil
	}
	s.job.Nodes = append([]MonitoredNode(nil), nodes...)
	s.job.NodeKeys = make([]string, len(nodes))
	for i, node := range nodes {
		s.job.NodeKeys[i] = node.NodeKey
	}
	s.job.BlockedReason = ""
	if s.state == JobStateBlocked {
		s.state = JobStateStopped
		s.job.State = JobStateStopped
	}
	return nil
}

// CancelPendingAdmission invalidates a not-yet-admitted scheduled round while
// leaving an already-running scheduler and admitted request alone.
func (s *Scheduler) CancelPendingAdmission() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pendingCancel == nil {
		return false
	}
	s.pendingCancel()
	return true
}

// CancelPendingRecoveryAdmission cancels only the first restored round before
// it is admitted. Later periodic work and already-admitted requests continue.
func (s *Scheduler) CancelPendingRecoveryAdmission() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.recoveryPending || s.pendingCancel == nil {
		return false
	}
	s.recoveryPending = false
	if s.state == JobStateRunning {
		s.job.RecoveryState = RecoveryStateActive
		s.job.RecoveryReason = ""
	}
	s.pendingCancel()
	return true
}

// StartRecovered marks the initial scheduled round as restart recovery while
// using the scheduler's existing fair admission and cancellation path.
func (s *Scheduler) StartRecovered(parentCtx context.Context) error {
	s.mu.Lock()
	if s.state == JobStateRunning {
		s.mu.Unlock()
		return nil
	}
	s.recoveryPending = true
	s.mu.Unlock()
	if err := s.Start(parentCtx); err != nil {
		s.mu.Lock()
		s.recoveryPending = false
		s.mu.Unlock()
		return err
	}
	return nil
}

func (s *Scheduler) checkBudget() (string, string) {
	if s.budgetGuard == nil {
		return "", ""
	}
	code, reason := s.budgetGuard()
	s.mu.Lock()
	if code != "" {
		s.job.BudgetState, s.job.BudgetReason = code, reason
	} else if s.job.BudgetState == "requests_exhausted" || s.job.BudgetState == "bytes_exhausted" || s.job.BudgetState == "budget_settings_invalid" || s.job.BudgetState == "budget_unavailable" {
		s.job.BudgetState, s.job.BudgetReason = "ok", ""
	}
	s.mu.Unlock()
	return code, reason
}

func (s *Scheduler) markBudgetBlock(err error) bool {
	block, ok := AsBudgetBlock(err)
	if !ok {
		return false
	}
	atomic.AddInt64(&s.resourceSkippedRounds, 1)
	s.mu.Lock()
	s.job.BudgetState, s.job.BudgetReason = block.Code, block.Reason
	s.mu.Unlock()
	return true
}

func (s *Scheduler) clearBudgetBlock() {
	s.mu.Lock()
	s.job.BudgetState, s.job.BudgetReason = "ok", ""
	s.mu.Unlock()
}

func (s *Scheduler) checkStorage() string {
	if s.storageGuard == nil {
		return ""
	}
	reason := s.storageGuard()
	s.mu.Lock()
	if reason != "" {
		s.job.StorageState = "storage_protected"
		s.job.StorageReason = reason
	} else {
		s.job.StorageState = "ok"
		s.job.StorageReason = ""
	}
	s.mu.Unlock()
	return reason
}

func (s *Scheduler) markPersistenceFailure(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.job.PersistenceState = PersistenceStateDegraded
	s.job.PersistenceError = err.Error()
	s.job.UpdatedAt = time.Now()
}

func (s *Scheduler) clearPersistenceFailure() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.job.PersistenceState = PersistenceStateHealthy
	s.job.PersistenceError = ""
}

// SkippedRounds returns the total number of ticks skipped due to overlap.
func (s *Scheduler) SkippedRounds() int64 {
	return atomic.LoadInt64(&s.skippedRounds)
}

// CompletedRuns returns the total number of runs completed.
func (s *Scheduler) CompletedRuns() int64 {
	return atomic.LoadInt64(&s.completedRuns)
}

// Start initiates or resumes the periodic monitoring loop.
// Running -> Start: idempotent no-op.
// Paused -> Start: resumes existing loop without creating a duplicate goroutine or overwriting cancel.
// Stopped -> Start: initializes a fresh lifecycle context and loop.
func (s *Scheduler) Start(parentCtx context.Context) error {
	if reason := s.checkStorage(); reason != "" {
		return fmt.Errorf("monitor storage protected: %s", reason)
	}
	s.mu.Lock()

	if s.stopping {
		s.mu.Unlock()
		return fmt.Errorf("scheduler is stopping")
	}
	if s.state == JobStateBlocked {
		blockedReason := s.job.BlockedReason
		s.mu.Unlock()
		if blockedReason != "" {
			return fmt.Errorf("monitor job is blocked: %s", blockedReason)
		}
		return fmt.Errorf("monitor job is blocked")
	}

	if s.state == JobStateRunning {
		s.mu.Unlock()
		return nil // Already running
	}

	if s.state == JobStatePaused {
		// Paused -> Start is equivalent to Resume, reusing existing loop and cancel func
		s.state = JobStateRunning
		s.job.State = JobStateRunning
		s.job.UpdatedAt = time.Now()
		s.mu.Unlock()
		return nil
	}

	// Stopped -> Start: create new lifecycle
	if parentCtx == nil {
		parentCtx = context.Background()
	}

	s.ctx, s.cancel = context.WithCancel(parentCtx)
	s.state = JobStateRunning
	s.job.State = JobStateRunning
	s.job.UpdatedAt = time.Now()

	s.loopWg.Add(1)
	scheduleCtx := s.ctx
	interval := s.job.Interval
	s.mu.Unlock()
	// Claim the initial round before returning. Pause/Stop can cancel its
	// pending admission; an already-admitted request follows normal cancellation.
	s.launchScheduledRound(scheduleCtx, time.Now())
	go s.scheduleLoop(scheduleCtx, interval)

	return nil
}

// Pause pauses the scheduled execution without destroying the loop context.
func (s *Scheduler) Pause() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != JobStateRunning || s.stopping {
		return nil
	}

	s.state = JobStatePaused
	s.job.State = JobStatePaused
	s.job.UpdatedAt = time.Now()
	if s.pendingCancel != nil {
		s.recoveryPending = false
		s.pendingCancel()
		s.job.BudgetState, s.job.BudgetReason = "paused_by_user", "用户暂停了尚未开始的 Monitor 等待轮次"
	}
	return nil
}

// Resume resumes execution from a paused state.
func (s *Scheduler) Resume() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != JobStatePaused || s.stopping {
		return nil
	}

	s.state = JobStateRunning
	s.job.State = JobStateRunning
	s.job.UpdatedAt = time.Now()
	return nil
}

// Stop terminates the scheduler, cancels in-flight tasks, and waits for all workers and the loop to exit.
// Follows strict happens-before: stopping = true -> prohibit new runs -> cancel -> Wait().
func (s *Scheduler) Stop() error {
	s.mu.Lock()
	if s.state == JobStateStopped || s.stopping {
		s.mu.Unlock()
		return nil
	}
	if s.state == JobStateBlocked {
		s.state = JobStateStopped
		s.job.State = JobStateStopped
		s.job.BlockedReason = ""
		s.job.UpdatedAt = time.Now()
		s.recoveryPending = false
		s.mu.Unlock()
		return nil
	}

	s.stopping = true
	s.state = JobStateStopped
	s.job.State = JobStateStopped
	s.job.UpdatedAt = time.Now()
	if s.cancel != nil {
		s.cancel()
	}
	if s.pendingCancel != nil {
		s.recoveryPending = false
		s.pendingCancel()
	}
	s.mu.Unlock()

	// Wait for background schedule loop and any active runs to finish
	s.runWg.Wait()
	s.loopWg.Wait()

	s.mu.Lock()
	s.stopping = false
	s.mu.Unlock()
	return nil
}

// TriggerImmediate runs an immediate single monitoring round bounded by scheduler lifecycle.
// Rejects execution if stopped or stopping, links to scheduler context, and drains via runWg.
func (s *Scheduler) TriggerImmediate(ctx context.Context) (*MonitorRun, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if reason := s.checkStorage(); reason != "" {
		return nil, fmt.Errorf("monitor storage protected: %s", reason)
	}
	if code, reason := s.checkBudget(); code != "" {
		return nil, budgetBlock(code, reason)
	}

	s.mu.Lock()
	blocked := s.state == JobStateBlocked
	blockedReason := s.job.BlockedReason
	allowPaused := s.state == JobStatePaused
	if s.state == JobStateStopped || blocked || s.stopping {
		s.mu.Unlock()
		if blocked && blockedReason != "" {
			return nil, fmt.Errorf("monitor job is blocked: %s", blockedReason)
		}
		return nil, fmt.Errorf("scheduler is stopped or stopping")
	}

	if !atomic.CompareAndSwapInt32(&s.isExecuting, 0, 1) {
		s.mu.Unlock()
		atomic.AddInt64(&s.skippedRounds, 1)
		return nil, fmt.Errorf("上一轮监测正在执行中，跳过本次触发")
	}

	s.runWg.Add(1)
	schedCtx := s.ctx
	jobCopy := *s.job
	jobCopy.SamplingTier = SamplingTierDiagnostic
	jobCopy.NextRunTrigger = SamplingTriggerManual
	s.pendingID++
	id := s.pendingID
	s.mu.Unlock()

	defer func() {
		atomic.StoreInt32(&s.isExecuting, 0)
		s.runWg.Done()
	}()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.mu.Lock()
	s.pendingCancel = cancel
	s.mu.Unlock()
	defer s.clearPending(id)

	if schedCtx != nil {
		stopAfter := context.AfterFunc(schedCtx, func() {
			cancel()
		})
		defer stopAfter()
	}

	run, _, err := s.runner.ExecuteRunWithAdmission(runCtx, &jobCopy, time.Now(), func() error { return s.admitPending(id, runCtx, allowPaused) })
	if err != nil {
		if s.markBudgetBlock(err) {
			return run, err
		}
		s.markPersistenceFailure(err)
		return run, err
	}
	s.clearPersistenceFailure()
	s.clearBudgetBlock()
	atomic.AddInt64(&s.completedRuns, 1)
	return run, err
}

func (s *Scheduler) scheduleLoop(ctx context.Context, interval time.Duration) {
	defer s.loopWg.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case tickTime := <-ticker.C:
			s.mu.Lock()
			currentState := s.state
			isStopping := s.stopping
			s.mu.Unlock()

			if currentState != JobStateRunning || isStopping {
				// Paused or stopped: skip tick
				continue
			}

			s.launchScheduledRound(ctx, tickTime)
		}
	}
}

func (s *Scheduler) launchScheduledRound(ctx context.Context, scheduledAt time.Time) {
	if reason := s.checkStorage(); reason != "" {
		s.blockRecoveryStart(reason)
		atomic.AddInt64(&s.resourceSkippedRounds, 1)
		return
	}
	if code, reason := s.checkBudget(); code != "" {
		s.blockRecoveryStart(reason)
		atomic.AddInt64(&s.resourceSkippedRounds, 1)
		return
	}
	if time.Since(scheduledAt) > s.job.Interval {
		atomic.AddInt64(&s.skippedRounds, 1)
		return
	}
	s.mu.Lock()
	if s.state != JobStateRunning || s.stopping {
		s.mu.Unlock()
		return
	}
	if !atomic.CompareAndSwapInt32(&s.isExecuting, 0, 1) {
		s.mu.Unlock()
		atomic.AddInt64(&s.skippedRounds, 1)
		return
	}
	recoveryRound := s.recoveryPending
	roundCtx, cancel := context.WithCancel(ctx)
	s.pendingID++
	id := s.pendingID
	s.pendingCancel = cancel
	s.runWg.Add(1)
	s.mu.Unlock()

	go func() {
		defer cancel()
		defer s.clearPending(id)
		defer atomic.StoreInt32(&s.isExecuting, 0)
		defer s.runWg.Done()
		s.executeScheduledRound(roundCtx, scheduledAt, id, recoveryRound)
	}()
}

func (s *Scheduler) blockRecoveryStart(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.recoveryPending || s.state != JobStateRunning || !s.job.ResumeOnLaunch || s.job.DesiredState != JobStateRunning {
		return
	}
	s.recoveryPending = false
	s.state = JobStateStopped
	s.job.State = JobStateStopped
	s.job.RecoveryState = RecoveryStateBlocked
	s.job.RecoveryReason = reason
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *Scheduler) clearPending(id uint64) {
	s.mu.Lock()
	if s.pendingID == id {
		s.pendingCancel = nil
		s.recoveryPending = false
	}
	s.mu.Unlock()
}

func (s *Scheduler) admitPending(id uint64, ctx context.Context, allowPaused bool) error {
	if reason := s.checkStorage(); reason != "" {
		return budgetBlock("storage_protected", reason)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stateAllowed := s.state == JobStateRunning || (allowPaused && s.state == JobStatePaused)
	if s.pendingID != id || s.stopping || !stateAllowed || ctx.Err() != nil {
		return budgetBlock("cancelled", "Monitor 等待已取消，本周期未探测")
	}
	s.pendingCancel = nil
	if s.recoveryPending {
		s.recoveryPending = false
		s.job.RecoveryState = RecoveryStateRestored
		s.job.RecoveryReason = ""
	}
	return nil
}

func (s *Scheduler) executeScheduledRound(ctx context.Context, scheduledAt time.Time, id uint64, recoveryRound bool) {

	s.mu.Lock()
	jobCopy := *s.job
	isStopping := s.stopping
	state := s.state
	s.mu.Unlock()

	if isStopping || state == JobStateStopped {
		return
	}

	run, _, err := s.runner.ExecuteRunWithAdmission(ctx, &jobCopy, scheduledAt, func() error { return s.admitPending(id, ctx, false) })
	if err != nil {
		if s.markBudgetBlock(err) {
			if recoveryRound {
				s.blockRecoveryStartAfterAdmissionFailure(err.Error())
			}
			return
		}
		s.markPersistenceFailure(err)
		return
	}
	_ = run
	s.clearPersistenceFailure()
	s.clearBudgetBlock()
	atomic.AddInt64(&s.completedRuns, 1)
}

func (s *Scheduler) blockRecoveryStartAfterAdmissionFailure(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != JobStateRunning || !s.job.ResumeOnLaunch || s.job.DesiredState != JobStateRunning || s.job.RecoveryState != RecoveryStateRestoring {
		return
	}
	s.state = JobStateStopped
	s.job.State = JobStateStopped
	s.job.RecoveryState = RecoveryStateBlocked
	s.job.RecoveryReason = reason
	if s.cancel != nil {
		s.cancel()
	}
}
