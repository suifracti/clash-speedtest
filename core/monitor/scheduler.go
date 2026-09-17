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
}

// Scheduler manages the periodic execution and lifecycle of a MonitorJob.
type Scheduler struct {
	mu            sync.Mutex
	job           *MonitorJob
	runner        *Runner
	store         SampleStore
	state         JobState
	stopping      bool
	ctx           context.Context
	cancel        context.CancelFunc
	loopWg        sync.WaitGroup
	runWg         sync.WaitGroup
	isExecuting   int32 // atomic flag: 1 if runner is active, 0 otherwise
	skippedRounds int64 // atomic counter for overlapped ticks skipped
	completedRuns int64 // atomic counter for finished runs
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

	s := &Scheduler{
		job:    cfg.Job,
		runner: cfg.Runner,
		store:  cfg.Store,
		state:  JobStateStopped,
	}
	s.job.State = JobStateStopped
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
	s.mu.Lock()
	defer s.mu.Unlock()
	return *s.job
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
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopping {
		return fmt.Errorf("scheduler is stopping")
	}

	if s.state == JobStateRunning {
		return nil // Already running
	}

	if s.state == JobStatePaused {
		// Paused -> Start is equivalent to Resume, reusing existing loop and cancel func
		s.state = JobStateRunning
		s.job.State = JobStateRunning
		s.job.UpdatedAt = time.Now()
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
	go s.scheduleLoop(s.ctx, s.job.Interval)

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

	s.stopping = true
	s.state = JobStateStopped
	s.job.State = JobStateStopped
	s.job.UpdatedAt = time.Now()
	if s.cancel != nil {
		s.cancel()
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

	s.mu.Lock()
	if s.state == JobStateStopped || s.stopping {
		s.mu.Unlock()
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
	s.mu.Unlock()

	defer func() {
		atomic.StoreInt32(&s.isExecuting, 0)
		s.runWg.Done()
	}()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if schedCtx != nil {
		stopAfter := context.AfterFunc(schedCtx, func() {
			cancel()
		})
		defer stopAfter()
	}

	run, _, err := s.runner.ExecuteRun(runCtx, &jobCopy, time.Now())
	if err == nil {
		atomic.AddInt64(&s.completedRuns, 1)
	}
	return run, err
}

func (s *Scheduler) scheduleLoop(ctx context.Context, interval time.Duration) {
	defer s.loopWg.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run initial round immediately upon start
	s.launchScheduledRound(ctx, time.Now())

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
	s.mu.Lock()
	if s.state != JobStateRunning || s.stopping {
		s.mu.Unlock()
		return
	}
	s.runWg.Add(1)
	s.mu.Unlock()

	go func() {
		defer s.runWg.Done()
		s.executeScheduledRound(ctx, scheduledAt)
	}()
}

func (s *Scheduler) executeScheduledRound(ctx context.Context, scheduledAt time.Time) {
	// Overlap Prevention Guard:
	// If a previous run is still executing, DO NOT spawn another runner goroutine!
	if !atomic.CompareAndSwapInt32(&s.isExecuting, 0, 1) {
		atomic.AddInt64(&s.skippedRounds, 1)

		// Record skipped run in store for audit observability
		if s.store != nil {
			s.mu.Lock()
			jobID := s.job.ID
			isStopping := s.stopping
			state := s.state
			s.mu.Unlock()

			if !isStopping && state != JobStateStopped {
				skippedRun := &MonitorRun{
					RunID:        newID("run_skip"),
					JobID:        jobID,
					ScheduledAt:  scheduledAt,
					StartedAt:    scheduledAt,
					FinishedAt:   &scheduledAt,
					Status:       RunStatusSkipped,
					ErrorMessage: "上一轮监测仍在执行，按防重叠策略跳过本轮",
				}
				_ = s.store.SaveMonitorRun(ctx, skippedRun)
			}
		}
		return
	}

	defer atomic.StoreInt32(&s.isExecuting, 0)

	s.mu.Lock()
	jobCopy := *s.job
	isStopping := s.stopping
	state := s.state
	s.mu.Unlock()

	if isStopping || state == JobStateStopped {
		return
	}

	_, _, err := s.runner.ExecuteRun(ctx, &jobCopy, scheduledAt)
	if err == nil {
		atomic.AddInt64(&s.completedRuns, 1)
	}
}
