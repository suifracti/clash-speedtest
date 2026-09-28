package publicservice

import (
	"context"
	"github.com/faceair/clash-speedtest/core/monitor"
	"net/http"
	"testing"
)

func TestServiceEvidenceDoesNotConfuseAccessWithAvailability(t *testing.T) {
	for _, tc := range []struct {
		name, service string
		status        int
		body, outcome string
	}{
		{"prime false is not restricted", "prime_video", 200, `{"currentTerritory":"JP","isServiceRestricted":false}`, "unlocked"},
		{"prime true is restricted", "prime_video", 200, `{"currentTerritory":"JP","isServiceRestricted":true}`, "region_blocked"},
		{"prime territory alone is insufficient", "prime_video", 200, `{"currentTerritory":"JP"}`, "unknown"},
		{"search server error", "google_search", 503, `maintenance`, "http_rejected"},
		{"hulu forbidden is not proven geoblocking", "hulu_jp_unlock", 403, `forbidden`, "http_rejected"},
		{"hulu homepage is not playback", "hulu_jp_unlock", 200, `<html>Welcome</html>`, "reachable"},
		{"netflix login is not a title", "netflix_unlock", 200, `<html>Sign in</html>`, "unknown"},
		{"empty DNS document", "cloudflare_doh", 200, `{}`, "criteria_mismatch"},
		{"soft challenge is not access", "grok_web", 200, `<title>Just a moment...</title><script src="/cdn-cgi/challenge-platform/x"></script>`, "challenge"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule, ok := RuleFor(tc.service)
			if !ok {
				t.Fatalf("missing rule %s", tc.service)
			}
			checker := Checker{ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
				return fixtureResponse(req, tc.status, "text/html", tc.body), nil
			})}
			r := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
			if r.Outcome != tc.outcome {
				t.Fatalf("got %s: %s", r.Outcome, r.Summary)
			}
		})
	}
}

func TestMissingIPQualityFieldsStayUnknown(t *testing.T) {
	for _, id := range []string{"ping0_ip_quality", "ippure_ip_quality"} {
		rule, _ := RuleFor(id)
		checker := Checker{ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
			return fixtureResponse(req, 200, "application/json", `{"ip":"203.0.113.7"}`), nil
		})}
		result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
		if result.Details["ip_type"] != "未提供" {
			t.Fatalf("missing property invented a network type: %+v", result)
		}
		score := "risk_score"
		if id == "ippure_ip_quality" {
			score = "fraud_score"
		}
		if result.Details[score] != "未提供" {
			t.Fatalf("missing score became low risk: %+v", result)
		}
	}
}

func TestIPFamilyObservationNeedsAnAddressOfTheRequestedFamily(t *testing.T) {
	for _, tc := range []struct{ service, body, outcome string }{
		{"exit_ipv4", `{"ip":"203.0.113.7"}`, "profiled"},
		{"exit_ipv6", `{"ip":"2001:db8::7"}`, "profiled"},
		{"exit_ipv6", `{"ip":"203.0.113.7"}`, "unknown"},
	} {
		rule, _ := RuleFor(tc.service)
		checker := Checker{ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
			return fixtureResponse(req, http.StatusOK, "application/json", tc.body), nil
		})}
		result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
		if result.Outcome != tc.outcome {
			t.Fatalf("%s with %s: got %s", tc.service, tc.body, result.Outcome)
		}
	}
}
