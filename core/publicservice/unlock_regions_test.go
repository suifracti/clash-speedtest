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

func TestTikTokSlardarWAFClassifiedAsChallenge(t *testing.T) {
	rule, ok := RuleFor("tiktok_unlock")
	if !ok {
		t.Fatal("missing tiktok_unlock")
	}
	wafBody := `<html><head><script id="slardar-config">{"slardarClient":"SlardarWAF"}</script></head><body>Please wait...<p id="wci" class="_wafchallengeid"></p></body></html>`
	got := checkRegionUnlock(context.Background(), aiUnlockFixtureClient(200, wafBody), rule, time.Now())
	if got.Outcome != "challenge" {
		t.Fatalf("SlardarWAF should classify as challenge, got %+v", got)
	}
}

func TestUNextApolloHeadersAndRegionStatus(t *testing.T) {
	rule, ok := RuleFor("unext_unlock")
	if !ok {
		t.Fatal("missing unext_unlock")
	}
	for _, tc := range []struct {
		name, body, want, region string
	}{
		{"outside japan 467", `{"data":{"webfront_playlistUrl":{"resultStatus":467}}}`, "region_blocked", ""},
		{"japan 200", `{"data":{"webfront_playlistUrl":{"resultStatus":200}}}`, "unlocked", "JP"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Header.Get("apollo-require-preflight") != "true" || req.Header.Get("Content-Type") != "application/json" {
					t.Fatalf("missing Apollo CSRF preflight headers: %v", req.Header)
				}
				return fixtureResponse(req, 200, "application/json", tc.body), nil
			})}
			got := checkRegionUnlock(context.Background(), client, rule, time.Now())
			if got.Outcome != tc.want || got.Details["service_region"] != tc.region {
				t.Fatalf("got %+v, want %s/%s", got, tc.want, tc.region)
			}
		})
	}
}

func TestDMMGraphQLRegionCheck(t *testing.T) {
	rule, ok := RuleFor("dmm_unlock")
	if !ok {
		t.Fatal("missing dmm_unlock")
	}
	for _, tc := range []struct {
		name, body, want, region string
	}{
		{"foreign access true", `{"data":{"client":{"isForeignAccess":true}}}`, "region_blocked", ""},
		{"japan local access", `{"data":{"client":{"isForeignAccess":false}}}`, "unlocked", "JP"},
		{"missing field", `{"data":{}}`, "unknown", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodPost || req.Header.Get("Content-Type") != "application/json" {
					t.Fatalf("unexpected DMM request method=%s content-type=%s", req.Method, req.Header.Get("Content-Type"))
				}
				return fixtureResponse(req, 200, "application/json", tc.body), nil
			})}
			got := checkRegionUnlock(context.Background(), client, rule, time.Now())
			if got.Outcome != tc.want || got.Details["service_region"] != tc.region {
				t.Fatalf("got %+v, want %s/%s", got, tc.want, tc.region)
			}
		})
	}
}
