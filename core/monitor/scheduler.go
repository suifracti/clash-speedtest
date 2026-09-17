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
	mu            sync.RWMutex
	job           *MonitorJob
	runner        *Runner
	store         SampleStore
	state         JobState
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
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
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// Job returns a copy of the underlying MonitorJob.
func (s *Scheduler) Job() MonitorJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
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

// Start initiates the periodic monitoring loop.
func (s *Scheduler) Start(parentCtx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == JobStateRunning {
		return nil // Already running
	}

	if parentCtx == nil {
		parentCtx = context.Background()
	}

	s.ctx, s.cancel = context.WithCancel(parentCtx)
	s.state = JobStateRunning
	s.job.State = JobStateRunning
	s.job.UpdatedAt = time.Now()

	s.wg.Add(1)
	go s.scheduleLoop(s.ctx, s.job.Interval)

	return nil
}

// Pause pauses the scheduled execution without destroying the loop context.
func (s *Scheduler) Pause() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != JobStateRunning {
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

	if s.state != JobStatePaused {
		return nil
	}

	s.state = JobStateRunning
	s.job.State = JobStateRunning
	s.job.UpdatedAt = time.Now()
	return nil
}

// Stop terminates the scheduler, cancels in-flight tasks, and waits for workers to exit.
func (s *Scheduler) Stop() error {
	s.mu.Lock()
	if s.state == JobStateStopped {
		s.mu.Unlock()
		return nil
	}

	s.state = JobStateStopped
	s.job.State = JobStateStopped
	s.job.UpdatedAt = time.Now()
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()

	// Wait for background schedule loop and any active run to finish
	s.wg.Wait()
	return nil
}

// TriggerImmediate runs an immediate single monitoring round in the background if not currently executing.
func (s *Scheduler) TriggerImmediate(ctx context.Context) (*MonitorRun, error) {
	if !atomic.CompareAndSwapInt32(&s.isExecuting, 0, 1) {
		atomic.AddInt64(&s.skippedRounds, 1)
		return nil, fmt.Errorf("上一轮监测正在执行中，跳过本次触发")
	}
	defer atomic.StoreInt32(&s.isExecuting, 0)

	s.mu.RLock()
	jobCopy := *s.job
	s.mu.RUnlock()

	run, _, err := s.runner.ExecuteRun(ctx, &jobCopy, time.Now())
	if err == nil {
		atomic.AddInt64(&s.completedRuns, 1)
	}
	return run, err
}

func (s *Scheduler) scheduleLoop(ctx context.Context, interval time.Duration) {
	defer s.wg.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run initial round immediately upon start
	s.launchScheduledRound(ctx, time.Now())

	for {
		select {
		case <-ctx.Done():
			return
		case tickTime := <-ticker.C:
			s.mu.RLock()
			currentState := s.state
			s.mu.RUnlock()

			if currentState != JobStateRunning {
				// Paused or stopped: skip tick
				continue
			}

			s.launchScheduledRound(ctx, tickTime)
		}
	}
}

func (s *Scheduler) launchScheduledRound(ctx context.Context, scheduledAt time.Time) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
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
			s.mu.RLock()
			jobID := s.job.ID
			s.mu.RUnlock()

			skippedRun := &MonitorRun{
				RunID:        newID("run_skip"),
				JobID:        jobID,
				ScheduledAt:  scheduledAt,
				StartedAt:    scheduledAt,
				Status:       RunStatusSkipped,
				ErrorMessage: "上一轮监测仍在执行，按防重叠策略跳过本轮",
			}
			_ = s.store.SaveMonitorRun(ctx, skippedRun)
		}
		return
	}

	defer atomic.StoreInt32(&s.isExecuting, 0)

	s.mu.RLock()
	jobCopy := *s.job
	s.mu.RUnlock()

	_, _, err := s.runner.ExecuteRun(ctx, &jobCopy, scheduledAt)
	if err == nil {
		atomic.AddInt64(&s.completedRuns, 1)
	}
}
