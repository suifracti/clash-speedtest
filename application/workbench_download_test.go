package application

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
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
	if failed.Source != history.WorkbenchDownloadSource || failed.Rule.RuleVersion != 1 || failed.Rule.Method != http.MethodGet || failed.Rule.MaximumBytes != 1000 ||
		failed.PersistenceState != "failed" || failed.Result == nil || failed.Result.Outcome != "byte_limit" || failed.Result.BytesRead != 1000 || requests.Load() != 1 {
		t.Fatalf("staged download attempt = %+v; requests=%d", failed, requests.Load())
	}
	if failed.Result.Samples == nil || len(failed.Result.Samples) == 0 || failed.Result.Samples[len(failed.Result.Samples)-1].CumulativeBytes != 1000 {
		t.Fatalf("actual response-byte samples were not retained: %+v", failed.Result.Samples)
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
	if recovered.PersistenceState != "failed" || recovered.Result == nil || recovered.AttemptID != started.AttemptID {
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
