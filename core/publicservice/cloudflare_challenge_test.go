package publicservice

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/faceair/clash-speedtest/core/monitor"
)

func TestCloudflareChallengeProbeClassifiesCleanPartialAndBlocked(t *testing.T) {
	rule, ok := RuleFor("cloudflare_challenge")
	if !ok {
		t.Fatal("expected cloudflare_challenge in RuleFor")
	}

	t.Run("all clean passes as profiled", func(t *testing.T) {
		checker := Checker{DriftSamples: 1, ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
			resp := fixtureResponse(req, 200, "text/html", "<html><title>Home</title></html>")
			resp.Header.Set("CF-Ray", "abc123-NRT")
			return resp, nil
		})}
		res := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
		if res.Outcome != "profiled" || res.Details["cf_challenged_count"] != "0" || res.Details["cf_passed_count"] != "3" || res.Details["cf_colo"] != "NRT" {
			t.Fatalf("unexpected clean result: %+v", res)
		}
	})

	t.Run("strict site only triggers verification_required", func(t *testing.T) {
		checker := Checker{DriftSamples: 1, ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "claude.ai" {
				resp := fixtureResponse(req, 403, "text/html", `<html><title>Just a moment...</title><script src="/cdn-cgi/challenge-platform/x"></script></html>`)
				resp.Header.Set("cf-mitigated", "challenge")
				return resp, nil
			}
			return fixtureResponse(req, 200, "text/html", "<html>ok</html>"), nil
		})}
		res := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
		if res.Outcome != "verification_required" || res.Details["cf_challenged_count"] != "1" || res.Details["cf_passed_count"] != "2" || !strings.Contains(res.Details["cf_challenge_detail"], "Claude: 弹盾") {
			t.Fatalf("unexpected partial challenge result: %+v", res)
		}
	})

	t.Run("majority challenged via turnstile HTML triggers challenge", func(t *testing.T) {
		checker := Checker{DriftSamples: 1, ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "grok.com" {
				return fixtureResponse(req, 200, "text/html", "<html>ok</html>"), nil
			}
			return fixtureResponse(req, 403, "text/html", `<html><div class="cf-turnstile"></div></html>`), nil
		})}
		res := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
		if res.Outcome != "challenge" || res.Details["cf_challenged_count"] != "2" || !strings.Contains(res.Details["challenge_evidence"], "turnstile") {
			t.Fatalf("unexpected majority challenge result: %+v", res)
		}
	})

	t.Run("all transport errors preserve transport_error", func(t *testing.T) {
		checker := Checker{DriftSamples: 1, ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("dial tcp: connection refused")
		})}
		res := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
		if res.Outcome != "transport_error" || res.Details["cf_responded_count"] != "0" {
			t.Fatalf("unexpected transport failure result: %+v", res)
		}
	})
}
