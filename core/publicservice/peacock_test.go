package publicservice

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/faceair/clash-speedtest/core/monitor"
)

func TestPeacockTerritoryNoticeIsNotAvailableHomepage(t *testing.T) {
	for _, tc := range []struct{ name, path, body, want string }{
		{"explicit notice beyond old prefix", "/unavailable", strings.Repeat(" ", 30*1024) + `<p>Peacock is not yet available in your territory.</p>`, "region_blocked"},
		{"unavailable path alone insufficient", "/unavailable", "<p>Temporary maintenance</p>", "unknown"},
		{"unrelated text is not a location check", "/", `<p>Our help page explains: not yet available in your territory.</p>`, "reachable"},
		{"normal public page", "/", "<p>Welcome to Peacock</p>", "reachable"},
		{"challenge takes precedence", "/unavailable", `<script src="/cdn-cgi/challenge-platform/x"></script><p>not yet available in your territory</p>`, "challenge"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			rule, _ := RuleFor("peacock_web")
			c := Checker{ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 && tc.path != "/" {
					r := fixtureResponse(req, 302, "text/html", "")
					r.Header.Set("Location", tc.path)
					return r, nil
				}
				return fixtureResponse(req, 200, "text/html", tc.body), nil
			})}
			r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
			if r.Outcome != tc.want {
				t.Fatalf("outcome=%s want=%s details=%v", r.Outcome, tc.want, r.Details)
			}
			if tc.want == "region_blocked" && (r.Details["playback_verified"] != "false" || r.Details["restriction_evidence"] != "unavailable_path_and_explicit_territory_notice") {
				t.Fatalf("missing evidence: %v", r.Details)
			}
		})
	}
}
