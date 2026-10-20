package application

import (
	"context"
	"github.com/faceair/clash-speedtest/core/monitor"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPeriodicCancellationBeforeProbeHasStage(t *testing.T) {
	for _, phase := range []string{"admission", "preparation"} {
		t.Run(phase, func(t *testing.T) {
			var calls atomic.Int32
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			app, _ := newPublicServiceApplication(t, func(_ string, node string) (monitor.MonitoredNode, error) {
				if phase == "preparation" {
					cancel()
				}
				return monitor.MonitoredNode{NodeKey: node, NodeIdentityKey: "identity-a", ConfigRevisionKey: "revision-a"}, nil
			}, func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				return publicServiceHTTPResponse(r, 204, "text/plain", ""), nil
			})
			if phase == "admission" {
				cancel()
			}
			a, err := app.startWorkbenchPublicServiceTest(ctx, publicServiceRequest("preprobe-budget", "profile-a", "identity-a", "revision-a", "google_204"), 16)
			app.publicServiceWG.Wait()
			if err == nil || a != nil || !strings.Contains(err.Error(), phase) || calls.Load() != 0 {
				t.Fatalf("preprobe budget lacked phase/not-executed: attempt=%v err=%v calls=%d", a != nil, err, calls.Load())
			}
		})
	}
}
