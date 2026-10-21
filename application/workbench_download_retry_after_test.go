package application

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
)

func runDownloadSources(t *testing.T, statuses ...int) (*history.WorkbenchDownloadAttempt, []string, *history.Store) {
	t.Helper()
	app, store, _ := newWorkbenchDownloadApplication(t)
	t.Cleanup(func() { _ = store.Close() })
	var targets []string
	configureDownloadResponse(app, func(r *http.Request) (*http.Response, error) {
		targets = append(targets, r.URL.String())
		status, body := statuses[len(targets)-1], "payload"
		if status != http.StatusOK {
			body = ""
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	started, err := app.StartWorkbenchDownloadTest(context.Background(), downloadRequest("source-retry"))
	if err != nil {
		t.Fatal(err)
	}
	app.workbenchWG.Wait()
	got, err := app.GetWorkbenchDownloadAttempt(context.Background(), started.AttemptID, downloadQuery())
	if err != nil {
		t.Fatal(err)
	}
	return got, targets, store
}

func TestWorkbenchDownloadSwitchesAfter429AndComparesActualTarget(t *testing.T) {
	got, targets, store := runDownloadSources(t, http.StatusTooManyRequests, http.StatusOK)
	if len(targets) != 2 || targets[0] != "https://speed.cloudflare.com/__down?bytes=1001" || targets[1] != "https://cachefly.cachefly.net/100mb.test" || got.Result.TargetURL != targets[1] || got.Result.Outcome != "completed" {
		t.Fatalf("fallback result mismatch: requests=%v result=%+v", targets, got.Result)
	}
	next := *got
	next.AttemptID, next.RequestedAt = "next", got.RequestedAt.Add(time.Minute)
	next.FinishedAt = &next.RequestedAt
	next.Result = &history.WorkbenchDownloadMeasurement{TargetURL: "https://speed.cloudflare.com/__down?bytes=1001"}
	previous, err := store.DB().PreviousWorkbenchDownloadAttempt(context.Background(), &next)
	if err != nil || previous != nil {
		t.Fatalf("different actual target matched prior attempt: %+v %v", previous, err)
	}
}

func TestWorkbenchDownloadReportsBothSourcesLimited(t *testing.T) {
	got, targets, _ := runDownloadSources(t, http.StatusForbidden, http.StatusTooManyRequests)
	if len(targets) != 2 || got.Result.TargetURL != targets[1] || got.Result.Outcome != "source_rate_limited" || got.Result.ErrorMessage != "测速源都限流" {
		t.Fatalf("both-source limit mismatch: requests=%v result=%+v", targets, got.Result)
	}
}
