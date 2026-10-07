package publicservice

import (
	"context"
	"github.com/faceair/clash-speedtest/core/monitor"
	"net/http"
	"testing"
	"time"
)

func TestExpiredBudgetNeverStartsNetworkProbe(t *testing.T) {
	for _, phase := range []string{"before_prepare", "during_prepare", "cancelled_before_prepare"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			if phase == "before_prepare" {
				<-ctx.Done()
			}
			if phase == "cancelled_before_prepare" {
				cancel()
			}
			factoryCalls, networkCalls := 0, 0
			checker := Checker{ClientFactory: func(monitor.MonitoredNode, time.Duration) (*http.Client, error) {
				factoryCalls++
				if phase == "during_prepare" {
					time.Sleep(30 * time.Millisecond)
				}
				return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					networkCalls++
					return &http.Response{StatusCode: http.StatusNoContent, Header: make(http.Header), Body: http.NoBody, Request: r}, nil
				})}, nil
			}}
			rule, _ := RuleFor("google_204")
			r := checker.Check(ctx, monitor.MonitoredNode{}, rule, time.Second)
			if r.Outcome != "not_executed" || r.RequestCount != 0 || networkCalls != 0 || r.FailurePhase != "preparation" || r.Details["execution_status"] != "not_executed" {
				t.Fatalf("expired preparation became network result: %+v, calls=%d", r, networkCalls)
			}
			if phase != "during_prepare" && factoryCalls != 0 {
				t.Fatal("already exhausted budget entered preparation")
			}
		})
	}
}
func TestActualNetworkDeadlineRemainsExecutedTimeout(t *testing.T) {
	rule, _ := RuleFor("google_204")
	checker := Checker{ClientFactory: fixtureClient(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })}
	r := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, 20*time.Millisecond)
	if r.Outcome != "timed_out" || r.RequestCount != 1 || r.Details["execution_status"] != "executed" {
		t.Fatalf("real network timeout became unexecuted: %+v", r)
	}
}
