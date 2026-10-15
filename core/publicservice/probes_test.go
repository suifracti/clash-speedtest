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

func TestIPQualityASNFormatResilience(t *testing.T) {
	for _, tc := range []struct {
		service string
		body    string
		wantASN string
	}{
		{"ping0_ip_quality", `{"ip":"203.0.113.7","asn":64500}`, "AS64500"},
		{"ping0_ip_quality", `{"ip":"203.0.113.7","asn":"AS64500"}`, "AS64500"},
		{"ping0_ip_quality", `{"ip":"203.0.113.7","asn":null}`, ""},
		{"ippure_ip_quality", `{"ip":"203.0.113.7","asn":"AS64500"}`, "AS64500"},
		{"ippure_ip_quality", `{"ip":"203.0.113.7","asn":64500}`, "AS64500"},
		{"ippure_ip_quality", `{"ip":"203.0.113.7","asn":null}`, ""},
	} {
		rule, ok := RuleFor(tc.service)
		if !ok {
			t.Fatalf("missing rule %s", tc.service)
		}
		checker := Checker{ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
			return fixtureResponse(req, http.StatusOK, "application/json", tc.body), nil
		})}
		result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
		if result.Outcome != "profiled" {
			t.Fatalf("%s with %s failed: outcome=%s, err=%s", tc.service, tc.body, result.Outcome, result.ErrorMessage)
		}
		if result.Details["asn"] != tc.wantASN {
			t.Fatalf("%s with %s: got asn %q, want %q", tc.service, tc.body, result.Details["asn"], tc.wantASN)
		}
	}
}

func TestCloudflareTraceWhitespaceAndCasingResilience(t *testing.T) {
	rule, ok := RuleFor("cloudflare_trace")
	if !ok {
		t.Fatal("missing cloudflare_trace rule")
	}
	body := "fl=123\n ip = 203.0.113.10 \n loc = jp \n colo = NRT \n tls=TLSv1.3\n http=http/2\n"
	checker := Checker{ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
		return fixtureResponse(req, http.StatusOK, "text/plain", body), nil
	})}
	result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	if result.Outcome != "profiled" {
		t.Fatalf("expected profiled, got %s: %s", result.Outcome, result.ErrorMessage)
	}
	if result.Details["ip"] != "203.0.113.10" {
		t.Fatalf("expected ip 203.0.113.10, got %q", result.Details["ip"])
	}
	if result.Details["country_code"] != "JP" {
		t.Fatalf("expected country_code JP, got %q", result.Details["country_code"])
	}
	if result.Details["cloudflare_colo"] != "NRT" {
		t.Fatalf("expected colo NRT, got %q", result.Details["cloudflare_colo"])
	}
}

func TestIPifyMappedAddress(t *testing.T) {
	rule, ok := RuleFor("exit_ipv4")
	if !ok {
		t.Fatal("missing exit_ipv4 rule")
	}
	body := `{"ip": "::ffff:203.0.113.15"}`
	checker := Checker{ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
		return fixtureResponse(req, http.StatusOK, "application/json", body), nil
	})}
	result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	if result.Outcome != "profiled" {
		t.Fatalf("expected profiled, got %s: %s", result.Outcome, result.ErrorMessage)
	}
	if result.Details["ip"] != "203.0.113.15" {
		t.Fatalf("expected cleanly converted IPv4 203.0.113.15, got %q", result.Details["ip"])
	}
}

func TestRuleForWhitespaceResilience(t *testing.T) {
	rule, ok := RuleFor("  cloudflare_204  ")
	if !ok || rule.ServiceID != "cloudflare_204" {
		t.Fatalf("expected to find cloudflare_204 with surrounding whitespace, got: %+v, ok=%v", rule, ok)
	}
}



