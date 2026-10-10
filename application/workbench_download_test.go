package application

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/profiles"
	"github.com/faceair/clash-speedtest/core/speedtester"
)

type workbenchDownloadRT func(*http.Request) (*http.Response, error)

func (f workbenchDownloadRT) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func downloadResponse(request *http.Request, body io.ReadCloser) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body, Request: request}
}

func newWorkbenchDownloadApplication(t *testing.T) (*AppService, *history.Store, profiles.Paths) {
	t.Helper()
	store, err := history.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	paths := profiles.Paths{Dir: filepath.Join(t.TempDir(), "profiles")}
	app := &AppService{historyStore: store, profilePaths: paths, emitter: NewMemoryEventEmitter(), publicServiceActive: make(map[string]*publicServiceRuntime)}
	app.workbenchDownloadResolveHook = func(profileID, nodeKey string) (monitor.MonitoredNode, *speedtester.CProxy, error) {
		return monitor.MonitoredNode{NodeKey: nodeKey, NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a", DisplayName: "冻结节点", Type: "http", RawConfig: map[string]any{"type": "http"}}, nil, nil
	}
	t.Cleanup(func() {
		app.workbenchMu.Lock()
		active := app.workbenchActiveDownload != nil
		app.workbenchMu.Unlock()
		if !active {
			_ = store.Close()
		}
	})
	return app, store, paths
}

func downloadRequest(id string) WorkbenchDownloadTestRequest {
	return WorkbenchDownloadTestRequest{RequestID: id, ProfileID: "profile-a", NodeKey: "node-a", NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a", MaximumBytes: 1000, TimeoutSeconds: 5}
}

func downloadQuery() WorkbenchDownloadHistoryQuery {
	return WorkbenchDownloadHistoryQuery{ProfileID: "profile-a", NodeKey: "node-a", NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a"}
}

func configureDownloadResponse(app *AppService, roundTrip func(*http.Request) (*http.Response, error)) {
	app.workbenchDownloadClientFactory = func(_ *speedtester.SpeedTester, _ *speedtester.CProxy, _ time.Duration) (*http.Client, error) {
		return &http.Client{Transport: workbenchDownloadRT(roundTrip)}, nil
	}
}

func TestWorkbenchDownloadSaveRetryAndReopenReuseAttemptWithoutRequest(t *testing.T) {
	app, store, paths := newWorkbenchDownloadApplication(t)
	var requests atomic.Int32
	configureDownloadResponse(app, func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		if r.Method != http.MethodGet || r.URL.Host != "speed.cloudflare.com" || !strings.Contains(r.URL.RawQuery, "bytes=1001") {
			t.Errorf("unexpected fixed download request: %s %s", r.Method, r.URL)
		}
		return downloadResponse(r, io.NopCloser(strings.NewReader(strings.Repeat("d", 1400)))), nil
	})
	app.workbenchDownloadSaveHook = func(context.Context, string) error { return errors.New("injected save failure") }

	started, err := app.StartWorkbenchDownloadTest(context.Background(), downloadRequest("download-request-a"))
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := app.StartWorkbenchDownloadTest(context.Background(), downloadRequest("download-request-a"))
	if err != nil || duplicate.AttemptID != started.AttemptID {
		t.Fatalf("repeated request ID must return the same attempt: first=%+v duplicate=%+v err=%v", started, duplicate, err)
	}
	app.workbenchWG.Wait()
	failed, err := app.GetWorkbenchDownloadAttempt(context.Background(), started.AttemptID, downloadQuery())
	if err != nil {
		t.Fatal(err)
	}
	rounds, roundErr := store.DB().QueryMeasurementRoundNodes(context.Background(), "profile-a", "node-a", "identity-a", "revision-a", 16)
	if roundErr != nil || len(rounds) != 1 || rounds[0].State != "finished" || len(rounds[0].Items) != 1 {
		t.Fatalf("worker finished before its single round was durable: %+v %v", rounds, roundErr)
	}
	if failed.Source != history.WorkbenchDownloadSource || failed.Rule.RuleVersion != 3 || failed.Rule.Method != http.MethodGet || failed.Rule.MaximumBytes != 1000 ||
		failed.PersistenceState != "failed" || failed.Result == nil || failed.Result.Outcome != "byte_limit" || failed.Result.BytesRead != 1000 || requests.Load() != 1 {
		t.Fatalf("staged download attempt = %+v; requests=%d", failed, requests.Load())
	}
	if failed.Result.Samples == nil || len(failed.Result.Samples) == 0 || failed.Result.Samples[len(failed.Result.Samples)-1].CumulativeBytes != 1000 {
		t.Fatalf("actual response-byte samples were not retained: %+v", failed.Result.Samples)
	}
	if failed.Result.NetworkPath == nil || failed.Result.NetworkPath.Method != "unknown" || failed.Result.NetworkPath.AddressFamily != "unknown" || failed.Result.NetworkPath.ResolutionSource != "unobserved" || failed.Result.NetworkPath.TUNEvidence != "packet_route_not_observed" {
		t.Fatalf("unverified test path must remain explicitly unknown: %+v", failed.Result.NetworkPath)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := history.NewStore(store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	restarted := NewAppService(reopened, paths, NewMemoryEventEmitter())
	t.Cleanup(func() { _ = restarted.Close() })
	recovered, err := restarted.GetWorkbenchDownloadAttempt(context.Background(), started.AttemptID, downloadQuery())
	if err != nil {
		t.Fatal(err)
	}
	if recovered.PersistenceState != "failed" || recovered.Result == nil || recovered.AttemptID != started.AttemptID || recovered.Result.NetworkPath == nil || recovered.Result.NetworkPath.TUNEvidence != "packet_route_not_observed" {
		t.Fatalf("staged result did not remain retryable after reopen: %+v", recovered)
	}
	saved, err := restarted.RetrySaveWorkbenchDownloadTest(context.Background(), started.AttemptID, downloadQuery())
	if err != nil {
		t.Fatal(err)
	}
	if saved.PersistenceState != "saved" || saved.AttemptID != started.AttemptID || requests.Load() != 1 {
		t.Fatalf("retry must save the same attempt without another download: attempt=%+v requests=%d", saved, requests.Load())
	}
	page, err := restarted.ListWorkbenchDownloadTests(context.Background(), downloadQuery())
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Attempts) != 1 || page.Attempts[0].AttemptID != started.AttemptID || page.Attempts[0].PersistenceState != "saved" {
		t.Fatalf("download history after retry = %+v", page)
	}
}

func TestWorkbenchDownloadRejectsStaleIdentityBeforeNetwork(t *testing.T) {
	app, store, _ := newWorkbenchDownloadApplication(t)
	defer store.Close()
	var requests atomic.Int32
	configureDownloadResponse(app, func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		return downloadResponse(r, io.NopCloser(strings.NewReader("body"))), nil
	})
	app.workbenchDownloadResolveHook = func(profileID, nodeKey string) (monitor.MonitoredNode, *speedtester.CProxy, error) {
		return monitor.MonitoredNode{NodeKey: nodeKey, NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-now", RawConfig: map[string]any{"type": "http"}}, nil, nil
	}
	_, err := app.StartWorkbenchDownloadTest(context.Background(), downloadRequest("stale-download"))
	if err == nil || requests.Load() != 0 {
		t.Fatalf("stale revision must be rejected before network: err=%v requests=%d", err, requests.Load())
	}
}

func TestWorkbenchDownloadCreateFailureSendsNoNetworkRequest(t *testing.T) {
	app, store, _ := newWorkbenchDownloadApplication(t)
	defer store.Close()
	var requests atomic.Int32
	configureDownloadResponse(app, func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		return downloadResponse(r, io.NopCloser(strings.NewReader("body"))), nil
	})
	raw, err := sql.Open("sqlite", filepath.Join(store.Dir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TRIGGER reject_download_attempt BEFORE INSERT ON workbench_download_attempts BEGIN SELECT RAISE(FAIL, 'injected create failure'); END`); err != nil {
		_ = raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = app.StartWorkbenchDownloadTest(context.Background(), downloadRequest("cannot-persist"))
	if err == nil || !strings.Contains(err.Error(), "未发出网络请求") || requests.Load() != 0 {
		t.Fatalf("failed durable attempt creation must send zero requests: err=%v requests=%d", err, requests.Load())
	}
}

type downloadBlockingBody struct {
	ctx     context.Context
	entered chan struct{}
	once    atomic.Bool
}

func (b *downloadBlockingBody) Read([]byte) (int, error) {
	if b.once.CompareAndSwap(false, true) {
		close(b.entered)
	}
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}
func (*downloadBlockingBody) Close() error { return nil }

func TestWorkbenchDownloadCancelAbortsRequestAndBlocksOtherWorkbenchStarts(t *testing.T) {
	app, store, _ := newWorkbenchDownloadApplication(t)
	defer store.Close()
	entered := make(chan struct{})
	configureDownloadResponse(app, func(r *http.Request) (*http.Response, error) {
		return downloadResponse(r, &downloadBlockingBody{ctx: r.Context(), entered: entered}), nil
	})
	started, err := app.StartWorkbenchDownloadTest(context.Background(), downloadRequest("blocking-download"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("download response body read did not start")
	}
	if _, err := app.RunWorkbenchLatencyTest(context.Background(), WorkbenchLatencyTestRequest{ProfileID: "profile-a", NodeKey: "node-a", TestProject: WorkbenchLatencyProject, TimeoutSeconds: 1}); err == nil || !strings.Contains(err.Error(), "下载测量正在运行") {
		t.Fatalf("latency single test must be refused during download: %v", err)
	}
	selection := WorkbenchLatencyBatchSelection{ProfileID: "profile-a", NodeKey: "node-a", NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a"}
	if _, err := app.StartWorkbenchLatencyBatch(context.Background(), WorkbenchLatencyBatchRequest{RequestID: "blocked-latency", TestProject: WorkbenchLatencyProject, TimeoutSeconds: 1, Selections: []WorkbenchLatencyBatchSelection{selection}}); err == nil {
		t.Fatal("latency batch must be refused during download")
	}
	if _, err := app.StartWorkbenchPublicServiceTest(context.Background(), publicServiceRequest("blocked-service", "profile-a", "identity-a", "revision-a", "cloudflare_204")); err == nil || !strings.Contains(err.Error(), "下载测量正在运行") {
		t.Fatalf("public-service test must be refused during download: %v", err)
	}
	if _, err := app.CancelWorkbenchDownloadTest(context.Background(), started.AttemptID, downloadQuery()); err != nil {
		t.Fatal(err)
	}
	app.workbenchWG.Wait()
	completed, err := app.GetWorkbenchDownloadAttempt(context.Background(), started.AttemptID, downloadQuery())
	if err != nil {
		t.Fatal(err)
	}
	if completed.ExecutionState != "user_cancelled" || completed.Result == nil || completed.Result.BytesRead != 0 {
		t.Fatalf("user cancellation must abort the body request: %+v", completed)
	}
}

func TestWorkbenchDownloadCancelDuringPreflightPersistsNotExecuted(t *testing.T) {
	app, store, _ := newWorkbenchDownloadApplication(t)
	defer store.Close()
	var requests atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseWorker := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseWorker()
	ctx, cancel := newWorkbenchDownloadLifecycleContext(context.Background(), 5*time.Second)
	now := time.Now().UTC()
	runtime := &workbenchDownloadRuntime{
		networkDone: make(chan struct{}), attemptID: "preflight-cancel-attempt", requestID: "preflight-cancel-request",
		ctx: ctx, cancel: cancel, ready: make(chan struct{}), node: monitor.MonitoredNode{
			NodeKey: "node-a", NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a", DisplayName: "冻结节点", Type: "http",
		}, rule: history.WorkbenchDownloadRuleSnapshot{
			RuleVersion: workbenchDownloadRuleVersion, TargetURL: "https://speed.cloudflare.com/__down?bytes=1001",
			Method: http.MethodGet, MaximumBytes: 1000, MaximumDurationNS: int64(5 * time.Second),
		}, preparePhysical: func(ctx context.Context, _ *speedtester.CProxy) (*speedtester.CProxy, *speedtester.PhysicalDownloadEgress, error) {
			requests.Add(1)
			close(entered)
			<-ctx.Done()
			<-release
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
			requests.Add(1)
			return nil, nil, nil
		},
	}
	attempt := &history.WorkbenchDownloadAttempt{
		AttemptID: runtime.attemptID, RequestID: runtime.requestID, ProfileID: "profile-a", NodeKey: "node-a",
		NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a", DisplayName: "冻结节点", NodeType: "http",
		Source: history.WorkbenchDownloadSource, RequestedAt: now, ExecutionState: "queued", PersistenceState: "not_started", Rule: runtime.rule,
	}
	if err := store.CreateWorkbenchDownloadAttempt(context.Background(), attempt); err != nil {
		t.Fatal(err)
	}
	if err := store.BeginWorkbenchDownloadAttempt(context.Background(), runtime.attemptID, now); err != nil {
		t.Fatal(err)
	}
	if _, duplicate, err := app.beginWorkbenchDownload(runtime); err != nil || duplicate {
		t.Fatalf("reserve download runtime: duplicate=%t err=%v", duplicate, err)
	}
	close(runtime.ready)
	go app.executeWorkbenchDownload(runtime)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("physical preflight did not start")
	}
	cancelling, err := app.CancelWorkbenchDownloadTest(context.Background(), runtime.attemptID, downloadQuery())
	if err != nil {
		t.Fatal(err)
	}
	if cancelling.ExecutionState != "cancelling" {
		t.Fatalf("cancellation must remain visible until preflight cleanup completes: %s", cancelling.ExecutionState)
	}
	releaseWorker()
	app.workbenchWG.Wait()
	finished, err := app.GetWorkbenchDownloadAttempt(context.Background(), runtime.attemptID, downloadQuery())
	if err != nil {
		t.Fatal(err)
	}
	if finished.ExecutionState != "not_executed" || finished.PersistenceState != "saved" || finished.Result == nil {
		t.Fatalf("preflight cancellation must be a saved not_executed attempt: %+v", finished)
	}
	if finished.Result.Outcome != "not_executed" || finished.Result.EndReason != "user_cancelled" || finished.Result.BytesRead != 0 || len(finished.Result.Samples) != 0 {
		t.Fatalf("preflight cancellation must retain zero-byte evidence: %+v", finished.Result)
	}
	if finished.Result.NetworkPath == nil || finished.Result.NetworkPath.TUNEvidence != "packet_route_not_observed" || finished.Result.NetworkPath.FailureReason != "user_cancelled" {
		t.Fatalf("preflight failure path evidence = %+v", finished.Result.NetworkPath)
	}
	if requests.Load() != 1 {
		t.Fatalf("cancelled preflight issued a later DNS/socket request: requests=%d", requests.Load())
	}
	select {
	case <-runtime.networkDone:
	default:
		t.Fatal("network cleanup did not finish before persistence completed")
	}
}

func TestWorkbenchDownloadNetworkPathEvidenceSurvivesReopen(t *testing.T) {
	_, store, _ := newWorkbenchDownloadApplication(t)
	requestID, attemptID := "path-evidence-request", "path-evidence-attempt"
	now := time.Now().UTC()
	attempt := &history.WorkbenchDownloadAttempt{
		AttemptID: attemptID, RequestID: requestID, ProfileID: "profile-a", NodeKey: "node-a",
		NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a", DisplayName: "冻结节点", NodeType: "vless",
		Source: history.WorkbenchDownloadSource, RequestedAt: now, ExecutionState: "queued", PersistenceState: "not_started",
		Rule: history.WorkbenchDownloadRuleSnapshot{RuleVersion: workbenchDownloadRuleVersion, Method: http.MethodGet, MaximumBytes: 1000, MaximumDurationNS: int64(time.Second)},
	}
	if err := store.CreateWorkbenchDownloadAttempt(context.Background(), attempt); err != nil {
		t.Fatal(err)
	}
	if err := store.BeginWorkbenchDownloadAttempt(context.Background(), attemptID, now); err != nil {
		t.Fatal(err)
	}
	measurement := history.WorkbenchDownloadMeasurement{
		Outcome: "not_executed", ErrorClass: "physical_path", EndReason: "ipv4_dns_not_found", BytesRead: 0,
		StartedAt: now, FinishedAt: now, Samples: []history.WorkbenchDownloadSample{},
		NetworkPath: &speedtester.DownloadNetworkPath{
			Method: "physical_socket_v1", Interface: "en1", AddressFamily: "ipv4",
			ResolutionSource: "physical_ipv4_dns",
			DNSMode:          "physical_interface_dns_v1", TUNEvidence: "packet_route_not_observed",
			FailureReason: "ipv4_dns_not_found", DNSRequests: 2, DNSDialAttempts: 1,
		},
	}
	if err := store.StageWorkbenchDownloadResult(context.Background(), attemptID, "not_executed", measurement); err != nil {
		t.Fatal(err)
	}
	if err := store.CommitWorkbenchDownloadResult(context.Background(), attemptID); err != nil {
		t.Fatal(err)
	}
	storeDir := store.Dir()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := history.NewStore(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.GetWorkbenchDownloadAttempt(context.Background(), attemptID, history.WorkbenchDownloadFilter{
		ProfileID: "profile-a", NodeKey: "node-a", NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	path := got.Result.NetworkPath
	if got.ExecutionState != "not_executed" || path == nil || path.Method != "physical_socket_v1" || path.AddressFamily != "ipv4" || path.ResolutionSource != "physical_ipv4_dns" || path.Interface != "en1" || path.FailureReason != "ipv4_dns_not_found" || path.DNSRequests != 2 || path.DNSDialAttempts != 1 || path.TUNEvidence != "packet_route_not_observed" {
		t.Fatalf("persisted path evidence changed after reopen: %+v; attempt=%+v", path, got)
	}
}

func TestWorkbenchDownloadDeadlineDuringPreflightPersistsNotExecuted(t *testing.T) {
	app, store, _ := newWorkbenchDownloadApplication(t)
	defer store.Close()
	ctx, cancel := newWorkbenchDownloadLifecycleContext(context.Background(), 150*time.Millisecond)
	started := time.Now().UTC()
	entered := make(chan struct{})
	runtime := &workbenchDownloadRuntime{
		timingNS: make(map[string]int64), networkDone: make(chan struct{}), attemptID: "preflight-deadline-attempt",
		requestID: "preflight-deadline-request", ctx: ctx, cancel: cancel, ready: make(chan struct{}),
		node: monitor.MonitoredNode{NodeKey: "node-a", NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a", DisplayName: "冻结节点", Type: "http"},
		rule: history.WorkbenchDownloadRuleSnapshot{RuleVersion: workbenchDownloadRuleVersion, TargetURL: "https://speed.cloudflare.com/__down?bytes=1001", Method: http.MethodGet, MaximumBytes: 1000, MaximumDurationNS: int64(150 * time.Millisecond)},
		preparePhysical: func(ctx context.Context, _ *speedtester.CProxy) (*speedtester.CProxy, *speedtester.PhysicalDownloadEgress, error) {
			close(entered)
			<-ctx.Done()
			return nil, nil, ctx.Err()
		},
	}
	attempt := &history.WorkbenchDownloadAttempt{
		AttemptID: runtime.attemptID, RequestID: runtime.requestID, ProfileID: "profile-a", NodeKey: "node-a",
		NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a", DisplayName: "冻结节点", NodeType: "http",
		Source: history.WorkbenchDownloadSource, RequestedAt: started, ExecutionState: "queued", PersistenceState: "not_started", Rule: runtime.rule,
	}
	if err := store.CreateWorkbenchDownloadAttempt(context.Background(), attempt); err != nil {
		t.Fatal(err)
	}
	if err := store.BeginWorkbenchDownloadAttempt(context.Background(), runtime.attemptID, started); err != nil {
		t.Fatal(err)
	}
	if _, duplicate, err := app.beginWorkbenchDownload(runtime); err != nil || duplicate {
		t.Fatalf("reserve download runtime: duplicate=%t err=%v", duplicate, err)
	}
	close(runtime.ready)
	go app.executeWorkbenchDownload(runtime)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("physical preflight did not start before its task deadline")
	}
	app.workbenchWG.Wait()
	finished, err := app.GetWorkbenchDownloadAttempt(context.Background(), runtime.attemptID, downloadQuery())
	if err != nil {
		t.Fatal(err)
	}
	if finished.ExecutionState != "not_executed" || finished.PersistenceState != "saved" || finished.Result == nil || finished.Result.EndReason != "deadline_exceeded" || finished.Result.BytesRead != 0 || len(finished.Result.Samples) != 0 {
		t.Fatalf("preflight deadline must persist a zero-byte not_executed result: %+v", finished)
	}
}

func TestUnverifiedPhysicalPathDoesNotTurnConnectionFailureIntoNodeFailure(t *testing.T) {
	result := speedtester.DownloadStreamResult{
		Outcome: "connection_failed", EndReason: "connection_failed", ErrorClass: "tls",
		FailurePhase: "tls", ErrorMessage: "TLS setup failed",
	}
	path := speedtester.DownloadNetworkPath{Method: "legacy_default_binding_unverified", FailureReason: "physical_binding_unavailable"}
	normalizeUnverifiedPathFailure(&result, path)
	if result.Outcome != "local_path_unverified" || result.EndReason != "local_path_unverified" || result.ErrorClass != "tls" || result.FailurePhase != "tls" {
		t.Fatalf("unknown local egress was misclassified as node failure or lost its TLS evidence: %+v", result)
	}
	rateLimited := speedtester.DownloadStreamResult{Outcome: "source_rate_limited", EndReason: "source_rate_limited"}
	normalizeUnverifiedPathFailure(&rateLimited, path)
	if rateLimited.Outcome != "source_rate_limited" {
		t.Fatalf("source rate limit was conflated with unknown egress: %+v", rateLimited)
	}
}

func TestCloseWorkbenchDownloadProxyAllowsMissingProxyAdapter(t *testing.T) {
	closeWorkbenchDownloadProxy(nil)
	closeWorkbenchDownloadProxy(&speedtester.CProxy{})
}

func TestWorkbenchDownloadAdmissionRejectsExistingActiveWorkbenchTest(t *testing.T) {
	app, store, _ := newWorkbenchDownloadApplication(t)
	defer store.Close()
	var requests atomic.Int32
	configureDownloadResponse(app, func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		return downloadResponse(r, io.NopCloser(strings.NewReader("body"))), nil
	})
	app.workbenchMu.Lock()
	app.workbenchActiveSingles = 1
	app.workbenchMu.Unlock()
	_, err := app.StartWorkbenchDownloadTest(context.Background(), downloadRequest("download-during-latency"))
	app.workbenchMu.Lock()
	app.workbenchActiveSingles = 0
	app.workbenchMu.Unlock()
	if err == nil || requests.Load() != 0 {
		t.Fatalf("download must reject an existing latency operation before networking: err=%v requests=%d", err, requests.Load())
	}
}

func TestWorkbenchDownloadAdmissionRejectsExistingPublicServiceAttempt(t *testing.T) {
	app, store, _ := newWorkbenchDownloadApplication(t)
	defer store.Close()
	var requests atomic.Int32
	configureDownloadResponse(app, func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		return downloadResponse(r, io.NopCloser(strings.NewReader("body"))), nil
	})
	app.publicServiceMu.Lock()
	app.publicServiceActive["already-running"] = &publicServiceRuntime{attemptID: "already-running", cancel: func() {}}
	app.publicServiceMu.Unlock()
	_, err := app.StartWorkbenchDownloadTest(context.Background(), downloadRequest("download-during-service"))
	if err == nil || requests.Load() != 0 {
		t.Fatalf("download must reject an existing public-service test before networking: err=%v requests=%d", err, requests.Load())
	}
}
