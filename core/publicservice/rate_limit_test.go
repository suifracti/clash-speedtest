package publicservice

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
)

func TestRateLimitStopsRemainingSamples(t *testing.T) {
	for _, tc := range []struct {
		name, service   string
		passFirst       bool
		status          int
		githubRemaining string
	}{
		{name: "initial 429", service: "cloudflare_204", status: 429},
		{name: "pass then 429", service: "cloudflare_204", passFirst: true, status: 429},
		{name: "GitHub quota zero", service: "github_api_root", status: 403, githubRemaining: "0"},
		{name: "specialized page", service: "prime_video", status: 429},
		{name: "bounded JSON", service: "cloudflare_doh", status: 429},
		{name: "Netflix first response", service: "netflix_unlock", status: 429},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := Checker{DriftSamples: 4, ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
				calls++
				status := tc.status
				if tc.passFirst && calls == 1 {
					status = 204
				}
				r := fixtureResponse(req, status, "text/plain", "")
				if status == tc.status {
					r.Header.Set("Retry-After", "120")
					if tc.githubRemaining != "" {
						r.Header.Set("X-RateLimit-Remaining", tc.githubRemaining)
						r.Header.Set("X-RateLimit-Reset", "1893456000")
					}
				}
				return r, nil
			})}
			rule, _ := RuleFor(tc.service)
			r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, time.Second)
			wantCalls := 1
			wantSuccess := "0"
			wantUnstarted := "3"
			if tc.passFirst {
				wantCalls = 2
				wantSuccess = "1"
				wantUnstarted = "2"
			}
			if calls != wantCalls || r.RequestCount != wantCalls || r.Outcome != "rate_limited" || r.HTTPStatus == nil || *r.HTTPStatus != tc.status {
				t.Fatalf("calls=%d want=%d result=%+v", calls, wantCalls, r)
			}
			d := r.Details
			if d["sample_success"] != wantSuccess || d["sample_failure"] != "0" || d["sample_unknown"] != "1" || d["sample_unstarted"] != wantUnstarted || d["sampling_stop_reason"] != "rate_limited" || d["retry_after_seconds"] != "120" {
				t.Fatalf("wrong partial evidence: %v", d)
			}
			if tc.githubRemaining != "" && d["rate_limit_reset_at"] != "2030-01-01T00:00:00Z" {
				t.Fatalf("missing reset time: %v", d)
			}
		})
	}
}

func TestRateLimitHintIsNormalizedNotCopied(t *testing.T) {
	for _, tc := range []struct{ name, header, seconds, at string }{
		{"seconds", "60", "60", ""},
		{"HTTP date", "Tue, 01 Jan 2030 00:00:00 GMT", "", "2030-01-01T00:00:00Z"},
		{"arbitrary header omitted", "Bearer fixture-secret?token=fixture-secret", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule, _ := RuleFor("cloudflare_204")
			c := Checker{ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
				r := fixtureResponse(req, 429, "", "")
				r.Header.Set("Retry-After", tc.header)
				return r, nil
			})}
			r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, time.Second)
			if r.Details["retry_after_seconds"] != tc.seconds || r.Details["retry_after_at"] != tc.at {
				t.Fatalf("details=%v", r.Details)
			}
		})
	}
}
