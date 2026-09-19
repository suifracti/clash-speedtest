package application

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/profiles"
)

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
		Name:            "UI job",
		ProfileID:       "profile-1",
		NodeKeys:        []string{options[0].NodeKey},
		ProbeSet:        monitor.ProbeSetLight,
		IntervalSeconds: 10,
		TimeoutSeconds:  5,
	})
	if err != nil {
		t.Fatalf("CreateMonitorJobFromRequest: %v", err)
	}
	if created.IntervalSeconds != 10 || created.TimeoutSeconds != 5 || created.State != monitor.JobStateStopped {
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
}
