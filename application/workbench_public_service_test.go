package application

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/publicservice"
)

type publicServiceRoundTripFunc func(*http.Request) (*http.Response, error)

func (f publicServiceRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func publicServiceHTTPResponse(request *http.Request, status int, contentType, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}
}

func newPublicServiceApplication(t *testing.T, resolver func(string, string) (monitor.MonitoredNode, error), rt publicServiceRoundTripFunc) (*AppService, *history.Store) {
	t.Helper()
	store, err := history.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := &AppService{historyStore: store, emitter: NewMemoryEventEmitter(), publicServiceActive: make(map[string]*publicServiceRuntime)}
	app.publicServiceResolveHook = resolver
	app.publicServiceChecker = publicservice.Checker{ClientFactory: func(monitor.MonitoredNode, time.Duration) (*http.Client, error) {
		return &http.Client{Transport: rt}, nil
	}}
	t.Cleanup(func() {
		app.publicServiceMu.Lock()
		active := len(app.publicServiceActive)
		app.publicServiceMu.Unlock()
		if active == 0 {
			_ = store.Close()
		}
	})
	return app, store
}

func publicServiceRequest(requestID, profile, identity, revision, serviceID string) WorkbenchPublicServiceTestRequest {
	return WorkbenchPublicServiceTestRequest{
		RequestID: requestID, ProfileID: profile, NodeKey: "node-same", NodeIdentityKey: identity,
		ConfigRevisionKey: revision, ServiceID: serviceID,
	}
}

func TestWorkbenchAntigravityRequestAndSavedEvidence(t *testing.T) {
	var reached atomic.Bool
	app, _ := newPublicServiceApplication(t, func(profile, node string) (monitor.MonitoredNode, error) {
		return monitor.MonitoredNode{NodeKey: node, NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a"}, nil
	}, func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.String() != "https://daily-cloudcode-pa.googleapis.com/v1internal:loadCodeAssist" || request.Header.Get("Authorization") != "Bearer test-token" {
			return nil, errors.New("unexpected Antigravity request contract")
		}
		reached.Store(true)
		return publicServiceHTTPResponse(request, http.StatusUnauthorized, "application/json", `{"error":{"status":"UNAUTHENTICATED"}}`), nil
	})
	app.antigravityToken = "test-token"
	started, err := app.StartWorkbenchPublicServiceTest(context.Background(), publicServiceRequest("antigravity-evidence", "profile-a", "identity-a", "revision-a", "antigravity"))
	if err != nil {
		t.Fatal(err)
	}
	app.publicServiceWG.Wait()
	saved, err := app.GetWorkbenchPublicServiceAttempt(context.Background(), started.AttemptID, publicServiceQueryForTest(started))
	if err != nil {
		t.Fatal(err)
	}
	if !reached.Load() || saved.PersistenceState != "saved" || saved.ServiceID != "antigravity" || saved.Rule.RuleVersion != 3 || saved.Result == nil || saved.Result.Outcome != "auth_failed" || saved.Result.HTTPStatus == nil || *saved.Result.HTTPStatus != 401 {
		t.Fatalf("request and stored evidence mismatch: %+v", saved)
	}
}

func TestWorkbenchAntigravitySaveRetryKeepsModelEvidence(t *testing.T) {
	var calls atomic.Int32
	app, _ := newPublicServiceApplication(t, func(profile, node string) (monitor.MonitoredNode, error) {
		return monitor.MonitoredNode{NodeKey: node, NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a"}, nil
	}, func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		body := `{"response":{"candidates":[{"content":{"parts":[{"text":"OK"}]}}]}}`
		if req.URL.Path == "/v1internal:loadCodeAssist" {
			body = `{"cloudaicompanionProject":"fixture-project"}`
		}
		if req.URL.Path == "/v1internal:fetchAvailableModels" {
			body = `{"models":{"gemini-fixture-flash":{}}}`
		}
		return publicServiceHTTPResponse(req, 200, "application/json", body), nil
	})
	app.antigravityToken = "fixture-secret"
	app.publicServiceSaveHook = func(context.Context, string) error { return errors.New("fixture save failure") }
	req := publicServiceRequest("antigravity-save", "profile-a", "identity-a", "revision-a", "antigravity")
	started, err := app.StartWorkbenchPublicServiceTest(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	app.publicServiceWG.Wait()
	query := publicServiceQueryForTest(started)
	staged, err := app.GetWorkbenchPublicServiceAttempt(context.Background(), started.AttemptID, query)
	if err != nil || staged.PersistenceState != "failed" {
		t.Fatalf("expected staged failure: %+v %v", staged, err)
	}
	app.publicServiceSaveHook = nil
	saved, err := app.RetrySaveWorkbenchPublicServiceTest(context.Background(), started.AttemptID, query)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Result == nil || saved.Result.Outcome != "matched" || saved.Result.Model != "gemini-fixture-flash" || saved.Result.RequestCount != 3 || saved.PersistenceState != "saved" || saved.AttemptID != started.AttemptID {
		t.Fatalf("missing model evidence: %+v", saved)
	}
	if _, err = app.StartWorkbenchPublicServiceTest(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatalf("retry must not remeasure, calls=%d", calls.Load())
	}
}

func TestWorkbenchPublicServiceSaveRetryReusesAttemptWithoutSecondRequest(t *testing.T) {
	var calls atomic.Int32
	app, store := newPublicServiceApplication(t, func(profile, node string) (monitor.MonitoredNode, error) {
		identity, revision := "identity-a", "revision-a"
		if profile == "profile-b" {
			identity, revision = "identity-b", "revision-b"
		}
		return monitor.MonitoredNode{NodeKey: node, NodeIdentityKey: identity, ConfigRevisionKey: revision, DisplayName: "同名节点", Type: "http", RawConfig: map[string]any{"type": "http"}}, nil
	}, func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		return publicServiceHTTPResponse(request, http.StatusNoContent, "", ""), nil
	})
	defer store.Close()
	app.publicServiceSaveHook = func(context.Context, string) error { return errors.New("injected local write failure") }

	req := publicServiceRequest("request-a", "profile-a", "identity-a", "revision-a", "cloudflare_204")
	started, err := app.StartWorkbenchPublicServiceTest(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	app.publicServiceWG.Wait()
	query := publicServiceQueryForTest(started)
	failedSave, err := app.GetWorkbenchPublicServiceAttempt(context.Background(), started.AttemptID, query)
	if err != nil || failedSave.PersistenceState != "failed" || failedSave.Result == nil || failedSave.Result.Outcome != "matched" {
		t.Fatalf("failed-save result = %+v, err=%v", failedSave, err)
	}

	app.publicServiceSaveHook = nil
	retried, err := app.RetrySaveWorkbenchPublicServiceTest(context.Background(), started.AttemptID, query)
	if err != nil || retried.AttemptID != started.AttemptID || retried.PersistenceState != "saved" {
		t.Fatalf("retry result = %+v, err=%v", retried, err)
	}
	duplicate, err := app.StartWorkbenchPublicServiceTest(context.Background(), req)
	if err != nil || duplicate.AttemptID != started.AttemptID || calls.Load() != 1 {
		t.Fatalf("duplicate request = %+v, calls=%d, err=%v", duplicate, calls.Load(), err)
	}

	other := publicServiceRequest("request-b", "profile-b", "identity-b", "revision-b", "cloudflare_204")
	second, err := app.StartWorkbenchPublicServiceTest(context.Background(), other)
	if err != nil {
		t.Fatal(err)
	}
	app.publicServiceWG.Wait()
	otherQuery := publicServiceQueryForTest(second)
	if _, err := app.GetWorkbenchPublicServiceAttempt(context.Background(), second.AttemptID, otherQuery); err != nil {
		t.Fatal(err)
	}
	if _, err := app.GetWorkbenchPublicServiceAttempt(context.Background(), started.AttemptID, otherQuery); err == nil {
		t.Fatal("same display name or node key exposed a result across profiles")
	}
	if calls.Load() != 2 {
		t.Fatalf("HTTP calls = %d, want one per distinct request", calls.Load())
	}
}

func TestWorkbenchPublicServiceDoesNotSendWhenAttemptIdentityCannotBePersisted(t *testing.T) {
	var calls atomic.Int32
	app, store := newPublicServiceApplication(t, func(profile, node string) (monitor.MonitoredNode, error) {
		return monitor.MonitoredNode{NodeKey: node, NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a", DisplayName: "node", RawConfig: map[string]any{"type": "http"}}, nil
	}, func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		return publicServiceHTTPResponse(request, http.StatusNoContent, "", ""), nil
	})
	defer store.Close()
	app.publicServiceAttemptCreateHook = func(context.Context, *history.PublicServiceAttempt) error {
		return errors.New("injected attempt write failure")
	}
	if _, err := app.StartWorkbenchPublicServiceTest(context.Background(), publicServiceRequest("request-fail-write", "profile-a", "identity-a", "revision-a", "cloudflare_204")); err == nil {
		t.Fatal("attempt persistence failure unexpectedly succeeded")
	}
	if calls.Load() != 0 {
		t.Fatalf("attempt persistence failure sent %d HTTP requests", calls.Load())
	}
}

func TestWorkbenchPublicServiceStagingFailureIsTerminalAndVisible(t *testing.T) {
	var calls atomic.Int32
	app, store := newPublicServiceApplication(t, func(profile, node string) (monitor.MonitoredNode, error) {
		return monitor.MonitoredNode{NodeKey: node, NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a", DisplayName: "node", RawConfig: map[string]any{"type": "http"}}, nil
	}, func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		return publicServiceHTTPResponse(request, http.StatusNoContent, "", ""), nil
	})
	defer store.Close()
	app.publicServiceStageHook = func(context.Context, string, string, history.PublicServiceMeasurement) error {
		return errors.New("injected result staging failure")
	}

	started, err := app.StartWorkbenchPublicServiceTest(context.Background(), publicServiceRequest("request-stage-fail", "profile-a", "identity-a", "revision-a", "cloudflare_204"))
	if err != nil {
		t.Fatal(err)
	}
	app.publicServiceWG.Wait()
	finished, err := app.GetWorkbenchPublicServiceAttempt(context.Background(), started.AttemptID, publicServiceQueryForTest(started))
	if err != nil || finished.ExecutionState != "completed" || finished.PersistenceState != "failed" || finished.Result != nil || !strings.Contains(finished.PersistenceError, "未能暂存") {
		t.Fatalf("staging failure state = %+v, err=%v", finished, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("staging failure request count = %d, want 1", calls.Load())
	}
}

func TestWorkbenchPublicServiceCancelStopsTheActiveRequestAndRejectsStaleRevision(t *testing.T) {
	enteredRequest := make(chan struct{}, 1)
	var calls atomic.Int32
	app, store := newPublicServiceApplication(t, func(profile, node string) (monitor.MonitoredNode, error) {
		return monitor.MonitoredNode{NodeKey: node, NodeIdentityKey: "current-identity", ConfigRevisionKey: "current-revision", DisplayName: "node", RawConfig: map[string]any{"type": "http"}}, nil
	}, func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		enteredRequest <- struct{}{}
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	defer store.Close()

	stale := publicServiceRequest("stale-request", "profile-a", "current-identity", "old-revision", "google_204")
	if _, err := app.StartWorkbenchPublicServiceTest(context.Background(), stale); err == nil {
		t.Fatal("stale revision unexpectedly started")
	}
	if calls.Load() != 0 {
		t.Fatalf("stale revision made %d network requests", calls.Load())
	}

	valid := publicServiceRequest("request-cancel", "profile-a", "current-identity", "current-revision", "google_204")
	started, err := app.StartWorkbenchPublicServiceTest(context.Background(), valid)
	if err != nil {
		t.Fatal(err)
	}
	<-enteredRequest
	concurrent := publicServiceRequest("request-concurrent", "profile-a", "current-identity", "current-revision", "cloudflare_204")
	if _, err := app.StartWorkbenchPublicServiceTest(context.Background(), concurrent); err == nil || !strings.Contains(err.Error(), "正在运行") {
		t.Fatalf("concurrent public-service start err = %v", err)
	}
	query := publicServiceQueryForTest(started)
	if _, err := app.CancelWorkbenchPublicServiceTest(context.Background(), started.AttemptID, query); err != nil {
		t.Fatal(err)
	}
	app.publicServiceWG.Wait()
	finished, err := app.GetWorkbenchPublicServiceAttempt(context.Background(), started.AttemptID, query)
	if err != nil || finished.ExecutionState != "cancelled" || finished.Result == nil || finished.Result.Outcome != "cancelled" || finished.PersistenceState != "saved" {
		t.Fatalf("cancelled attempt = %+v, err=%v", finished, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("HTTP request count = %d, want 1", calls.Load())
	}
}

func publicServiceQueryForTest(attempt *history.PublicServiceAttempt) WorkbenchPublicServiceHistoryQuery {
	return WorkbenchPublicServiceHistoryQuery{
		ProfileID: attempt.ProfileID, NodeKey: attempt.NodeKey, NodeIdentityKey: attempt.NodeIdentityKey,
		ConfigRevisionKey: attempt.ConfigRevisionKey, ServiceID: attempt.ServiceID,
	}
}
