package publicservice

import (
	"context"
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

func TestDisneyRegionFlow(t *testing.T) {
	rule, _ := RuleFor("disney_unlock")
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Host != "disney.api.edge.bamgrid.com" {
			t.Fatalf("unexpected Disney target %s", req.URL)
		}
		switch req.URL.Path {
		case "/devices":
			return fixtureResponse(req, 200, "application/json", `{"assertion":"fixture-assertion"}`), nil
		case "/token":
			return fixtureResponse(req, 200, "application/json", `{"refresh_token":"fixture-refresh"}`), nil
		case "/graph/v1/device/graphql":
			return fixtureResponse(req, 200, "application/json", `{"data":{"countryCode":"SG","inSupportedLocation":true}}`), nil
		}
		t.Fatalf("unexpected Disney path %s", req.URL.Path)
		return nil, nil
	})}
	got := checkDisneyRegion(context.Background(), client, rule, time.Now())
	if got.Outcome != "unlocked" || got.Details["service_region"] != "SG" || calls != 3 || rule.RuleVersion != 4 {
		t.Fatalf("Disney flow did not reach a region: calls=%d result=%+v", calls, got)
	}

	banned := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return fixtureResponse(req, http.StatusForbidden, "application/json", `{}`), nil
	})}
	blocked := checkDisneyRegion(context.Background(), banned, rule, time.Now())
	if blocked.Outcome != "banned" || blocked.Details["service_region"] != "" {
		t.Fatalf("Disney 403 was not treated as a ban: %+v", blocked)
	}
}
