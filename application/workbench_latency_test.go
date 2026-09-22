package application

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/profiles"
	"gopkg.in/yaml.v2"
)

func TestWorkbenchLatencyHistoryWindowProjectsStatsFromRawSamples(t *testing.T) {
	ctx := context.Background()
	store, err := history.NewStore(filepath.Join(t.TempDir(), "history"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	asOf := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	since := asOf.Add(-4 * time.Hour)
	until := asOf
	attemptID := "windowed-attempt"
	if err := store.SaveLatencyTest(ctx, &history.LatencyTest{
		AttemptID:      attemptID,
		ProfileID:      "profile-window",
		NodeKey:        "node-window",
		TestProject:    WorkbenchLatencyProject,
		RequestedAt:    since,
		StartedAt:      since,
		FinishedAt:     until,
		Status:         "partial_failed",
		LatencyMs:      99,
		PacketLoss:     50,
		TotalSamples:   4,
		SuccessSamples: 2,
		FailureSamples: 2,
		Samples: []history.LatencyTestSample{
			{Seq: 1, Timestamp: since.Add(-time.Second), LatencyMs: 9, Success: true},
			{Seq: 2, Timestamp: since, LatencyMs: 40, Success: true},
			{Seq: 3, Timestamp: until.Add(-time.Second), LatencyMs: 44, Success: false, Error: "timeout"},
			{Seq: 4, Timestamp: until, LatencyMs: 80, Success: true},
		},
	}); err != nil {
		t.Fatalf("SaveLatencyTest: %v", err)
	}

	service := &AppService{historyStore: store}
	result, err := service.ListWorkbenchLatencyTests(ctx, WorkbenchLatencyHistoryQuery{
		ProfileID: "profile-window",
		NodeKey:   "node-window",
		Since:     &since,
		Until:     &until,
	})
	if err != nil {
		t.Fatalf("ListWorkbenchLatencyTests: %v", err)
	}
	if !result.Complete || result.HasMore || !result.Since.Equal(since) || !result.Until.Equal(until) || !result.AsOf.Equal(until) {
		t.Fatalf("unexpected frozen window metadata: %+v", result)
	}
	if len(result.Tests) != 1 {
		t.Fatalf("expected one windowed attempt, got %+v", result.Tests)
	}
	windowed := result.Tests[0]
	if len(windowed.Samples) != 2 || windowed.Samples[0].Timestamp != since || windowed.Samples[1].Timestamp != until.Add(-time.Second) {
		t.Fatalf("unexpected half-open raw samples: %+v", windowed.Samples)
	}
	if windowed.TotalSamples != 2 || windowed.SuccessSamples != 1 || windowed.FailureSamples != 1 || windowed.Status != "partial_failed" || windowed.LatencyMs != 40 || windowed.PacketLoss != 50 {
		t.Fatalf("window stats used non-window samples: %+v", windowed)
	}

	detail, err := service.GetWorkbenchLatencyTest(ctx, WorkbenchLatencyHistoryDetailQuery{
		ProfileID: "profile-window",
		NodeKey:   "node-window",
		AttemptID: attemptID,
		Since:     &since,
		Until:     &until,
	})
	if err != nil {
		t.Fatalf("GetWorkbenchLatencyTest: %v", err)
	}
	if len(detail.Samples) != 2 || detail.Samples[0].Timestamp != since || !detail.Samples[1].Timestamp.Before(until) {
		t.Fatalf("windowed detail leaked raw samples: %+v", detail.Samples)
	}
}

func TestWorkbenchLatencyTestUsesStableIdentityPersistsAndSeparatesProfiles(t *testing.T) {
	proxyA := newLatencyProxy(t, 0)
	proxyB := newLatencyProxy(t, 0)
	profileDir := t.TempDir()
	paths := profiles.Paths{Dir: profileDir}
	if err := profiles.SaveStore(paths.StoreFile(), &profiles.Store{Airports: []*profiles.Airport{
		{ID: "profile-a", Name: "订阅 A"},
		{ID: "profile-b", Name: "订阅 B"},
	}}); err != nil {
		t.Fatalf("SaveStore: %v", err)
	}
	writeLatencyProxyCache(t, paths, "profile-a", "相同名称节点", proxyA)
	writeLatencyProxyCache(t, paths, "profile-b", "相同名称节点", proxyB)

	store, err := history.NewStore(filepath.Join(t.TempDir(), "history"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	emitter := NewMemoryEventEmitter()
	persistenceEvents := subscribeLatencyPersistence(emitter)
	service := &AppService{
		historyStore: store,
		profilePaths: paths,
		emitter:      emitter,
	}

	options, err := service.ListMonitorNodeOptions()
	if err != nil {
		t.Fatalf("ListMonitorNodeOptions: %v", err)
	}
	if len(options) != 2 || options[0].DisplayName != "相同名称节点" || options[1].DisplayName != "相同名称节点" {
		t.Fatalf("expected two same-name options from separate profiles, got %+v", options)
	}
	var optionA, optionB MonitorNodeOptionDTO
	for _, option := range options {
		switch option.ProfileID {
		case "profile-a":
			optionA = option
		case "profile-b":
			optionB = option
		}
	}
	if optionA.NodeKey == "" || optionB.NodeKey == "" || optionA.NodeKey == optionB.NodeKey {
		t.Fatalf("same-name profiles must have distinct stable keys: A=%+v B=%+v", optionA, optionB)
	}

	result, err := service.RunWorkbenchLatencyTest(context.Background(), WorkbenchLatencyTestRequest{
		ProfileID:      "profile-a",
		NodeKey:        optionA.NodeKey,
		TestProject:    WorkbenchLatencyProject,
		TimeoutSeconds: 1,
	})
	if err != nil {
		t.Fatalf("RunWorkbenchLatencyTest: %v", err)
	}
	if result.PersistenceState != "saving" || result.ProfileID != "profile-a" || result.NodeKey != optionA.NodeKey {
		t.Fatalf("unexpected saved result: %+v", result)
	}
	if len(result.Samples) == 0 || result.TotalSamples != len(result.Samples) {
		t.Fatalf("expected raw latency samples, got %+v", result)
	}
	saved := waitForLatencyPersistence(t, persistenceEvents, result.AttemptID)
	if saved.PersistenceState != "saved" {
		t.Fatalf("expected asynchronous persistence success, got %+v", saved)
	}

	profileAHistory, err := service.ListWorkbenchLatencyTests(context.Background(), WorkbenchLatencyHistoryQuery{
		ProfileID: "profile-a",
		NodeKey:   optionA.NodeKey,
	})
	if err != nil || len(profileAHistory.Tests) != 1 {
		t.Fatalf("expected one profile A history row, got %d, err=%v", len(profileAHistory.Tests), err)
	}
	profileBHistory, err := service.ListWorkbenchLatencyTests(context.Background(), WorkbenchLatencyHistoryQuery{
		ProfileID: "profile-b",
		NodeKey:   optionB.NodeKey,
	})
	if err != nil {
		t.Fatalf("profile B history query: %v", err)
	}
	if len(profileBHistory.Tests) != 0 {
		t.Fatalf("same display name history leaked across profiles: %+v", profileBHistory.Tests)
	}

	// A cache revision change invalidates the old stable key instead of falling back to a name.
	writeLatencyProxyCache(t, paths, "profile-a", "相同名称节点", proxyB)
	if _, err := service.RunWorkbenchLatencyTest(context.Background(), WorkbenchLatencyTestRequest{
		ProfileID:      "profile-a",
		NodeKey:        optionA.NodeKey,
		TestProject:    WorkbenchLatencyProject,
		TimeoutSeconds: 1,
	}); err == nil || !monitor.IsValidationError(err) {
		t.Fatalf("expected changed cache key to be rejected, got %v", err)
	}

	// A reachable subscription entry can still produce a persisted all-failure result.
	unavailable := closedLatencyProxy(t)
	writeLatencyProxyCache(t, paths, "profile-a", "相同名称节点", unavailable)
	refreshedOptions, err := service.ListMonitorNodeOptions()
	if err != nil {
		t.Fatalf("refresh options for failure case: %v", err)
	}
	var failureKey string
	for _, option := range refreshedOptions {
		if option.ProfileID == "profile-a" {
			failureKey = option.NodeKey
		}
	}
	failureResult, err := service.RunWorkbenchLatencyTest(context.Background(), WorkbenchLatencyTestRequest{
		ProfileID:      "profile-a",
		NodeKey:        failureKey,
		TestProject:    WorkbenchLatencyProject,
		TimeoutSeconds: 1,
	})
	if err != nil {
		t.Fatalf("connection failure test: %v", err)
	}
	if failureResult.PersistenceState != "saving" || failureResult.FailureSamples == 0 || failureResult.ErrorMessage == "" {
		t.Fatalf("expected persisted connection failure evidence: %+v", failureResult)
	}
	failureSaved := waitForLatencyPersistence(t, persistenceEvents, failureResult.AttemptID)
	if failureSaved.PersistenceState != "saved" {
		t.Fatalf("expected connection failure record to be saved, got %+v", failureSaved)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("close before reopen: %v", err)
	}
	reopened, err := history.NewStore(store.Dir())
	if err != nil {
		t.Fatalf("reopen history: %v", err)
	}
	defer reopened.Close()
	reopenedHistory, err := reopened.GetLatencyTest(context.Background(), result.AttemptID)
	if err != nil {
		t.Fatalf("reopen query: %v", err)
	}
	if reopenedHistory.ProfileID != "profile-a" || len(reopenedHistory.Samples) != len(result.Samples) {
		t.Fatalf("reopened record mismatch: %+v", reopenedHistory)
	}

	// A stale key cannot fall back to the same display name.
	service.historyStore = reopened
	_, err = service.RunWorkbenchLatencyTest(context.Background(), WorkbenchLatencyTestRequest{
		ProfileID:      "profile-a",
		NodeKey:        "相同名称节点",
		TestProject:    WorkbenchLatencyProject,
		TimeoutSeconds: 1,
	})
	if err == nil || !monitor.IsValidationError(err) {
		t.Fatalf("expected display name fallback to be rejected, got %v", err)
	}
}

func TestWorkbenchLatencyTestReportsPersistenceFailureSeparately(t *testing.T) {
	proxy := newLatencyProxy(t, 0)
	profileDir := t.TempDir()
	paths := profiles.Paths{Dir: profileDir}
	if err := profiles.SaveStore(paths.StoreFile(), &profiles.Store{Airports: []*profiles.Airport{{ID: "profile-a", Name: "订阅 A"}}}); err != nil {
		t.Fatalf("SaveStore: %v", err)
	}
	writeLatencyProxyCache(t, paths, "profile-a", "节点", proxy)

	store, err := history.NewStore(filepath.Join(t.TempDir(), "history"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	emitter := NewMemoryEventEmitter()
	persistenceEvents := subscribeLatencyPersistence(emitter)
	service := &AppService{historyStore: store, profilePaths: paths, emitter: emitter}
	options, err := service.ListMonitorNodeOptions()
	if err != nil || len(options) != 1 {
		t.Fatalf("node options: %v %+v", err, options)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close history: %v", err)
	}

	result, err := service.RunWorkbenchLatencyTest(context.Background(), WorkbenchLatencyTestRequest{
		ProfileID:      "profile-a",
		NodeKey:        options[0].NodeKey,
		TestProject:    WorkbenchLatencyProject,
		TimeoutSeconds: 1,
	})
	if err != nil {
		t.Fatalf("run with closed store: %v", err)
	}
	if result.PersistenceState != "saving" {
		t.Fatalf("expected result-first saving state, got %+v", result)
	}
	if len(result.Samples) == 0 {
		t.Fatalf("test result must remain visible when persistence fails: %+v", result)
	}
	failed := waitForLatencyPersistence(t, persistenceEvents, result.AttemptID)
	if failed.PersistenceState != "failed" || failed.PersistenceError == "" {
		t.Fatalf("expected independent persistence failure, got %+v", failed)
	}
}

func TestWorkbenchLatencyTestReturnsBeforeSlowPersistence(t *testing.T) {
	proxy := newLatencyProxy(t, 0)
	paths := profiles.Paths{Dir: t.TempDir()}
	if err := profiles.SaveStore(paths.StoreFile(), &profiles.Store{Airports: []*profiles.Airport{{ID: "profile-slow", Name: "慢保存订阅"}}}); err != nil {
		t.Fatalf("SaveStore: %v", err)
	}
	writeLatencyProxyCache(t, paths, "profile-slow", "慢保存节点", proxy)

	store, err := history.NewStore(filepath.Join(t.TempDir(), "history"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	emitter := NewMemoryEventEmitter()
	persistenceEvents := subscribeLatencyPersistence(emitter)
	saveStarted := make(chan struct{})
	releaseSave := make(chan struct{})
	var releaseOnce sync.Once
	service := &AppService{
		historyStore: store,
		profilePaths: paths,
		emitter:      emitter,
		latencySaveHook: func(ctx context.Context, _ *history.LatencyTest) error {
			close(saveStarted)
			select {
			case <-releaseSave:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
	release := func() { releaseOnce.Do(func() { close(releaseSave) }) }
	t.Cleanup(func() {
		release()
		_ = service.Close()
	})

	options, err := service.ListMonitorNodeOptions()
	if err != nil || len(options) != 1 {
		t.Fatalf("node options: %v %+v", err, options)
	}
	resultCh := make(chan struct {
		result *WorkbenchLatencyTestDTO
		err    error
	}, 1)
	go func() {
		result, runErr := service.RunWorkbenchLatencyTest(context.Background(), WorkbenchLatencyTestRequest{
			ProfileID:      "profile-slow",
			NodeKey:        options[0].NodeKey,
			TestProject:    WorkbenchLatencyProject,
			TimeoutSeconds: 1,
		})
		resultCh <- struct {
			result *WorkbenchLatencyTestDTO
			err    error
		}{result: result, err: runErr}
	}()

	var result *WorkbenchLatencyTestDTO
	select {
	case outcome := <-resultCh:
		if outcome.err != nil {
			t.Fatalf("RunWorkbenchLatencyTest: %v", outcome.err)
		}
		result = outcome.result
	case <-time.After(3 * time.Second):
		t.Fatal("measurement result was blocked by slow persistence")
	}
	if result == nil || result.PersistenceState != "saving" || len(result.Samples) == 0 {
		t.Fatalf("expected result-first saving response, got %+v", result)
	}
	select {
	case <-saveStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("slow persistence hook did not start")
	}
	release()
	final := waitForLatencyPersistence(t, persistenceEvents, result.AttemptID)
	if final.PersistenceState != "saved" {
		t.Fatalf("expected delayed save to finish successfully, got %+v", final)
	}
}

func subscribeLatencyPersistence(emitter *MemoryEventEmitter) <-chan WorkbenchLatencyTestDTO {
	events := make(chan WorkbenchLatencyTestDTO, 8)
	emitter.Subscribe(func(event Event) {
		if event.Type != "workbench_latency_test_persistence_updated" {
			return
		}
		dto, ok := event.Payload.(WorkbenchLatencyTestDTO)
		if ok {
			events <- dto
		}
	})
	return events
}

func waitForLatencyPersistence(t *testing.T, events <-chan WorkbenchLatencyTestDTO, attemptID string) WorkbenchLatencyTestDTO {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case dto := <-events:
		if dto.AttemptID == attemptID {
			return dto
		}
		return waitForLatencyPersistence(t, events, attemptID)
	case <-timer.C:
		t.Fatalf("timed out waiting for persistence update for %s", attemptID)
		return WorkbenchLatencyTestDTO{}
	}
}

func newLatencyProxy(t *testing.T, delay time.Duration) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if delay > 0 {
			time.Sleep(delay)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse proxy URL: %v", err)
	}
	return parsed.Host
}

func closedLatencyProxy(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve unavailable proxy port: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close unavailable proxy port: %v", err)
	}
	return address
}

func writeLatencyProxyCache(t *testing.T, paths profiles.Paths, profileID, name, proxyHost string) {
	t.Helper()
	parts := strings.Split(proxyHost, ":")
	if len(parts) != 2 {
		t.Fatalf("unexpected proxy host %q", proxyHost)
	}
	port, err := strconv.Atoi(parts[1])
	if err != nil {
		t.Fatalf("proxy port: %v", err)
	}
	body, err := yaml.Marshal(map[string]any{"proxies": []map[string]any{{
		"name":   name,
		"type":   "http",
		"server": parts[0],
		"port":   port,
	}}})
	if err != nil {
		t.Fatalf("marshal cache: %v", err)
	}
	if err := paths.WriteCache(profileID, body); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	if _, err := os.Stat(paths.CacheFile(profileID)); err != nil {
		t.Fatalf("cache not written: %v", err)
	}
	if !strings.Contains(string(body), fmt.Sprintf("port: %d", port)) {
		t.Fatalf("unexpected cache body: %s", body)
	}
}
