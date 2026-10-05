package application

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/appdata"
	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/profiles"
)

func TestMonitorJobListKeepsCreationOrderAcrossRefreshes(t *testing.T) {
	created := time.Date(2026, time.October, 3, 0, 0, 0, 0, time.UTC)
	svc := &AppService{monitorSchedulers: make(map[string]*monitor.Scheduler)}
	runner := monitor.NewRunner(monitor.RunnerConfig{})
	for _, job := range []*monitor.MonitorJob{
		{ID: "older-z", CreatedAt: created},
		{ID: "newer-a", CreatedAt: created.Add(time.Hour)},
		{ID: "older-a", CreatedAt: created},
	} {
		scheduler, err := monitor.NewScheduler(monitor.SchedulerConfig{Job: job, Runner: runner})
		if err != nil {
			t.Fatal(err)
		}
		svc.monitorSchedulers[job.ID] = scheduler
	}
	want := []string{"older-a", "older-z", "newer-a"}
	for refresh := 0; refresh < 20; refresh++ {
		jobs := svc.ListMonitorJobs()
		for i, id := range want {
			if jobs[i].ID != id {
				t.Fatalf("refresh %d moved job at position %d: got %s, want %s", refresh, i, jobs[i].ID, id)
			}
		}
	}
}

func TestMonitorSubscriptionIdentityCreatesAndReopens(t *testing.T) {
	paths := profiles.Paths{Dir: filepath.Join(t.TempDir(), "profiles")}
	if err := profiles.SaveStore(paths.StoreFile(), &profiles.Store{Airports: []*profiles.Airport{
		{ID: "airport", Name: "Airport", Subscriptions: []*profiles.Subscription{
			{ID: "sub-main", Name: "Main"}, {ID: "sub-backup", Name: "Backup"},
		}},
		{ID: "legacy", Name: "Legacy"},
	}}); err != nil {
		t.Fatal(err)
	}
	for _, profileID := range []string{"sub-main", "sub-backup", "legacy"} {
		writeP0MonitorProfileFixture(t, paths, profileID, "fixture-password", "127.0.0.1")
	}
	historyDir := filepath.Join(t.TempDir(), "history")
	store, err := history.NewStore(historyDir)
	if err != nil {
		t.Fatal(err)
	}
	appPaths := appdata.FromLegacy(paths.Dir, historyDir)
	svc := NewAppServiceWithOptions(store, appPaths, nil, Options{NoAutoCredentials: true})
	t.Cleanup(func() { _ = svc.Close() })
	options, err := svc.ListMonitorNodeOptions()
	if err != nil || len(options) != 3 {
		t.Fatalf("node options: count=%d err=%v", len(options), err)
	}
	expectedNames := map[string]string{
		"sub-main": "Airport · Main", "sub-backup": "Airport · Backup",
		"airport": "Airport", "legacy": "Legacy",
	}
	for _, option := range options {
		profileIDs := []string{option.ProfileID}
		if option.ProfileID == "sub-main" {
			profileIDs = append(profileIDs, "airport")
		}
		for _, profileID := range profileIDs {
			created, err := svc.CreateMonitorJobFromRequest(MonitorJobCreateRequest{
				ProfileID: profileID, NodeKeys: []string{option.NodeKey},
				NodeContexts: []MonitorNodeSelectionContext{{NodeKey: option.NodeKey,
					NodeIdentityKey: option.NodeIdentityKey, ConfigRevisionKey: option.ConfigRevisionKey}},
				ProbeSet: monitor.ProbeSetLight, IntervalSeconds: 3600, TimeoutSeconds: 1,
			})
			if err != nil {
				t.Fatalf("create monitor for %s: %v", profileID, err)
			}
			if created.ProfileID != profileID || created.ProfileName != expectedNames[profileID] {
				t.Fatalf("created monitor lost profile identity: %+v", created)
			}
		}
	}
	checkJobs := func(service *AppService) {
		t.Helper()
		jobs, err := service.ListMonitorJobDTOs()
		if err != nil || len(jobs) != len(expectedNames) {
			t.Fatalf("monitor jobs: count=%d err=%v", len(jobs), err)
		}
		for _, job := range jobs {
			if job.ProfileName != expectedNames[job.ProfileID] || job.State != monitor.JobStateStopped || job.BlockedReason != "" {
				t.Fatalf("monitor profile or stopped state changed: %+v", job)
			}
			detail, err := service.GetMonitorJobDTO(job.ID)
			if err != nil || detail.ProfileName != job.ProfileName {
				t.Fatalf("monitor list/detail profile mismatch: detail=%+v err=%v", detail, err)
			}
		}
	}
	checkJobs(svc)
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	reopenedStore, err := history.NewStore(historyDir)
	if err != nil {
		t.Fatal(err)
	}
	reopened := NewAppServiceWithOptions(reopenedStore, appPaths, nil, Options{NoAutoCredentials: true})
	t.Cleanup(func() { _ = reopened.Close() })
	checkJobs(reopened)
	samples, err := reopened.QueryMonitorSamples(context.Background(), monitor.SampleFilter{})
	if err != nil || len(samples) != 0 {
		t.Fatalf("monitor creation/reopen unexpectedly produced samples: count=%d err=%v", len(samples), err)
	}
}

func TestMonitorJobSelectionResolvesCachedConfigAndKeepsPublicDTOCredentialFree(t *testing.T) {
	profileDir := t.TempDir()
	paths := profiles.Paths{Dir: profileDir}
	if err := profiles.SaveStore(paths.StoreFile(), &profiles.Store{Airports: []*profiles.Airport{{
		ID:   "profile-1",
		Name: "Test Subscription",
	}}}); err != nil {
		t.Fatalf("SaveStore: %v", err)
	}
	cache := []byte("proxies:\n" +
		"  - name: Test Node\n" +
		"    type: ss\n" +
		"    server: 127.0.0.1\n" +
		"    port: 1\n" +
		"    cipher: aes-128-gcm\n" +
		"    password: subscription-secret\n")
	if err := paths.WriteCache("profile-1", cache); err != nil {
		t.Fatalf("WriteCache: %v", err)
	}

	store, err := history.NewStore(filepath.Join(t.TempDir(), "history"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	svc := NewAppService(store, paths, nil)
	defer func() { _ = svc.Close() }()

	options, err := svc.ListMonitorNodeOptions()
	if err != nil {
		t.Fatalf("ListMonitorNodeOptions: %v", err)
	}
	if len(options) != 1 {
		t.Fatalf("expected one safe option, got %d: %+v", len(options), options)
	}
	if options[0].NodeKey == "" || options[0].NodeIdentityKey == "" || options[0].ConfigRevisionKey == "" {
		t.Fatalf("expected stable keys from cached config, got %+v", options[0])
	}
	optionJSON, err := json.Marshal(options[0])
	if err != nil {
		t.Fatalf("marshal option: %v", err)
	}
	if strings.Contains(string(optionJSON), "subscription-secret") || strings.Contains(string(optionJSON), "raw_config") {
		t.Fatalf("safe node option leaked raw subscription config: %s", optionJSON)
	}

	created, err := svc.CreateMonitorJobFromRequest(MonitorJobCreateRequest{
		Name:      "UI job",
		ProfileID: "profile-1",
		NodeKeys:  []string{options[0].NodeKey},
		NodeContexts: []MonitorNodeSelectionContext{{
			NodeKey:           options[0].NodeKey,
			NodeIdentityKey:   options[0].NodeIdentityKey,
			ConfigRevisionKey: options[0].ConfigRevisionKey,
		}},
		ProbeSet:        monitor.ProbeSetLight,
		IntervalSeconds: 10,
		TimeoutSeconds:  5,
	})
	if err != nil {
		t.Fatalf("CreateMonitorJobFromRequest: %v", err)
	}
	if created.IntervalSeconds != 10 || created.TimeoutSeconds != 5 || created.State != monitor.JobStateStopped || created.SamplingTier != monitor.SamplingTierRegular {
		t.Fatalf("unexpected public job DTO: %+v", created)
	}
	createdJSON, err := json.Marshal(created)
	if err != nil {
		t.Fatalf("marshal created job: %v", err)
	}
	if strings.Contains(string(createdJSON), "subscription-secret") || strings.Contains(string(createdJSON), "raw_config") {
		t.Fatalf("public job DTO leaked raw subscription config: %s", createdJSON)
	}

	internal, err := svc.GetMonitorJob(created.ID)
	if err != nil {
		t.Fatalf("GetMonitorJob: %v", err)
	}
	if len(internal.Nodes) != 1 || len(internal.Nodes[0].RawConfig) == 0 {
		t.Fatalf("scheduler did not retain resolved runnable config: %+v", internal)
	}

	if _, err := svc.CreateMonitorJobFromRequest(MonitorJobCreateRequest{
		ProfileID:       "profile-1",
		NodeKeys:        []string{"display-name-is-not-a-key"},
		ProbeSet:        monitor.ProbeSetLight,
		IntervalSeconds: 10,
		TimeoutSeconds:  5,
	}); err == nil || !monitor.IsValidationError(err) {
		t.Fatalf("expected stale/display-name key to be rejected, got %v", err)
	}
	if _, err := svc.CreateMonitorJobFromRequest(MonitorJobCreateRequest{
		ProfileID: "profile-1",
		NodeKeys:  []string{options[0].NodeKey},
		NodeContexts: []MonitorNodeSelectionContext{{
			NodeKey:           options[0].NodeKey,
			NodeIdentityKey:   options[0].NodeIdentityKey,
			ConfigRevisionKey: "stale-revision",
		}},
		ProbeSet:        monitor.ProbeSetLight,
		IntervalSeconds: 10,
		TimeoutSeconds:  5,
	}); err == nil || !monitor.IsValidationError(err) || !strings.Contains(err.Error(), "revision") {
		t.Fatalf("expected stale revision to be rejected before create, got %v", err)
	}
	if len(svc.ListMonitorJobs()) != 1 {
		t.Fatalf("stale Workbench context created an extra job: %+v", svc.ListMonitorJobs())
	}
}
