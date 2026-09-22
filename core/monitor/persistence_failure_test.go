package monitor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type persistenceFailureStore struct {
	mockSampleStore
	initialErr error
	sampleErr  error
	updateErr  error
}

func (s *persistenceFailureStore) SaveMonitorRun(ctx context.Context, run *MonitorRun) error {
	if s.initialErr != nil {
		return s.initialErr
	}
	s.mockSampleStore.mu.Lock()
	defer s.mockSampleStore.mu.Unlock()
	copyRun := *run
	s.mockSampleStore.runs = append(s.mockSampleStore.runs, &copyRun)
	return nil
}

func (s *persistenceFailureStore) SaveMonitorSamples(ctx context.Context, samples []*MonitorSample) error {
	if s.sampleErr != nil {
		return s.sampleErr
	}
	return s.mockSampleStore.SaveMonitorSamples(ctx, samples)
}

func (s *persistenceFailureStore) UpdateMonitorRun(ctx context.Context, run *MonitorRun) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	s.mockSampleStore.mu.Lock()
	defer s.mockSampleStore.mu.Unlock()
	copyRun := *run
	for i, existing := range s.mockSampleStore.runs {
		if existing.RunID == run.RunID {
			s.mockSampleStore.runs[i] = &copyRun
			return nil
		}
	}
	s.mockSampleStore.runs = append(s.mockSampleStore.runs, &copyRun)
	return nil
}

func persistenceSuccessDialer() *mockDialer {
	return &mockDialer{responses: map[string]func(req *http.Request) (*http.Response, error){
		"node-persistence": func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 204,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		},
	}}
}

func persistenceTestJob() *MonitorJob {
	return &MonitorJob{
		ID:        "job-persistence-failure",
		ProfileID: "profile-persistence",
		ProbeSet:  ProbeSetLight,
		Timeout:   time.Second,
		Nodes: []MonitoredNode{{
			NodeKey:     "node-persistence",
			DisplayName: "fixture-node",
			Server:      "fixture.invalid",
			Port:        443,
		}},
	}
}

func TestRunnerPersistenceFailuresAreNotReportedAsCompleted(t *testing.T) {
	tests := []struct {
		name       string
		initialErr error
		sampleErr  error
		updateErr  error
	}{
		{name: "initial run save", initialErr: fmt.Errorf("initial run database is unavailable")},
		{name: "sample save", sampleErr: fmt.Errorf("sample database is unavailable")},
		{name: "final run update", updateErr: fmt.Errorf("run database is unavailable")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &persistenceFailureStore{
				initialErr: test.initialErr,
				sampleErr:  test.sampleErr,
				updateErr:  test.updateErr,
			}
			runner := NewRunner(RunnerConfig{
				Store:       store,
				Dialer:      persistenceSuccessDialer(),
				WorkerCount: 1,
			})

			run, _, err := runner.ExecuteRun(context.Background(), persistenceTestJob(), time.Now().UTC())
			if err == nil {
				t.Fatal("persistence failure was swallowed")
			}
			if run == nil || run.Status != RunStatusPersistenceFailed {
				t.Fatalf("persistence failure did not change run status: run=%+v err=%v", run, err)
			}
			if run.ErrorMessage == "" {
				t.Fatalf("persistence failure did not become visible: %+v", run)
			}

			runs := store.GetRuns()
			if test.initialErr != nil {
				if len(runs) != 0 {
					t.Fatalf("failed initial save should not leave a durable run: %+v", runs)
				}
				return
			}
			if test.updateErr != nil {
				if len(runs) != 1 || runs[0].Status != RunStatusRunning {
					t.Fatalf("failed final update should leave only the honest in-progress row: %+v", runs)
				}
			} else {
				if len(runs) != 1 || runs[0].Status != RunStatusPersistenceFailed {
					t.Fatalf("sample failure should be recorded as persistence_failed: %+v", runs)
				}
			}
		})
	}
}

func TestSchedulerExposesPersistenceDegradedState(t *testing.T) {
	store := &persistenceFailureStore{sampleErr: fmt.Errorf("fixture sample write failed")}
	runner := NewRunner(RunnerConfig{
		Store:       store,
		Dialer:      persistenceSuccessDialer(),
		WorkerCount: 1,
	})
	scheduler, err := NewScheduler(SchedulerConfig{
		Job:    persistenceTestJob(),
		Runner: runner,
		Store:  store,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = scheduler.Stop() })

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job := scheduler.Job()
		if job.PersistenceState == PersistenceStateDegraded {
			if job.PersistenceError == "" {
				t.Fatal("degraded scheduler state omitted persistence error")
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("scheduler did not expose persistence degradation: %+v", scheduler.Job())
}

func TestRunnerNormalPersistenceRemainsCompleted(t *testing.T) {
	store := &persistenceFailureStore{}
	runner := NewRunner(RunnerConfig{
		Store:       store,
		Dialer:      persistenceSuccessDialer(),
		WorkerCount: 1,
	})

	run, samples, err := runner.ExecuteRun(context.Background(), persistenceTestJob(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != RunStatusCompleted || len(samples) != 1 {
		t.Fatalf("normal persistence regressed: run=%+v samples=%d", run, len(samples))
	}
	if runs := store.GetRuns(); len(runs) != 1 || runs[0].Status != RunStatusCompleted {
		t.Fatalf("normal completed row not retained: %+v", runs)
	}
}
