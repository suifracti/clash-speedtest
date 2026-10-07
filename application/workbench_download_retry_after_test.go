package application

import (
	"context"
	"github.com/faceair/clash-speedtest/core/speedtester"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadRespectsPersistedRetryAfterWithoutAnotherNetworkRequest(t *testing.T) {
	for _, header := range []string{"3100", ""} {
		t.Run("header-"+header, func(t *testing.T) {
			app, _, _ := newWorkbenchDownloadApplication(t)
			var calls atomic.Int32
			app.workbenchDownloadClientFactory = func(*speedtester.SpeedTester, *speedtester.CProxy, time.Duration) (*http.Client, error) {
				return &http.Client{Transport: workbenchDownloadRT(func(req *http.Request) (*http.Response, error) {
					calls.Add(1)
					return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{header}}, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
				})}, nil
			}
			req := WorkbenchDownloadTestRequest{RequestID: "rate-first", ProfileID: "profile-a", NodeKey: "node-a", NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a", MaximumBytes: 10485760, TimeoutSeconds: 10}
			if _, err := app.StartWorkbenchDownloadTest(context.Background(), req); err != nil {
				t.Fatal(err)
			}
			app.workbenchWG.Wait()
			req.RequestID = "rate-second"
			req.MaximumBytes = 1024
			if _, err := app.StartWorkbenchDownloadTest(context.Background(), req); err == nil || !strings.Contains(err.Error(), "测速源要求等待") {
				t.Fatalf("cooldown not applied: %v", err)
			}
			if calls.Load() != 1 {
				t.Fatalf("sent %d requests during Retry-After", calls.Load())
			}
		})
	}

}

func TestExcessiveSourceCooldownDoesNotCreateUnboundedAutomaticWait(t *testing.T) {
	app, _, _ := newWorkbenchDownloadApplication(t)
	var calls atomic.Int32
	app.workbenchDownloadClientFactory = func(*speedtester.SpeedTester, *speedtester.CProxy, time.Duration) (*http.Client, error) {
		return &http.Client{Transport: workbenchDownloadRT(func(r *http.Request) (*http.Response, error) {
			calls.Add(1)
			return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"7200"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		})}, nil
	}
	req := WorkbenchDownloadTestRequest{RequestID: "long-first", ProfileID: "profile-a", NodeKey: "node-a", NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a", MaximumBytes: 10485760, TimeoutSeconds: 10}
	if _, err := app.StartWorkbenchDownloadTest(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	app.workbenchWG.Wait()
	req.RequestID = "long-second"
	_, err := app.StartWorkbenchDownloadTest(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "超过 3600 秒冷却预算") || calls.Load() != 1 {
		t.Fatalf("unbounded wait or extra request: %v count=%d", err, calls.Load())
	}
}
