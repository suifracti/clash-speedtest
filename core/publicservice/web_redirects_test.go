package publicservice

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
)

func TestWebServiceFollowsCanonicalRedirects(t *testing.T) {
	for _, tc := range []struct{ service, destination string }{
		{"viu_web", "https://www.viu.com/ott/sg"},
		{"unext_jp", "https://video.unext.jp/"},
		{"gitlab_web", "https://about.gitlab.com/"},
		{"nintendo_jp", "https://store-jp-wr.nintendo.com/"},
		{"nhk_plus_jp", "https://www.web.nhk/"},
	} {
		t.Run(tc.service, func(t *testing.T) {
			rule, _ := RuleFor(tc.service)
			calls := 0
			checker := Checker{ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					r := fixtureResponse(req, 302, "text/html", "")
					r.Header.Set("Location", tc.destination)
					return r, nil
				}
				if req.URL.String() != tc.destination {
					t.Fatalf("unexpected target %s", req.URL)
				}
				return fixtureResponse(req, 200, "text/html", "<html>public homepage</html>"), nil
			})}
			r := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
			if r.Outcome != "reachable" || calls != 2 || r.Details["redirect_hops"] != "1" || r.Details["final_url"] != tc.destination {
				t.Fatalf("canonical redirect not checked: calls=%d result=%+v", calls, r)
			}
		})
	}
}

func TestWebRedirectStopsOutsideServiceAndAtLimit(t *testing.T) {
	for _, tc := range []struct {
		name, destination, reason string
		calls                     int
	}{
		{"outside service", "https://unrelated.invalid/", "outside_service", 1},
		{"HTTP downgrade", "http://www.viu.com/", "unsupported_redirect", 1},
		{"redirect loop", "https://www.viu.com/loop", "redirect_limit", 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule, _ := RuleFor("viu_web")
			calls := 0
			checker := Checker{ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
				calls++
				r := fixtureResponse(req, 302, "text/html", "")
				r.Header.Set("Location", tc.destination)
				return r, nil
			})}
			r := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
			if r.Outcome != "redirect" || calls != tc.calls || r.Details["redirect_stop_reason"] != tc.reason {
				t.Fatalf("redirect stop incorrect: calls=%d result=%+v", calls, r)
			}
		})
	}
}

func TestWebRedirectChallengeDoesNotBecomeAvailability(t *testing.T) {
	rule, _ := RuleFor("viu_web")
	calls := 0
	checker := Checker{ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			r := fixtureResponse(req, 302, "text/html", "")
			r.Header.Set("Location", "/ott/sg")
			return r, nil
		}
		return fixtureResponse(req, 200, "text/html", `<title>Just a moment...</title><script src="/cdn-cgi/challenge-platform/x"></script>`), nil
	})}
	r := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	if r.Outcome != "challenge" || calls != 2 {
		t.Fatalf("challenge after redirect = %+v, calls=%d", r, calls)
	}
}

func TestRetiredKKTVDoesNotProbeNode(t *testing.T) {
	rule, _ := RuleFor("kktv_tw")
	calls := 0
	checker := Checker{DriftSamples: 4, ClientFactory: func(monitor.MonitoredNode, time.Duration) (*http.Client, error) {
		calls++
		return nil, nil
	}}
	r := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	if calls != 0 || r.RequestCount != 0 || r.Outcome != "not_executed" || r.FailurePhase != "service_retired" {
		t.Fatalf("retired service became node result: calls=%d result=%+v", calls, r)
	}
}
