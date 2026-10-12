package application

import (
	"context"
	"testing"
	"time"
)

func TestWorkbenchDownloadLifecycleContextIsIndependentAndBounded(t *testing.T) {
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	ctx, cancelTask := newWorkbenchDownloadLifecycleContext(requestCtx, 250*time.Millisecond)
	defer cancelTask()
	defer cancelRequest()
	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("download lifecycle must have an explicit deadline without an HTTP deadline")
	}
	cancelRequest()
	if err := ctx.Err(); err != nil {
		t.Fatalf("short-lived start request must not cancel the download task: %v", err)
	}
	select {
	case <-ctx.Done():
		t.Fatal("download lifecycle deadline expired too early")
	default:
	}
}
