package publicservice

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
)

func TestMultiSampleSeparatesUnconfirmedFromFailure(t *testing.T) {
	rule, _ := RuleFor("cloudflare_204")
	for _, tc := range []struct {
		name                            string
		statuses                        []int
		want, success, failure, unknown string
	}{
		{"pass then server error", []int{204, 500, 500}, "unknown", "1", "0", "2"},
		{"all server errors", []int{500, 500, 500}, "unknown", "0", "0", "3"},
		{"confirmed rejection remains failure", []int{403, 403, 403}, "http_rejected", "0", "3", "0"},
		{"pass and confirmed rejection", []int{204, 403, 403}, "flapping", "1", "2", "0"},
		{"rejection with unconfirmed samples", []int{403, 500, 500}, "unknown", "0", "1", "2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := Checker{DriftSamples: 3, ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
				status := tc.statuses[calls]
				calls++
				return fixtureResponse(req, status, "", ""), nil
			})}
			r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, time.Second)
			if r.Outcome != tc.want || r.Details["sample_success"] != tc.success || r.Details["sample_failure"] != tc.failure || r.Details["sample_unknown"] != tc.unknown {
				t.Fatalf("outcome=%s details=%v", r.Outcome, r.Details)
			}
			if r.Details["sample_semantics_version"] != "2" || r.RequestCount != 3 {
				t.Fatalf("result=%+v", r)
			}
			if tc.success == "0" && tc.failure == "0" && r.Details["sample_success_rate"] != "未知" {
				t.Fatalf("unknown samples received numeric rate: %v", r.Details)
			}
		})
	}
}

func TestMultiSampleCancellationStopsRemainingSamples(t *testing.T) {
	rule, _ := RuleFor("cloudflare_204")
	calls := 0
	c := Checker{DriftSamples: 4, ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return fixtureResponse(req, 204, "", ""), nil
		}
		return nil, context.Canceled
	})}
	r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, time.Second)
	if r.Outcome != "cancelled" || calls != 2 || r.Details["sample_success"] != "1" || r.Details["sample_failure"] != "0" || r.Details["sample_cancelled"] != "1" || r.Details["sample_unstarted"] != "2" || r.RequestCount != 2 {
		t.Fatalf("calls=%d result=%+v", calls, r)
	}
}

func TestMultiSampleUnfinishedPlanDoesNotClaimFullPass(t *testing.T) {
	rule, _ := RuleFor("cloudflare_204")
	calls := 0
	c := Checker{DriftSamples: 3, ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
		calls++
		return fixtureResponse(req, 204, "", ""), nil
	})}
	// The shared budget cannot admit a second sample. Do not wait or send a probe.
	r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, 300*time.Millisecond)
	if r.Outcome != "unknown" || calls != 1 || r.Details["sample_unstarted"] != "2" || r.Details["sample_completion"] != "partial" || strings.Contains(r.Summary, "一致通过") {
		t.Fatalf("calls=%d result=%+v", calls, r)
	}
}

func TestMultiSampleExitClaimsRequireIPObservations(t *testing.T) {
	for _, tc := range []struct {
		name, service string
		ips, colos    []string
		status        string
	}{
		{"no exit observation", "cloudflare_204", nil, nil, "unobserved"},
		{"colo changes alone are not IP drift", "cloudflare_204", nil, []string{"SIN", "HKG", "SIN"}, "unobserved"},
		{"same observed IP", "cloudflare_trace", []string{"198.51.100.1", "198.51.100.1", "198.51.100.1"}, nil, "same_ip_observed"},
		{"different observed IPs", "cloudflare_trace", []string{"198.51.100.1", "198.51.100.2", "198.51.100.2"}, nil, "ip_drift_observed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule, _ := RuleFor(tc.service)
			calls := 0
			c := Checker{DriftSamples: 3, ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
				i := calls
				calls++
				resp := fixtureResponse(req, 204, "", "")
				if tc.ips != nil {
					resp = fixtureResponse(req, 200, "text/plain", "ip="+tc.ips[i]+"\nloc=SG\ncolo=SIN\n")
				}
				if tc.colos != nil {
					resp.Header.Set("CF-Ray", "fixture-"+tc.colos[i])
				}
				return resp, nil
			})}
			r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, time.Second)
			if r.Details["exit_observation_status"] != tc.status {
				t.Fatalf("result=%+v", r)
			}
			if tc.ips == nil && (r.Details["ip_drift_detected"] != "unknown" || strings.Contains(r.Details["drift_summary"], "出口稳定")) {
				t.Fatalf("unobserved exit claimed stable: %+v", r)
			}
		})
	}
}

func TestMultiSampleTransportFailureStaysUnconfirmed(t *testing.T) {
	rule, _ := RuleFor("cloudflare_204")
	c := Checker{DriftSamples: 3, ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) { return nil, io.EOF })}
	r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, time.Second)
	if r.Outcome != "unknown" || r.Details["sample_unknown"] != "3" || r.Details["sample_failure"] != "0" {
		t.Fatalf("result=%+v", r)
	}
}

// A timing wrapper must not swallow the HTTP client's reconnect operation.
type multiSampleCloseTransport struct{ closed int }

func (t *multiSampleCloseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return fixtureResponse(req, 204, "", ""), nil
}
func (t *multiSampleCloseTransport) CloseIdleConnections() { t.closed++ }
func TestMultiSampleReconnectReachesTransport(t *testing.T) {
	transport := &multiSampleCloseTransport{}
	rule, _ := RuleFor("cloudflare_204")
	c := Checker{DriftSamples: 3, ClientFactory: func(monitor.MonitoredNode, time.Duration) (*http.Client, error) {
		return &http.Client{Transport: transport}, nil
	}}
	r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, time.Second)
	if r.Outcome != "matched" || transport.closed != 1 {
		t.Fatalf("close calls=%d outcome=%s", transport.closed, r.Outcome)
	}
}
