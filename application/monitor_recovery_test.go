package application

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/profiles"
	_ "modernc.org/sqlite"
)

type recoveryRoundTripper func(*http.Request) (*http.Response, error)

func (f recoveryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type recoveryDialer struct {
	mu        sync.Mutex
	requests  map[string]int
	blockNode string
	started   chan string
	release   chan struct{}
}

func (d *recoveryDialer) CreateClient(node monitor.MonitoredNode, _ time.Duration) (*http.Client, error) {
	return &http.Client{Transport: recoveryRoundTripper(func(req *http.Request) (*http.Response, error) {
		d.mu.Lock()
		d.requests[node.NodeKey]++
		d.mu.Unlock()
		if node.NodeKey == d.blockNode {
			select {
			case d.started <- node.NodeKey:
			default:
			}
			select {
			case <-d.release:
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
		}
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Status:     "204 No Content",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    req,
		}, nil
	})}, nil
}

func (d *recoveryDialer) count(nodeKey string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.requests[nodeKey]
}

type recoverySignalStore struct {
	monitor.SampleStore
	completed chan monitor.MonitorRun
}

func (s *recoverySignalStore) UpdateMonitorRun(ctx context.Context, run *monitor.MonitorRun) error {
	if err := s.SampleStore.UpdateMonitorRun(ctx, run); err != nil {
		return err
	}
	if run.Status != monitor.RunStatusRunning {
		select {
		case s.completed <- *run:
		default:
		}
	}
	return nil
}

func setRecoveryRunner(svc *AppService, store *history.Store, dialer *recoveryDialer) *recoverySignalStore {
	signal := &recoverySignalStore{SampleStore: store, completed: make(chan monitor.MonitorRun, 32)}
	svc.SetMonitorRunner(monitor.NewRunner(monitor.RunnerConfig{Store: signal, Dialer: dialer, WorkerCount: 1}))
	return signal
}

func waitRecoveryRuns(t *testing.T, events <-chan monitor.MonitorRun, jobIDs ...string) map[string]monitor.MonitorRun {
	t.Helper()
	wanted := make(map[string]struct{}, len(jobIDs))
	for _, id := range jobIDs {
		wanted[id] = struct{}{}
	}
	got := make(map[string]monitor.MonitorRun, len(jobIDs))
	timer := time.NewTimer(4 * time.Second)
	defer timer.Stop()
	for len(got) < len(wanted) {
		select {
		case run := <-events:
			if _, ok := wanted[run.JobID]; ok {
				got[run.JobID] = run
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for persisted runs for %v; got %v", jobIDs, got)
		}
	}
	return got
}

func createRecoveryJob(t *testing.T, svc *AppService, profileID string, tier monitor.SamplingTier) (*MonitorJobDTO, string) {
	t.Helper()
	options, err := svc.ListMonitorNodeOptions()
	if err != nil {
		t.Fatal(err)
	}
	var node MonitorNodeOptionDTO
	for _, option := range options {
		if option.ProfileID == profileID {
			node = option
			break
		}
	}
	if node.NodeKey == "" {
		t.Fatalf("no node option for profile %s", profileID)
	}
	job, err := svc.CreateMonitorJobFromRequest(MonitorJobCreateRequest{
		Name: "recovery " + profileID, ProfileID: profileID, NodeKeys: []string{node.NodeKey},
		ProbeSet: monitor.ProbeSetLight, SamplingTier: tier, IntervalSeconds: 3600, TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	return job, node.NodeKey
}

func exhaustRecoveryRequestBudget(t *testing.T, historyDir string) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(historyDir, "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	day := time.Now().UTC().Format("2006-01-02")
	if _, err := db.Exec(`UPDATE monitor_budget_usage SET utc_day=?, requests_used=20000, bytes_used=0 WHERE singleton=1`, day); err != nil {
		t.Fatal(err)
	}
}

func TestMonitorRecoveryWaitCanBeCancelledByPauseStopDisableAndDelete(t *testing.T) {
	root := t.TempDir()
	paths := profiles.Paths{Dir: filepath.Join(root, "profiles")}
	profileIDs := []string{"occupier", "pause", "stop", "disable", "delete"}
	saveP0MonitorProfileStore(t, paths, profileIDs...)
	for i, profileID := range profileIDs {
		writeP0MonitorProfileFixture(t, paths, profileID, "secret-"+profileID, "127.0.0."+string(rune('1'+i)))
	}
	historyDir := filepath.Join(root, "history")
	store, err := history.NewStore(historyDir)
	if err != nil {
		t.Fatal(err)
	}
	first := NewAppService(store, paths, nil)
	settings, err := first.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	maxConcurrent := 1
	settings.MonitorBudgetMaxConcurrent = &maxConcurrent
	if err := first.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	dialer := &recoveryDialer{requests: make(map[string]int)}
	firstSignals := setRecoveryRunner(first, store, dialer)
	jobs := make(map[string]*MonitorJobDTO)
	nodes := make(map[string]string)
	for _, profileID := range profileIDs {
		job, nodeKey := createRecoveryJob(t, first, profileID, monitor.SamplingTierRegular)
		jobs[profileID], nodes[profileID] = job, nodeKey
		if err := first.SetMonitorJobResumeOnLaunch(job.ID, true); err != nil {
			t.Fatal(err)
		}
		if err := first.StartMonitorJob(job.ID); err != nil {
			t.Fatal(err)
		}
		waitRecoveryRuns(t, firstSignals.completed, job.ID)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	reopenedStore, err := history.NewStore(historyDir)
	if err != nil {
		t.Fatal(err)
	}
	reopened := NewAppService(reopenedStore, paths, nil)
	defer reopened.Close()
	blockedDialer := &recoveryDialer{
		requests: make(map[string]int), blockNode: nodes["occupier"],
		started: make(chan string, 1), release: make(chan struct{}),
	}
	secondSignals := setRecoveryRunner(reopened, reopenedStore, blockedDialer)
	if err := reopened.RecoverMonitorJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-blockedDialer.started:
	case <-time.After(4 * time.Second):
		t.Fatal("first recovered request did not enter the controlled transport")
	}
	for _, profileID := range []string{"pause", "stop", "disable", "delete"} {
		job, err := reopened.GetMonitorJob(jobs[profileID].ID)
		if err != nil || job.RecoveryState != monitor.RecoveryStateRestoring {
			t.Fatalf("%s job did not wait for shared admission: job=%+v err=%v", profileID, job, err)
		}
	}
	if err := reopened.PauseMonitorJob(jobs["pause"].ID); err != nil {
		t.Fatal(err)
	}
	if err := reopened.StopMonitorJob(jobs["stop"].ID); err != nil {
		t.Fatal(err)
	}
	if err := reopened.SetMonitorJobResumeOnLaunch(jobs["disable"].ID, false); err != nil {
		t.Fatal(err)
	}
	if err := reopened.DeleteMonitorJob(jobs["delete"].ID); err != nil {
		t.Fatal(err)
	}
	paused, _ := reopened.GetMonitorJob(jobs["pause"].ID)
	stopped, _ := reopened.GetMonitorJob(jobs["stop"].ID)
	disabled, _ := reopened.GetMonitorJob(jobs["disable"].ID)
	if paused.DesiredState != monitor.JobStatePaused || stopped.DesiredState != monitor.JobStateStopped ||
		disabled.State != monitor.JobStateRunning || disabled.DesiredState != monitor.JobStateRunning || disabled.ResumeOnLaunch {
		t.Fatalf("cancel actions changed the wrong runtime or durable intent: paused=%+v stopped=%+v disabled=%+v", paused, stopped, disabled)
	}
	if _, err := reopened.GetMonitorJob(jobs["delete"].ID); err == nil {
		t.Fatal("deleted monitor task remains registered")
	}
	deletedHistory, err := reopened.QueryMonitorRuns(context.Background(), jobs["delete"].ID, 10)
	if err != nil || len(deletedHistory) != 1 {
		t.Fatalf("deleting the task removed its historical run: runs=%+v err=%v", deletedHistory, err)
	}
	close(blockedDialer.release)
	waitRecoveryRuns(t, secondSignals.completed, jobs["occupier"].ID)
	reopened.StopAllMonitorJobs()
	for _, profileID := range []string{"pause", "stop", "disable", "delete"} {
		if got := blockedDialer.count(nodes[profileID]); got != 0 {
			t.Fatalf("cancelled %s recovery issued %d requests", profileID, got)
		}
	}
}

func TestMonitorRecoveryRestoresOnlyExplicitlyRunningOptedInJobsOnce(t *testing.T) {
	root := t.TempDir()
	paths := profiles.Paths{Dir: filepath.Join(root, "profiles")}
	saveP0MonitorProfileStore(t, paths, "enabled", "disabled")
	writeP0MonitorProfileFixture(t, paths, "enabled", "secret-enabled", "127.0.0.1")
	writeP0MonitorProfileFixture(t, paths, "disabled", "secret-disabled", "127.0.0.2")
	historyDir := filepath.Join(root, "history")
	store, err := history.NewStore(historyDir)
	if err != nil {
		t.Fatal(err)
	}
	dialer := &recoveryDialer{requests: make(map[string]int)}
	first := NewAppService(store, paths, nil)
	firstSignals := setRecoveryRunner(first, store, dialer)
	enabledJob, enabledNode := createRecoveryJob(t, first, "enabled", monitor.SamplingTierFocus)
	disabledJob, disabledNode := createRecoveryJob(t, first, "disabled", monitor.SamplingTierRegular)
	if err := first.SetMonitorJobResumeOnLaunch(enabledJob.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := first.StartMonitorJob(enabledJob.ID); err != nil {
		t.Fatal(err)
	}
	if err := first.StartMonitorJob(disabledJob.ID); err != nil {
		t.Fatal(err)
	}
	waitRecoveryRuns(t, firstSignals.completed, enabledJob.ID, disabledJob.ID)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	reopenedStore, err := history.NewStore(historyDir)
	if err != nil {
		t.Fatal(err)
	}
	reopened := NewAppService(reopenedStore, paths, nil)
	defer reopened.Close()
	secondSignals := setRecoveryRunner(reopened, reopenedStore, dialer)
	if err := reopened.RecoverMonitorJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := reopened.RecoverMonitorJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	recovered := waitRecoveryRuns(t, secondSignals.completed, enabledJob.ID)
	if time.Since(recovered[enabledJob.ID].ScheduledAt) > 30*time.Second {
		t.Fatalf("recovered cycle backfilled an old scheduled time: %s", recovered[enabledJob.ID].ScheduledAt)
	}
	if got := dialer.count(enabledNode); got != 2 {
		t.Fatalf("opted-in running job requests = %d, want one original and one restored round", got)
	}
	if got := dialer.count(disabledNode); got != 1 {
		t.Fatalf("job without recovery permission sent %d requests after reopen", got-1)
	}
	if recovered[enabledJob.ID].SamplingTier != monitor.SamplingTierFocus || recovered[enabledJob.ID].TriggerType != monitor.SamplingTriggerScheduled || recovered[enabledJob.ID].SamplingStrategyVersion != monitor.SamplingStrategyVersion {
		t.Fatalf("recovered cycle lost periodic source metadata: %+v", recovered[enabledJob.ID])
	}
	job, err := reopened.GetMonitorJob(enabledJob.ID)
	if err != nil || job.State != monitor.JobStateRunning || job.DesiredState != monitor.JobStateRunning || !job.ResumeOnLaunch {
		t.Fatalf("recovered intent/runtime mismatch: job=%+v err=%v", job, err)
	}
	disabledAfterReopen, err := reopened.GetMonitorJob(disabledJob.ID)
	if err != nil || disabledAfterReopen.State != monitor.JobStateStopped || disabledAfterReopen.DesiredState != monitor.JobStateRunning || disabledAfterReopen.ResumeOnLaunch {
		t.Fatalf("job without recovery permission was not kept stopped: job=%+v err=%v", disabledAfterReopen, err)
	}
	page, err := reopened.QueryMonitorSamples(context.Background(), monitor.SampleFilter{RunID: recovered[enabledJob.ID].RunID})
	if err != nil || len(page) != 1 || page[0].NodeKey != enabledNode || page[0].NodeIdentityKey != enabledJob.Nodes[0].NodeIdentityKey || page[0].ConfigRevisionKey != enabledJob.Nodes[0].ConfigRevisionKey {
		t.Fatalf("recovered sample changed stable identity or revision: samples=%+v err=%v", page, err)
	}
	if err := reopened.StopMonitorJob(enabledJob.ID); err != nil {
		t.Fatal(err)
	}
}

func TestMonitorRecoveryHonorsPausedIntentAndLeavesDiagnosticManual(t *testing.T) {
	root := t.TempDir()
	paths := profiles.Paths{Dir: filepath.Join(root, "profiles")}
	saveP0MonitorProfileStore(t, paths, "profile", "stopped-profile")
	writeP0MonitorProfileFixture(t, paths, "profile", "stable-secret", "127.0.0.1")
	writeP0MonitorProfileFixture(t, paths, "stopped-profile", "stopped-secret", "127.0.0.2")
	historyDir := filepath.Join(root, "history")
	store, err := history.NewStore(historyDir)
	if err != nil {
		t.Fatal(err)
	}
	dialer := &recoveryDialer{requests: make(map[string]int)}
	first := NewAppService(store, paths, nil)
	firstSignals := setRecoveryRunner(first, store, dialer)
	job, nodeKey := createRecoveryJob(t, first, "profile", monitor.SamplingTierRegular)
	stoppedJob, stoppedNode := createRecoveryJob(t, first, "stopped-profile", monitor.SamplingTierRegular)
	if err := first.SetMonitorJobResumeOnLaunch(stoppedJob.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := first.SetMonitorJobResumeOnLaunch(job.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := first.StartMonitorJob(job.ID); err != nil {
		t.Fatal(err)
	}
	waitRecoveryRuns(t, firstSignals.completed, job.ID)
	if err := first.PauseMonitorJob(job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := first.TriggerMonitorJob(job.ID); err != nil {
		t.Fatal(err)
	}
	waitRecoveryRuns(t, firstSignals.completed, job.ID)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	reopenedStore, err := history.NewStore(historyDir)
	if err != nil {
		t.Fatal(err)
	}
	reopened := NewAppService(reopenedStore, paths, nil)
	defer reopened.Close()
	secondSignals := setRecoveryRunner(reopened, reopenedStore, dialer)
	if err := reopened.RecoverMonitorJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	paused, err := reopened.GetMonitorJob(job.ID)
	if err != nil || paused.DesiredState != monitor.JobStatePaused || paused.ResumeOnLaunch != true || paused.State != monitor.JobStateStopped || paused.RecoveryState != monitor.RecoveryStatePaused {
		t.Fatalf("paused intent should survive without launching: job=%+v err=%v", paused, err)
	}
	stopped, err := reopened.GetMonitorJob(stoppedJob.ID)
	if err != nil || stopped.State != monitor.JobStateStopped || stopped.DesiredState != monitor.JobStateStopped || !stopped.ResumeOnLaunch {
		t.Fatalf("enabling recovery alone changed a stopped task: job=%+v err=%v", stopped, err)
	}
	if dialer.count(nodeKey) != 2 || dialer.count(stoppedNode) != 0 {
		t.Fatalf("paused or stopped job sent an automatic request after reopen: paused=%d stopped=%d", dialer.count(nodeKey), dialer.count(stoppedNode))
	}
	select {
	case run := <-secondSignals.completed:
		t.Fatalf("paused/diagnostic job unexpectedly produced a startup run: %+v", run)
	default:
	}
	runs, err := reopened.QueryMonitorRuns(context.Background(), job.ID, 10)
	if err != nil || len(runs) != 2 || runs[0].SamplingTier != monitor.SamplingTierDiagnostic || runs[0].TriggerType != monitor.SamplingTriggerManual {
		t.Fatalf("manual diagnostic source or history changed: runs=%+v err=%v", runs, err)
	}
}

func TestMonitorRecoveryBlocksRevisionAndBudgetFailuresUntilExplicitStart(t *testing.T) {
	root := t.TempDir()
	paths := profiles.Paths{Dir: filepath.Join(root, "profiles")}
	saveP0MonitorProfileStore(t, paths, "revision", "budget")
	writeP0MonitorProfileFixture(t, paths, "revision", "original-revision", "127.0.0.1")
	writeP0MonitorProfileFixture(t, paths, "budget", "stable-budget", "127.0.0.2")
	historyDir := filepath.Join(root, "history")
	store, err := history.NewStore(historyDir)
	if err != nil {
		t.Fatal(err)
	}
	dialer := &recoveryDialer{requests: make(map[string]int)}
	first := NewAppService(store, paths, nil)
	firstSignals := setRecoveryRunner(first, store, dialer)
	revisionJob, revisionNode := createRecoveryJob(t, first, "revision", monitor.SamplingTierRegular)
	budgetJob, budgetNode := createRecoveryJob(t, first, "budget", monitor.SamplingTierRegular)
	for _, job := range []MonitorJobDTO{*revisionJob, *budgetJob} {
		if err := first.SetMonitorJobResumeOnLaunch(job.ID, true); err != nil {
			t.Fatal(err)
		}
		if err := first.StartMonitorJob(job.ID); err != nil {
			t.Fatal(err)
		}
	}
	waitRecoveryRuns(t, firstSignals.completed, revisionJob.ID, budgetJob.ID)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	writeP0MonitorProfileFixture(t, paths, "revision", "changed-revision", "127.0.0.1")
	settingsFile := filepath.Join(root, "settings.json")
	if err := os.WriteFile(settingsFile, []byte(`{"monitor_budget_daily_requests":0}`), 0o600); err != nil {
		t.Fatal(err)
	}
	reopenedStore, err := history.NewStore(historyDir)
	if err != nil {
		t.Fatal(err)
	}
	reopened := NewAppService(reopenedStore, paths, nil)
	defer reopened.Close()
	secondSignals := setRecoveryRunner(reopened, reopenedStore, dialer)
	if err := reopened.RecoverMonitorJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	revisionState, _ := reopened.GetMonitorJob(revisionJob.ID)
	budgetState, _ := reopened.GetMonitorJob(budgetJob.ID)
	if revisionState.RecoveryState != monitor.RecoveryStateBlocked || !strings.Contains(revisionState.RecoveryReason, "revision") || revisionState.State != monitor.JobStateBlocked {
		t.Fatalf("revision conflict did not visibly block recovery: %+v", revisionState)
	}
	if budgetState.RecoveryState != monitor.RecoveryStateBlocked || !strings.Contains(budgetState.RecoveryReason, "预算") || budgetState.State == monitor.JobStateRunning {
		t.Fatalf("unavailable budget did not fail closed: %+v", budgetState)
	}
	if dialer.count(revisionNode) != 1 || dialer.count(budgetNode) != 1 {
		t.Fatalf("blocked recovery issued network requests: revision=%d budget=%d", dialer.count(revisionNode), dialer.count(budgetNode))
	}
	select {
	case run := <-secondSignals.completed:
		t.Fatalf("blocked recovery unexpectedly persisted a run: %+v", run)
	default:
	}
	if err := os.Remove(settingsFile); err != nil {
		t.Fatal(err)
	}
	writeP0MonitorProfileFixture(t, paths, "revision", "original-revision", "127.0.0.1")
	if err := reopened.StartMonitorJob(budgetJob.ID); err != nil {
		t.Fatalf("explicit Start after budget became available: %v", err)
	}
	waitRecoveryRuns(t, secondSignals.completed, budgetJob.ID)
	if err := reopened.StartMonitorJob(revisionJob.ID); err != nil {
		t.Fatalf("explicit Start after the saved node revision was restored: %v", err)
	}
	waitRecoveryRuns(t, secondSignals.completed, revisionJob.ID)
}

func TestMonitorStopIntentPersistenceFailureIsVisibleAfterRuntimeStops(t *testing.T) {
	root := t.TempDir()
	paths := profiles.Paths{Dir: filepath.Join(root, "profiles")}
	saveP0MonitorProfileStore(t, paths, "profile")
	writeP0MonitorProfileFixture(t, paths, "profile", "secret", "127.0.0.1")
	historyDir := filepath.Join(root, "history")
	store, err := history.NewStore(historyDir)
	if err != nil {
		t.Fatal(err)
	}
	service := NewAppService(store, paths, nil)
	job, _ := createRecoveryJob(t, service, "profile", monitor.SamplingTierRegular)
	if err := service.SetMonitorJobResumeOnLaunch(job.ID, true); err != nil {
		t.Fatal(err)
	}
	exhaustRecoveryRequestBudget(t, historyDir)
	if err := service.StartMonitorJob(job.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	err = service.StopMonitorJob(job.ID)
	if err == nil || !strings.Contains(err.Error(), "当前进程已停止") || !strings.Contains(err.Error(), "下次启动仍可能") {
		t.Fatalf("failed stop-intent write was not reported honestly: %v", err)
	}
	current, getErr := service.GetMonitorJob(job.ID)
	if getErr != nil || current.State != monitor.JobStateStopped || current.IntentPersistenceError == "" {
		t.Fatalf("runtime was not stopped or persistence warning is missing: job=%+v err=%v", current, getErr)
	}
}
