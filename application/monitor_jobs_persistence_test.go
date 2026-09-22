package application

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/profiles"
)

func writeP0MonitorProfileFixture(t *testing.T, paths profiles.Paths, profileID, password, server string) {
	t.Helper()
	cache := []byte("proxies:\n" +
		"  - name: Shared Node\n" +
		"    type: ss\n" +
		"    server: " + server + "\n" +
		"    port: 1\n" +
		"    cipher: aes-128-gcm\n" +
		"    password: " + password + "\n")
	if err := paths.WriteCache(profileID, cache); err != nil {
		t.Fatalf("WriteCache(%s): %v", profileID, err)
	}
}

func saveP0MonitorProfileStore(t *testing.T, paths profiles.Paths, ids ...string) {
	t.Helper()
	store := &profiles.Store{Airports: make([]*profiles.Airport, 0, len(ids))}
	for _, id := range ids {
		store.Airports = append(store.Airports, &profiles.Airport{ID: id, Name: id})
	}
	if err := profiles.SaveStore(paths.StoreFile(), store); err != nil {
		t.Fatalf("SaveStore: %v", err)
	}
}

func TestMonitorJobDefinitionReopensStoppedWithoutStarting(t *testing.T) {
	root := t.TempDir()
	paths := profiles.Paths{Dir: filepath.Join(root, "profiles")}
	saveP0MonitorProfileStore(t, paths, "profile-1")
	writeP0MonitorProfileFixture(t, paths, "profile-1", "subscription-secret", "127.0.0.1")
	historyDir := filepath.Join(root, "history")

	store, err := history.NewStore(historyDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	svc := NewAppService(store, paths, nil)
	options, err := svc.ListMonitorNodeOptions()
	if err != nil || len(options) != 1 {
		t.Fatalf("ListMonitorNodeOptions: options=%+v err=%v", options, err)
	}
	created, err := svc.CreateMonitorJobFromRequest(MonitorJobCreateRequest{
		Name:            "Persisted monitor",
		ProfileID:       "profile-1",
		NodeKeys:        []string{options[0].NodeKey},
		ProbeSet:        monitor.ProbeSetService,
		IntervalSeconds: 30,
		TimeoutSeconds:  5,
	})
	if err != nil {
		t.Fatalf("CreateMonitorJobFromRequest: %v", err)
	}
	if created.State != monitor.JobStateStopped {
		t.Fatalf("created job state = %s, want stopped", created.State)
	}
	if err := svc.Close(); err != nil {
		t.Fatalf("close first AppService: %v", err)
	}

	reopenedStore, err := history.NewStore(historyDir)
	if err != nil {
		t.Fatalf("reopen history store: %v", err)
	}
	reopened := NewAppService(reopenedStore, paths, nil)
	defer reopened.Close()
	loaded, err := reopened.GetMonitorJob(created.ID)
	if err != nil {
		t.Fatalf("GetMonitorJob after reopen: %v", err)
	}
	if loaded.State != monitor.JobStateStopped || loaded.BlockedReason != "" {
		t.Fatalf("reopened job state = %s reason=%q, want stopped without reason", loaded.State, loaded.BlockedReason)
	}
	if loaded.ID != created.ID || loaded.ProfileID != created.ProfileID || loaded.ProbeSet != monitor.ProbeSetService ||
		loaded.Interval != 30*time.Second || loaded.Timeout != 5*time.Second || len(loaded.Nodes) != 1 {
		t.Fatalf("reopened definition lost product fields: %+v", loaded)
	}
	if len(loaded.Nodes[0].RawConfig) == 0 {
		t.Fatal("reopened valid node was not resolved from the current canonical cache")
	}

	runs, err := reopened.QueryMonitorRuns(context.Background(), created.ID, 10)
	if err != nil {
		t.Fatalf("QueryMonitorRuns after reopen: %v", err)
	}
	samples, err := reopened.QueryMonitorSamples(context.Background(), monitor.SampleFilter{})
	if err != nil {
		t.Fatalf("QueryMonitorSamples after reopen: %v", err)
	}
	if len(runs) != 0 || len(samples) != 0 {
		t.Fatalf("reopen unexpectedly executed monitor work: runs=%d samples=%d", len(runs), len(samples))
	}
}

func TestMonitorJobDefinitionReopensBlockedForProfileNodeAndRevisionChanges(t *testing.T) {
	root := t.TempDir()
	paths := profiles.Paths{Dir: filepath.Join(root, "profiles")}
	ids := []string{"profile-missing", "profile-node", "profile-revision"}
	saveP0MonitorProfileStore(t, paths, ids...)
	for _, id := range ids {
		writeP0MonitorProfileFixture(t, paths, id, "password-"+id, "127.0.0.1")
	}
	historyDir := filepath.Join(root, "history")
	store, err := history.NewStore(historyDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	svc := NewAppService(store, paths, nil)
	jobIDs := make([]string, 0, len(ids))
	for _, profileID := range ids {
		options, err := svc.ListMonitorNodeOptions()
		if err != nil {
			t.Fatalf("ListMonitorNodeOptions(%s): %v", profileID, err)
		}
		var nodeKey string
		for _, option := range options {
			if option.ProfileID == profileID {
				nodeKey = option.NodeKey
				break
			}
		}
		if nodeKey == "" {
			t.Fatalf("no node option for %s", profileID)
		}
		created, err := svc.CreateMonitorJobFromRequest(MonitorJobCreateRequest{
			Name:            profileID,
			ProfileID:       profileID,
			NodeKeys:        []string{nodeKey},
			ProbeSet:        monitor.ProbeSetLight,
			IntervalSeconds: 30,
			TimeoutSeconds:  5,
		})
		if err != nil {
			t.Fatalf("CreateMonitorJobFromRequest(%s): %v", profileID, err)
		}
		jobIDs = append(jobIDs, created.ID)
	}
	if err := svc.Close(); err != nil {
		t.Fatalf("close first AppService: %v", err)
	}

	currentProfiles, err := profiles.LoadStore(paths.StoreFile())
	if err != nil {
		t.Fatalf("LoadStore before invalidation: %v", err)
	}
	currentProfiles.Remove("profile-missing")
	if err := profiles.SaveStore(paths.StoreFile(), currentProfiles); err != nil {
		t.Fatalf("SaveStore after profile removal: %v", err)
	}
	writeP0MonitorProfileFixture(t, paths, "profile-node", "password-profile-node", "127.0.0.2")
	writeP0MonitorProfileFixture(t, paths, "profile-revision", "password-profile-revision-changed", "127.0.0.1")

	reopenedStore, err := history.NewStore(historyDir)
	if err != nil {
		t.Fatalf("reopen history store: %v", err)
	}
	reopened := NewAppService(reopenedStore, paths, nil)
	defer reopened.Close()
	jobs := reopened.ListMonitorJobs()
	if len(jobs) != len(ids) {
		t.Fatalf("reopened %d jobs, want %d; jobs=%+v", len(jobs), len(ids), jobs)
	}
	byID := make(map[string]monitor.MonitorJob, len(jobs))
	for _, job := range jobs {
		byID[job.ID] = job
	}
	for i, jobID := range jobIDs {
		job, ok := byID[jobID]
		if !ok {
			t.Fatalf("reopened job %s not found", jobID)
		}
		if job.State != monitor.JobStateBlocked || job.BlockedReason == "" {
			t.Fatalf("job %s state=%s reason=%q, want blocked with reason", jobID, job.State, job.BlockedReason)
		}
		if len(job.Nodes) != 1 || len(job.Nodes[0].RawConfig) != 0 {
			t.Fatalf("blocked job %s unexpectedly retained runnable raw config: %+v", jobID, job.Nodes)
		}
		switch i {
		case 0:
			if !strings.Contains(job.BlockedReason, "订阅") {
				t.Fatalf("missing-profile reason is not explicit: %q", job.BlockedReason)
			}
		case 1:
			if !strings.Contains(job.BlockedReason, "不存在") {
				t.Fatalf("missing-node reason is not explicit: %q", job.BlockedReason)
			}
		case 2:
			if !strings.Contains(job.BlockedReason, "revision") {
				t.Fatalf("revision reason is not explicit: %q", job.BlockedReason)
			}
		}
		if err := reopened.StartMonitorJob(jobID); err == nil {
			t.Fatalf("blocked job %s unexpectedly accepted StartMonitorJob", jobID)
		}
	}
}

func TestAppServiceDoesNotRecreateJobsFromMonitorHistory(t *testing.T) {
	root := t.TempDir()
	paths := profiles.Paths{Dir: filepath.Join(root, "profiles")}
	historyDir := filepath.Join(root, "history")
	store, err := history.NewStore(historyDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	now := time.Now().UTC()
	if err := store.SaveMonitorRun(context.Background(), &monitor.MonitorRun{
		RunID: "historical-run", JobID: "historical-only", ScheduledAt: now, StartedAt: now,
		Status: monitor.RunStatusCompleted,
	}); err != nil {
		t.Fatalf("SaveMonitorRun: %v", err)
	}
	if err := store.SaveMonitorSamples(context.Background(), []*monitor.MonitorSample{{
		SampleID: "historical-sample", RunID: "historical-run",
		NodeKey: "nk-history", NodeIdentityKey: "nid-history", ConfigRevisionKey: "rev-history",
		ProfileID: "profile-history", DisplayNameSnapshot: "History only", ProbeType: "rtt",
		Target: "fixture", Timestamp: now, Success: true,
	}}); err != nil {
		t.Fatalf("SaveMonitorSamples: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	reopenedStore, err := history.NewStore(historyDir)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	svc := NewAppService(reopenedStore, paths, nil)
	defer svc.Close()
	if jobs := svc.ListMonitorJobs(); len(jobs) != 0 {
		t.Fatalf("history-only rows resurrected monitor jobs: %+v", jobs)
	}
}

func TestCreateMonitorJobRequiresDurableCommitBeforeRegistration(t *testing.T) {
	root := t.TempDir()
	store, err := history.NewStore(filepath.Join(root, "history"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	paths := profiles.Paths{Dir: filepath.Join(root, "profiles")}
	svc := NewAppService(store, paths, nil)
	defer svc.Close()
	svc.SetMonitorRunner(monitor.NewRunner(monitor.RunnerConfig{Store: store}))
	if err := store.Close(); err != nil {
		t.Fatalf("close history store for fault injection: %v", err)
	}

	_, err = svc.CreateMonitorJob(monitor.MonitorJob{
		ID: "durability-failure", Name: "Should not register", ProfileID: "profile-1",
		ProbeSet: monitor.ProbeSetLight, Interval: time.Minute, Timeout: time.Second,
		Nodes: []monitor.MonitoredNode{{DisplayName: "Node", Type: "ss", Server: "127.0.0.1", Port: 1, RawConfig: map[string]any{
			"type": "ss", "server": "127.0.0.1", "port": 1, "password": "secret",
		}}},
	})
	if err == nil {
		t.Fatal("CreateMonitorJob succeeded after durable store was closed")
	}
	if jobs := svc.ListMonitorJobs(); len(jobs) != 0 {
		t.Fatalf("failed persistence left an in-memory scheduler: %+v", jobs)
	}
}
