package monitor

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestScheduler_Lifecycle(t *testing.T) {
	mockStore := &mockSampleStore{}
	dialer := &mockDialer{
		responses: map[string]func(req *http.Request) (*http.Response, error){
			"nk_node1": func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: 204,
					Body:       io.NopCloser(bytes.NewReader(nil)),
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	runner := NewRunner(RunnerConfig{
		Dialer:      dialer,
		Store:       mockStore,
		WorkerCount: 2,
	})

	job := &MonitorJob{
		ID:        "job_lifecycle_test",
		Name:      "Lifecycle Test",
		ProbeSet:  ProbeSetLight,
		Interval:  50 * time.Millisecond,
		Timeout:   1 * time.Second,
		State:     JobStateStopped,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Nodes: []MonitoredNode{
			{
				NodeKey:     "nk_node1",
				DisplayName: "Node 1",
				RawConfig: map[string]interface{}{
					"type":   "ss",
					"server": "1.1.1.1",
					"port":   8388,
				},
			},
		},
	}

	scheduler, err := NewScheduler(SchedulerConfig{
		Job:    job,
		Runner: runner,
		Store:  mockStore,
	})
	if err != nil {
		t.Fatalf("Failed to create scheduler: %v", err)
	}

	if scheduler.State() != JobStateStopped {
		t.Fatalf("Expected initial state %s, got %s", JobStateStopped, scheduler.State())
	}

	// 1. Start
	ctx := context.Background()
	if err := scheduler.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if scheduler.State() != JobStateRunning {
		t.Fatalf("Expected state %s after Start, got %s", JobStateRunning, scheduler.State())
	}

	// Wait for at least 1 completed run
	time.Sleep(80 * time.Millisecond)
	completed1 := scheduler.CompletedRuns()
	if completed1 < 1 {
		t.Fatalf("Expected at least 1 completed run after Start, got %d", completed1)
	}

	// 2. Pause
	if err := scheduler.Pause(); err != nil {
		t.Fatalf("Pause failed: %v", err)
	}
	if scheduler.State() != JobStatePaused {
		t.Fatalf("Expected state %s after Pause, got %s", JobStatePaused, scheduler.State())
	}

	// Sleep across multiple intervals to ensure no new runs execute while paused
	time.Sleep(120 * time.Millisecond)
	completedAfterPause := scheduler.CompletedRuns()
	if completedAfterPause != completed1 {
		t.Fatalf("Expected no new runs during pause (expected %d, got %d)", completed1, completedAfterPause)
	}

	// 3. Resume
	if err := scheduler.Resume(); err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if scheduler.State() != JobStateRunning {
		t.Fatalf("Expected state %s after Resume, got %s", JobStateRunning, scheduler.State())
	}

	// Sleep to allow a new tick to execute
	time.Sleep(100 * time.Millisecond)
	completedAfterResume := scheduler.CompletedRuns()
	if completedAfterResume <= completedAfterPause {
		t.Fatalf("Expected runs to increase after Resume (was %d, now %d)", completedAfterPause, completedAfterResume)
	}

	// 4. Stop
	if err := scheduler.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if scheduler.State() != JobStateStopped {
		t.Fatalf("Expected state %s after Stop, got %s", JobStateStopped, scheduler.State())
	}

	// Calling Stop again should be a no-op and safe
	if err := scheduler.Stop(); err != nil {
		t.Fatalf("Idempotent Stop failed: %v", err)
	}
}

func TestScheduler_OverlapPrevention(t *testing.T) {
	mockStore := &mockSampleStore{}
	blockCh := make(chan struct{})

	dialer := &mockDialer{
		responses: map[string]func(req *http.Request) (*http.Response, error){
			"nk_slow_node": func(req *http.Request) (*http.Response, error) {
				// Block until test releases channel or context is cancelled
				select {
				case <-blockCh:
					return &http.Response{
						StatusCode: 204,
						Body:       io.NopCloser(bytes.NewReader(nil)),
						Header:     make(http.Header),
					}, nil
				case <-req.Context().Done():
					return nil, req.Context().Err()
				}
			},
		},
	}

	runner := NewRunner(RunnerConfig{
		Dialer:      dialer,
		Store:       mockStore,
		WorkerCount: 1,
	})

	job := &MonitorJob{
		ID:        "job_overlap_test",
		Name:      "Overlap Test",
		ProbeSet:  ProbeSetLight,
		Interval:  20 * time.Millisecond, // very short tick interval
		Timeout:   2 * time.Second,
		State:     JobStateStopped,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Nodes: []MonitoredNode{
			{
				NodeKey:     "nk_slow_node",
				DisplayName: "Slow Node",
				RawConfig: map[string]interface{}{
					"type":   "ss",
					"server": "1.1.1.1",
					"port":   8388,
				},
			},
		},
	}

	scheduler, err := NewScheduler(SchedulerConfig{
		Job:    job,
		Runner: runner,
		Store:  mockStore,
	})
	if err != nil {
		t.Fatalf("Failed to create scheduler: %v", err)
	}

	ctx := context.Background()
	if err := scheduler.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Let the scheduler tick a few times while the first run is still blocked
	time.Sleep(80 * time.Millisecond)

	skipped := scheduler.SkippedRounds()
	if skipped == 0 {
		t.Fatalf("Expected skipped rounds > 0 due to overlap prevention guard, got %d", skipped)
	}

	// Trigger immediate should also fail with overlap guard error
	_, err = scheduler.TriggerImmediate(ctx)
	if err == nil {
		t.Fatalf("Expected TriggerImmediate to fail during active run, but got nil error")
	}

	// Unblock probe so the first run finishes
	close(blockCh)

	// Stop scheduler cleanly
	if err := scheduler.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Overlap skips are bounded runtime counts, not one durable row per tick.
	for _, r := range mockStore.GetRuns() {
		if r.Status == RunStatusSkipped {
			t.Fatalf("overlap skip wrote an unbounded historical row: %+v", r)
		}
	}
}

func TestScheduler_CancellationAndDrain(t *testing.T) {
	mockStore := &mockSampleStore{}
	var probeActive int32

	dialer := &mockDialer{
		responses: map[string]func(req *http.Request) (*http.Response, error){
			"nk_cancellable": func(req *http.Request) (*http.Response, error) {
				atomic.StoreInt32(&probeActive, 1)
				defer atomic.StoreInt32(&probeActive, 0)
				<-req.Context().Done()
				return nil, req.Context().Err()
			},
		},
	}

	runner := NewRunner(RunnerConfig{
		Dialer:      dialer,
		Store:       mockStore,
		WorkerCount: 1,
	})

	job := &MonitorJob{
		ID:        "job_cancellation_test",
		Name:      "Cancellation Test",
		ProbeSet:  ProbeSetLight,
		Interval:  100 * time.Millisecond,
		Timeout:   5 * time.Second,
		State:     JobStateStopped,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Nodes: []MonitoredNode{
			{
				NodeKey:     "nk_cancellable",
				DisplayName: "Cancellable Node",
				RawConfig: map[string]interface{}{
					"type":   "ss",
					"server": "1.1.1.1",
					"port":   8388,
				},
			},
		},
	}

	scheduler, err := NewScheduler(SchedulerConfig{
		Job:    job,
		Runner: runner,
		Store:  mockStore,
	})
	if err != nil {
		t.Fatalf("Failed to create scheduler: %v", err)
	}

	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait until probe starts
	deadline := time.Now().Add(500 * time.Millisecond)
	for atomic.LoadInt32(&probeActive) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(&probeActive) == 0 {
		t.Fatalf("Probe did not become active within deadline")
	}

	// Stop must cancel probe and wait for it to exit cleanly
	stopDone := make(chan error, 1)
	go func() {
		stopDone <- scheduler.Stop()
	}()

	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatalf("Stop returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Stop timed out; potential goroutine leak or deadlocked drain")
	}

	if atomic.LoadInt32(&probeActive) != 0 {
		t.Fatalf("Probe was not cleanly drained after Stop")
	}
}

func TestScheduler_TriggerImmediate(t *testing.T) {
	mockStore := &mockSampleStore{}
	var runCount int32

	dialer := &mockDialer{
		responses: map[string]func(req *http.Request) (*http.Response, error){
			"nk_imm": func(req *http.Request) (*http.Response, error) {
				atomic.AddInt32(&runCount, 1)
				return &http.Response{
					StatusCode: 204,
					Body:       io.NopCloser(bytes.NewReader(nil)),
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	runner := NewRunner(RunnerConfig{
		Dialer:      dialer,
		Store:       mockStore,
		WorkerCount: 1,
	})

	job := &MonitorJob{
		ID:        "job_immediate_test",
		Name:      "Immediate Test",
		ProbeSet:  ProbeSetLight,
		Interval:  1 * time.Hour, // long interval, will not tick during test
		Timeout:   1 * time.Second,
		State:     JobStateStopped,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Nodes: []MonitoredNode{
			{
				NodeKey:     "nk_imm",
				DisplayName: "Imm Node",
				RawConfig: map[string]interface{}{
					"type":   "ss",
					"server": "1.1.1.1",
					"port":   8388,
				},
			},
		},
	}

	scheduler, err := NewScheduler(SchedulerConfig{
		Job:    job,
		Runner: runner,
		Store:  mockStore,
	})
	if err != nil {
		t.Fatalf("Failed to create scheduler: %v", err)
	}

	ctx := context.Background()

	// 1. Calling TriggerImmediate while Stopped must fail
	_, err = scheduler.TriggerImmediate(ctx)
	if err == nil {
		t.Fatalf("Expected TriggerImmediate to fail when scheduler is stopped")
	}

	// 2. Start scheduler so it can accept immediate triggers
	if err := scheduler.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer scheduler.Stop()
	deadline := time.Now().Add(time.Second)
	for scheduler.CompletedRuns() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if scheduler.CompletedRuns() == 0 {
		t.Fatal("initial round did not finish before immediate trigger")
	}

	// 3. Calling with nil context must defensively work with context.Background fallback
	run, err := scheduler.TriggerImmediate(nil)
	if err != nil {
		t.Fatalf("TriggerImmediate with nil context failed: %v", err)
	}
	if run == nil {
		t.Fatalf("Expected non-nil run")
	}
	if atomic.LoadInt32(&runCount) < 1 {
		t.Fatalf("Expected runCount >= 1, got %d", atomic.LoadInt32(&runCount))
	}
	if scheduler.CompletedRuns() < 1 {
		t.Fatalf("Expected CompletedRuns >= 1, got %d", scheduler.CompletedRuns())
	}
}

func TestScheduler_PausedThenStartNoDuplicateLoop(t *testing.T) {
	mockStore := &mockSampleStore{}
	var runCount int32

	dialer := &mockDialer{
		responses: map[string]func(req *http.Request) (*http.Response, error){
			"nk_loop_test": func(req *http.Request) (*http.Response, error) {
				atomic.AddInt32(&runCount, 1)
				return &http.Response{
					StatusCode: 204,
					Body:       io.NopCloser(bytes.NewReader(nil)),
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	runner := NewRunner(RunnerConfig{
		Dialer:      dialer,
		Store:       mockStore,
		WorkerCount: 1,
	})

	job := &MonitorJob{
		ID:        "job_no_duplicate_loop",
		Name:      "No Duplicate Loop",
		ProbeSet:  ProbeSetLight,
		Interval:  40 * time.Millisecond,
		Timeout:   1 * time.Second,
		State:     JobStateStopped,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Nodes: []MonitoredNode{
			{
				NodeKey:     "nk_loop_test",
				DisplayName: "Loop Node",
				RawConfig: map[string]interface{}{
					"type":   "ss",
					"server": "1.1.1.1",
					"port":   8388,
				},
			},
		},
	}

	scheduler, err := NewScheduler(SchedulerConfig{
		Job:    job,
		Runner: runner,
		Store:  mockStore,
	})
	if err != nil {
		t.Fatalf("Failed to create scheduler: %v", err)
	}

	// 1. Initial Start
	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for initial run + 1 tick (~50ms)
	time.Sleep(50 * time.Millisecond)

	// 2. Pause
	if err := scheduler.Pause(); err != nil {
		t.Fatalf("Pause failed: %v", err)
	}
	runsAtPause := atomic.LoadInt32(&runCount)

	// 3. Repeatedly call Start while Paused (simulating re-entrancy / restart attempts)
	for i := 0; i < 5; i++ {
		if err := scheduler.Start(context.Background()); err != nil {
			t.Fatalf("Start while paused failed: %v", err)
		}
	}

	// 4. Repeatedly call Start while already Running (idempotency check)
	for i := 0; i < 3; i++ {
		if err := scheduler.Start(context.Background()); err != nil {
			t.Fatalf("Start while running failed: %v", err)
		}
	}

	// Let it run for 100ms (~2-3 ticks)
	time.Sleep(100 * time.Millisecond)

	// Clean stop
	if err := scheduler.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	totalRuns := atomic.LoadInt32(&runCount)
	runsAfterResume := totalRuns - runsAtPause

	// If 5 duplicate loops had been spawned, runsAfterResume would be 10-15+ runs!
	// With 1 loop and 40ms interval over 100ms, it should be between 2 and 4 runs.
	if runsAfterResume > 5 {
		t.Fatalf("Duplicate loops detected! Expected <= 5 runs after resume, got %d", runsAfterResume)
	}
	if scheduler.SkippedRounds() > 0 {
		t.Fatalf("Unexpected overlap/skipped rounds: %d (indicates concurrent duplicate loops)", scheduler.SkippedRounds())
	}
}

func TestScheduler_TriggerImmediateConcurrentWithStop(t *testing.T) {
	mockStore := &mockSampleStore{}
	var storeAccessAfterStop int32
	var stopped int32

	dialer := &mockDialer{
		responses: map[string]func(req *http.Request) (*http.Response, error){
			"nk_race_node": func(req *http.Request) (*http.Response, error) {
				// Artificial delay
				time.Sleep(5 * time.Millisecond)
				if atomic.LoadInt32(&stopped) == 1 {
					atomic.AddInt32(&storeAccessAfterStop, 1)
				}
				return &http.Response{
					StatusCode: 204,
					Body:       io.NopCloser(bytes.NewReader(nil)),
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	runner := NewRunner(RunnerConfig{
		Dialer:      dialer,
		Store:       mockStore,
		WorkerCount: 4,
	})

	job := &MonitorJob{
		ID:        "job_race_test",
		Name:      "Race Test",
		ProbeSet:  ProbeSetLight,
		Interval:  10 * time.Minute,
		Timeout:   1 * time.Second,
		State:     JobStateStopped,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Nodes: []MonitoredNode{
			{
				NodeKey:     "nk_race_node",
				DisplayName: "Race Node",
				RawConfig: map[string]interface{}{
					"type":   "ss",
					"server": "1.1.1.1",
					"port":   8388,
				},
			},
		},
	}

	scheduler, err := NewScheduler(SchedulerConfig{
		Job:    job,
		Runner: runner,
		Store:  mockStore,
	})
	if err != nil {
		t.Fatalf("Failed to create scheduler: %v", err)
	}

	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Concurrently invoke TriggerImmediate while calling Stop
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				// Pass nil context occasionally to verify defensive fallback
				var ctx context.Context
				if (id+j)%2 == 0 {
					ctx = nil
				} else {
					ctx = context.Background()
				}
				_, _ = scheduler.TriggerImmediate(ctx)
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}

	// Let triggers run briefly
	time.Sleep(15 * time.Millisecond)

	// Stop the scheduler
	if err := scheduler.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Mark stopped as 1: any probe or store access strictly after Stop() has returned is a violation
	atomic.StoreInt32(&stopped, 1)

	wg.Wait()

	if access := atomic.LoadInt32(&storeAccessAfterStop); access > 0 {
		t.Fatalf("Violation of drain guarantee: %d probe executions occurred after Stop() returned", access)
	}

	// Triggering after Stop must immediately fail
	_, err = scheduler.TriggerImmediate(context.Background())
	if err == nil {
		t.Fatalf("Expected TriggerImmediate to fail after Stop(), got nil error")
	}
}

func TestScheduler_ExpiredPeriodDoesNotBackfill(t *testing.T) {
	store := &mockSampleStore{}
	runner := NewRunner(RunnerConfig{Store: store, Dialer: &mockDialer{}})
	job := &MonitorJob{ID: "expired", ProfileID: "profile", Interval: time.Minute, Nodes: []MonitoredNode{{NodeKey: "node", Server: "node.invalid", Port: 443}}}
	scheduler, err := NewScheduler(SchedulerConfig{Job: job, Runner: runner, Store: store})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		scheduler.launchScheduledRound(context.Background(), time.Now().Add(-24*time.Hour))
	}
	if scheduler.SkippedRounds() != 100 || len(store.GetRuns()) != 0 {
		t.Fatalf("expired periods were enqueued or persisted: skipped=%d runs=%d", scheduler.SkippedRounds(), len(store.GetRuns()))
	}
}
