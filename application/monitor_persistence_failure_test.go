package application

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
)

type sqliteMonitorFailureStore struct {
	*history.Store
	failSamples bool
	failUpdate  bool
}

func (s *sqliteMonitorFailureStore) SaveMonitorSamples(ctx context.Context, samples []*monitor.MonitorSample) error {
	if s.failSamples {
		return context.Canceled
	}
	return s.Store.SaveMonitorSamples(ctx, samples)
}

func (s *sqliteMonitorFailureStore) UpdateMonitorRun(ctx context.Context, run *monitor.MonitorRun) error {
	if s.failUpdate {
		return context.Canceled
	}
	return s.Store.UpdateMonitorRun(ctx, run)
}

type successfulMonitorDialer struct{}

func (successfulMonitorDialer) CreateClient(monitor.MonitoredNode, time.Duration) (*http.Client, error) {
	return &http.Client{Transport: successfulRoundTripper{}}, nil
}

type successfulRoundTripper struct{}

func (successfulRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: 204,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     make(http.Header),
	}, nil
}

func TestMonitorSQLitePersistenceFailuresRemainVisible(t *testing.T) {
	tests := []struct {
		name        string
		failSamples bool
		failUpdate  bool
		wantStored  monitor.RunStatus
		wantSamples int
	}{
		{
			name:        "sample write fails",
			failSamples: true,
			wantStored:  monitor.RunStatusPersistenceFailed,
		},
		{
			name:        "final update fails",
			failUpdate:  true,
			wantStored:  monitor.RunStatusRunning,
			wantSamples: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, err := history.NewStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()

			runner := monitor.NewRunner(monitor.RunnerConfig{
				Store:  &sqliteMonitorFailureStore{Store: store, failSamples: test.failSamples, failUpdate: test.failUpdate},
				Dialer: successfulMonitorDialer{},
			})
			run, _, err := runner.ExecuteRun(context.Background(), &monitor.MonitorJob{
				ID:        "job-sqlite-failure",
				ProfileID: "profile-sqlite-failure",
				ProbeSet:  monitor.ProbeSetLight,
				Timeout:   time.Second,
				Nodes:     []monitor.MonitoredNode{{NodeKey: "node-sqlite-failure", DisplayName: "fixture-node"}},
			}, time.Now().UTC())
			if err == nil || run == nil || run.Status != monitor.RunStatusPersistenceFailed {
				t.Fatalf("SQLite persistence failure was not surfaced: run=%+v err=%v", run, err)
			}

			runs, err := store.QueryMonitorRuns(context.Background(), run.JobID, 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(runs) != 1 || runs[0].Status != test.wantStored {
				t.Fatalf("unexpected durable run state: got=%+v want=%s", runs, test.wantStored)
			}
			samples, err := store.QueryMonitorSamples(context.Background(), monitor.SampleFilter{RunID: run.RunID})
			if err != nil {
				t.Fatal(err)
			}
			if len(samples) != test.wantSamples {
				t.Fatalf("unexpected durable sample count: got=%d want=%d", len(samples), test.wantSamples)
			}
		})
	}
}
