package application

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDownloadWorkerReportsPanicAndCanRunAgain(t *testing.T) {
	app, _, _ := newWorkbenchDownloadApplication(t)
	configureDownloadResponse(app, func(*http.Request) (*http.Response, error) { panic("fixture") })
	first, err := app.StartWorkbenchDownloadTest(context.Background(), downloadRequest("panic"))
	if err != nil {
		t.Fatal(err)
	}
	app.workbenchWG.Wait()
	failed, err := app.GetWorkbenchDownloadAttempt(context.Background(), first.AttemptID, downloadQuery())
	if err != nil || failed.ExecutionState != "failed" || failed.Result != nil {
		t.Fatalf("panic outcome=%+v err=%v", failed, err)
	}
	configureDownloadResponse(app, func(r *http.Request) (*http.Response, error) {
		return downloadResponse(r, io.NopCloser(strings.NewReader(strings.Repeat("d", 1400)))), nil
	})
	next, err := app.StartWorkbenchDownloadTest(context.Background(), downloadRequest("after-panic"))
	if err != nil {
		t.Fatal(err)
	}
	app.workbenchWG.Wait()
	saved, err := app.GetWorkbenchDownloadAttempt(context.Background(), next.AttemptID, downloadQuery())
	if err != nil || saved.Result == nil {
		t.Fatalf("next outcome=%+v err=%v", saved, err)
	}
}
