package application

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/speedtester"
)

func waitWorkbenchRecoverySignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func TestWorkbenchPublicServiceRetrySaveWaitsForClose(t *testing.T) {
	var calls atomic.Int32
	app, store := newPublicServiceApplication(t, func(_, node string) (monitor.MonitoredNode, error) {
		return monitor.MonitoredNode{NodeKey: node, NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a"}, nil
	}, func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		return publicServiceHTTPResponse(request, http.StatusNoContent, "", ""), nil
	})
	app.publicServiceSaveHook = func(context.Context, string) error { return errors.New("fixture save failure") }
	started, err := app.StartWorkbenchPublicServiceTest(context.Background(), publicServiceRequest("save-before-close", "profile-a", "identity-a", "revision-a", "cloudflare_204"))
	if err != nil {
		t.Fatal(err)
	}
	app.publicServiceWG.Wait()
	query := publicServiceQueryForTest(started)

	saveEntered, releaseSave := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-releaseSave:
		default:
			close(releaseSave)
		}
	}()
	app.publicServiceSaveHook = func(context.Context, string) error {
		close(saveEntered)
		<-releaseSave
		return nil
	}
	type retryResult struct {
		attempt *history.PublicServiceAttempt
		err     error
		panic   any
	}
	retryDone := make(chan retryResult, 1)
	go func() {
		var result retryResult
		defer func() {
			result.panic = recover()
			retryDone <- result
		}()
		result.attempt, result.err = app.RetrySaveWorkbenchPublicServiceTest(context.Background(), started.AttemptID, query)
	}()
	waitWorkbenchRecoverySignal(t, saveEntered, "retry save")
	closeReached := make(chan struct{})
	app.emitter.(*MemoryEventEmitter).Subscribe(func(event Event) {
		if event.Type == "test_stopped" {
			close(closeReached)
		}
	})
	closeDone := make(chan error, 1)
	go func() { closeDone <- app.Close() }()
	waitWorkbenchRecoverySignal(t, closeReached, "application close")
	closedTooSoon := false
	var closeErr error
	select {
	case closeErr = <-closeDone:
		closedTooSoon = true
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseSave)
	var retried retryResult
	select {
	case retried = <-retryDone:
	case <-time.After(time.Second):
		t.Fatal("retry save did not finish after release")
	}
	if !closedTooSoon {
		select {
		case closeErr = <-closeDone:
		case <-time.After(time.Second):
			t.Fatal("close did not finish after retry save")
		}
	}
	if closedTooSoon || retried.panic != nil || retried.err != nil || closeErr != nil {
		t.Fatalf("close must wait for retry save: closed_early=%v panic=%v retry_error=%v close_error=%v", closedTooSoon, retried.panic, retried.err, closeErr)
	}
	if retried.attempt == nil || retried.attempt.PersistenceState != "saved" {
		t.Fatalf("retry did not save the original attempt: %+v", retried.attempt)
	}
	reopened, err := history.NewStore(store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	saved, err := reopened.GetPublicServiceAttemptByRequestID(context.Background(), started.RequestID)
	if err != nil || saved == nil || saved.PersistenceState != "saved" || saved.AttemptID != started.AttemptID || calls.Load() != 1 {
		t.Fatalf("saved result must survive close without another request: attempt=%+v calls=%d err=%v", saved, calls.Load(), err)
	}
}

func TestWorkbenchDownloadConcurrentRequestReusesCompletedAttempt(t *testing.T) {
	app, _, _ := newWorkbenchDownloadApplication(t)
	var requests, resolutions atomic.Int32
	configureDownloadResponse(app, func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		return downloadResponse(request, io.NopCloser(strings.NewReader(strings.Repeat("d", 1400)))), nil
	})
	resolveEntered, releaseResolve := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-releaseResolve:
		default:
			close(releaseResolve)
		}
	}()
	resolve := app.workbenchDownloadResolveHook
	app.workbenchDownloadResolveHook = func(profile, node string) (monitor.MonitoredNode, *speedtester.CProxy, error) {
		if resolutions.Add(1) == 1 {
			close(resolveEntered)
			<-releaseResolve
		}
		return resolve(profile, node)
	}
	type startResult struct {
		attempt *history.WorkbenchDownloadAttempt
		err     error
	}
	delayedDone := make(chan startResult, 1)
	req := downloadRequest("concurrent-download")
	go func() {
		attempt, err := app.StartWorkbenchDownloadTest(context.Background(), req)
		delayedDone <- startResult{attempt: attempt, err: err}
	}()
	waitWorkbenchRecoverySignal(t, resolveEntered, "delayed download resolution")
	started, err := app.StartWorkbenchDownloadTest(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	app.workbenchWG.Wait()
	close(releaseResolve)
	select {
	case delayed := <-delayedDone:
		if delayed.err != nil || delayed.attempt == nil || delayed.attempt.AttemptID != started.AttemptID {
			t.Fatalf("same request must reuse the completed attempt: first=%+v delayed=%+v err=%v", started, delayed.attempt, delayed.err)
		}
	case <-time.After(time.Second):
		t.Fatal("delayed download start did not finish")
	}
	page, err := app.ListWorkbenchDownloadTests(context.Background(), downloadQuery())
	if err != nil || len(page.Attempts) != 1 || requests.Load() != 1 {
		t.Fatalf("one request ID must produce one attempt and one request: history=%+v requests=%d err=%v", page, requests.Load(), err)
	}
}

func rejectWorkbenchAttemptBegin(t *testing.T, store *history.Store, table string) func() {
	t.Helper()
	raw, err := sql.Open("sqlite", filepath.Join(store.Dir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	statement := fmt.Sprintf(`CREATE TRIGGER reject_fixture_begin BEFORE UPDATE OF execution_state ON %s WHEN NEW.execution_state='running' BEGIN SELECT RAISE(FAIL, 'fixture begin failure'); END`, table)
	if _, err := raw.Exec(statement); err != nil {
		t.Fatal(err)
	}
	return func() {
		t.Helper()
		if _, err := raw.Exec(`DROP TRIGGER reject_fixture_begin`); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWorkbenchAttemptBeginFailureIsTerminal(t *testing.T) {
	t.Run("download", func(t *testing.T) {
		app, store, _ := newWorkbenchDownloadApplication(t)
		var calls atomic.Int32
		configureDownloadResponse(app, func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			return downloadResponse(request, io.NopCloser(strings.NewReader("fixture"))), nil
		})
		restoreBegin := rejectWorkbenchAttemptBegin(t, store, "workbench_download_attempts")
		req := downloadRequest("download-begin-failed")
		if _, err := app.StartWorkbenchDownloadTest(context.Background(), req); err == nil {
			t.Fatal("injected begin failure unexpectedly started a download")
		}
		restoreBegin()
		attempt, err := app.StartWorkbenchDownloadTest(context.Background(), req)
		if err != nil || attempt == nil || attempt.ExecutionState != "interrupted" || attempt.PersistenceState != "not_applicable" || calls.Load() != 0 {
			t.Fatalf("failed start must return a terminal attempt on replay, without traffic: attempt=%+v calls=%d err=%v", attempt, calls.Load(), err)
		}
		if _, err := app.StartWorkbenchDownloadTest(context.Background(), downloadRequest("download-after-begin-failure")); err != nil {
			t.Fatal(err)
		}
		app.workbenchWG.Wait()
		if calls.Load() != 1 {
			t.Fatalf("new request must remain executable after begin failure: calls=%d", calls.Load())
		}
	})
	t.Run("public_service", func(t *testing.T) {
		var calls atomic.Int32
		app, store := newPublicServiceApplication(t, func(_, node string) (monitor.MonitoredNode, error) {
			return monitor.MonitoredNode{NodeKey: node, NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a"}, nil
		}, func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			return publicServiceHTTPResponse(request, http.StatusNoContent, "", ""), nil
		})
		restoreBegin := rejectWorkbenchAttemptBegin(t, store, "workbench_public_service_attempts")
		req := publicServiceRequest("public-begin-failed", "profile-a", "identity-a", "revision-a", "cloudflare_204")
		if _, err := app.StartWorkbenchPublicServiceTest(context.Background(), req); err == nil {
			t.Fatal("injected begin failure unexpectedly started a public-service test")
		}
		restoreBegin()
		attempt, err := app.StartWorkbenchPublicServiceTest(context.Background(), req)
		if err != nil || attempt == nil || attempt.ExecutionState != "interrupted" || attempt.PersistenceState != "not_applicable" || calls.Load() != 0 {
			t.Fatalf("failed start must return a terminal attempt on replay, without traffic: attempt=%+v calls=%d err=%v", attempt, calls.Load(), err)
		}
		if _, err := app.StartWorkbenchPublicServiceTest(context.Background(), publicServiceRequest("public-after-begin-failure", "profile-a", "identity-a", "revision-a", "cloudflare_204")); err != nil {
			t.Fatal(err)
		}
		app.publicServiceWG.Wait()
		if calls.Load() != 1 {
			t.Fatalf("new request must remain executable after begin failure: calls=%d", calls.Load())
		}
	})
}
