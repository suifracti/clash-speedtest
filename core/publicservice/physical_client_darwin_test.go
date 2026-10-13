//go:build darwin

package publicservice

import (
	"context"
	"errors"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/speedtester"
	"net/http"
	"testing"
	"time"
)

func TestDefaultMacServiceMissingPhysicalConfigIsUnexecuted(t *testing.T) {
	rule, _ := RuleFor("cloudflare_204")
	got := (Checker{}).Check(context.Background(), monitor.MonitoredNode{NodeKey: "fixture-no-config"}, rule, time.Second)
	if got.RequestCount != 0 || got.Outcome != "not_executed" || got.FailurePhase != "physical_preflight" {
		t.Fatalf("local path absence must not become a node fault or enter HTTP: %+v", got)
	}
	if got.Details["network_path"] == "" || got.Details["egress_verification"] == "verified" {
		t.Fatalf("bounded preparation evidence missing or overstated: %+v", got.Details)
	}
}

func TestServiceLocalBindingErrorIsUnexecutedForSingleAndSpecializedRules(t *testing.T) {
	for _, id := range []string{"cloudflare_204", "cloudflare_trace"} {
		t.Run(id, func(t *testing.T) {
			rule, _ := RuleFor(id)
			checker := Checker{ClientFactory: fixtureClient(func(*http.Request) (*http.Response, error) {
				return nil, &speedtester.PhysicalPathError{Cause: errors.New("fixture local binding blocked")}
			})}
			got := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, time.Second)
			if got.Outcome != "not_executed" || got.Details["execution_status"] != "not_executed" || got.Details["http_requests_written"] != "0" || got.RequestCount != 1 {
				t.Fatalf("local error became business/node failure or lost attempt count: %+v", got)
			}
		})
	}
}
