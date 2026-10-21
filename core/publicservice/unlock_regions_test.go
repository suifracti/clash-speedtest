package publicservice

import (
	"context"
	"github.com/faceair/clash-speedtest/core/monitor"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRegionUnlockAPIs(t *testing.T) {
	for _, id := range []string{"spotify_unlock", "tiktok_unlock"} {
		t.Run(id, func(t *testing.T) {
			rule, _ := RuleFor(id)
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				body := `{"status":311,"country":"JP","is_country_launched":true}`
				switch req.URL.String() {
				case tiktokRegionURL:
					body = strings.Repeat(" ", 70000) + `<script id="__UNIVERSAL_DATA_FOR_REHYDRATION__">{"__DEFAULT_SCOPE__":{"webapp.app-context":{"region":"JP"}}}</script>`
				}
				return fixtureResponse(req, 200, "application/json", body), nil
			})}
			got := checkRegionUnlock(context.Background(), client, rule, time.Now())
			if got.Outcome != "unlocked" || got.Details["service_region"] != "JP" {
				t.Fatalf("%+v", got)
			}
			failure := `{}`
			want := "unknown"
			if id == "spotify_unlock" {
				failure = `{"status":320,"errors":{"generic_error":"You seem to be using a proxy service."}}`
				want = "service_rejected"
			}
			missing := checkRegionUnlock(context.Background(), aiUnlockFixtureClient(200, failure), rule, time.Now())
			if missing.Outcome != want {
				t.Fatalf("missing evidence: %+v", missing)
			}
		})
	}
}

func TestDisneyUnsupportedDoesNotCreateClient(t *testing.T) {
	rule, _ := RuleFor("disney_unlock")
	checker := Checker{ClientFactory: func(monitor.MonitoredNode, time.Duration) (*http.Client, error) {
		t.Fatal("unsupported service must not open a node client")
		return nil, nil
	}}
	got := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, time.Second)
	if got.Outcome != "not_executed" || got.Summary != "暂不支持" || got.RequestCount != 0 {
		t.Fatalf("%+v", got)
	}
}
