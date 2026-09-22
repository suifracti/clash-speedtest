package monitor

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSchedulerStorageProtectionSkipsWithoutNodeFailureAndRecovers(t *testing.T) {
	store := &mockSampleStore{}
	dialer := &mockDialer{responses: map[string]func(*http.Request) (*http.Response, error){
		"node": func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 204, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}, nil
		},
	}}
	runner := NewRunner(RunnerConfig{Store: store, Dialer: dialer, WorkerCount: 1})
	job := &MonitorJob{ID: "capacity-job", Interval: 80 * time.Millisecond, Timeout: time.Second, ProbeSet: ProbeSetLight, State: JobStateStopped, Nodes: []MonitoredNode{{NodeKey: "node", DisplayName: "node", RawConfig: map[string]interface{}{"type": "ss", "server": "example.invalid", "port": 443}}}}
	var protected atomic.Bool
	protected.Store(true)
	scheduler, err := NewScheduler(SchedulerConfig{Job: job, Runner: runner, Store: store, StorageGuard: func() string {
		if protected.Load() {
			return "storage limit"
		}
		return ""
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "storage") {
		t.Fatalf("protected start must fail: %v", err)
	}
	if scheduler.State() != JobStateStopped || scheduler.Job().StorageState != "storage_protected" {
		t.Fatalf("protected stopped job changed state: %+v", scheduler.Job())
	}
	if runs, _ := store.QueryMonitorRuns(context.Background(), job.ID, 10); len(runs) != 0 {
		t.Fatalf("protected start wrote a run: %+v", runs)
	}

	protected.Store(false)
	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for scheduler.CompletedRuns() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if scheduler.CompletedRuns() == 0 {
		t.Fatal("safe storage did not allow normal run")
	}
	safeSamples, err := store.QueryMonitorSamples(context.Background(), SampleFilter{})
	if err != nil || len(safeSamples) == 0 {
		t.Fatalf("normal round saved no sample: %+v %v", safeSamples, err)
	}
	protected.Store(true)
	before := scheduler.CompletedRuns()
	time.Sleep(250 * time.Millisecond)
	if scheduler.CompletedRuns() != before {
		t.Fatalf("protected interval collected a new run: before=%d after=%d", before, scheduler.CompletedRuns())
	}
	if scheduler.Job().StorageState != "storage_protected" {
		t.Fatalf("protection reason not visible: %+v", scheduler.Job())
	}
	runs, err := store.QueryMonitorRuns(context.Background(), job.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != int(before) {
		t.Fatalf("protected ticks wrote audit/failure runs: %+v", runs)
	}
	protectedSamples, err := store.QueryMonitorSamples(context.Background(), SampleFilter{})
	if err != nil || len(protectedSamples) != len(safeSamples) {
		t.Fatalf("protected ticks wrote node samples: %+v %v", protectedSamples, err)
	}
	protected.Store(false)
	deadline = time.Now().Add(time.Second)
	for scheduler.CompletedRuns() == before && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if scheduler.CompletedRuns() == before {
		t.Fatal("running scheduler did not recover after space was freed")
	}
	if err := scheduler.Stop(); err != nil {
		t.Fatal(err)
	}
	stoppedRuns := scheduler.CompletedRuns()
	time.Sleep(170 * time.Millisecond)
	if scheduler.CompletedRuns() != stoppedRuns {
		t.Fatal("stopped job resumed without user Start")
	}
}
