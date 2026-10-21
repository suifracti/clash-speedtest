package publicservice

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Synthetic page shapes mirror subs-check's verdicts; no provider tokens or copied page.
func youtubeFixture(t *testing.T, body string) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Query().Get("hl") != "en" {
			t.Error("missing hl=en")
		}
		if req.Header.Get("Sec-Ch-Ua") == "" || req.Header.Get("Sec-Fetch-Mode") != "navigate" || !strings.Contains(req.Header.Get("User-Agent"), "Windows") {
			t.Error("missing browser headers")
		}
		return fixtureResponse(req, 200, "text/html", body), nil
	})}
}

func TestYouTubePremiumVerdicts(t *testing.T) {
	for _, tc := range []struct{ name, body, want, region string }{
		{"google cn means mainland", `<html>YouTube <a href="https://www.google.cn/">link</a></html>`, "region_blocked", "CN"},
		{"country unavailable", `<html><h1>YouTube Premium is not available in your country</h1></html>`, "region_blocked", ""},
		{"region unavailable", `<html>premium is not available in your region</html>`, "region_blocked", ""},
		{"innertube context region", `<script>var ytcfg={"INNERTUBE_CONTEXT_GL":"SG"};</script><title>YouTube Premium</title>`, "matched", "SG"},
		{"innertube wins over country code", `<script>{"INNERTUBE_CONTEXT_GL":"JP","countryCode":"US"}</script><title>YouTube Premium</title>`, "matched", "JP"},
		{"country-code element", `<title>YouTube Premium</title><span id="country-code">HK</span>`, "matched", "HK"},
		{"premium marker without region", `<html>YouTube Premium</html>`, "unknown", ""},
		{"marketing text is not a premium marker", `<meta name="description" content="videos">`, "unknown", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule, _ := RuleFor("youtube_premium")
			result := checkYouTubePremium(context.Background(), youtubeFixture(t, tc.body), rule, time.Now())
			if result.Outcome != tc.want {
				t.Fatalf("outcome=%s want=%s result=%+v", result.Outcome, tc.want, result)
			}
			if tc.region != "" && result.Details["service_region"] != tc.region {
				t.Fatalf("region=%q want=%q details=%v", result.Details["service_region"], tc.region, result.Details)
			}
		})
	}
}

func TestYouTubePremiumOversizeRemainsUnconfirmed(t *testing.T) {
	rule, _ := RuleFor("youtube_premium")
	const limit = 2 * 1024 * 1024
	body := `<title>YouTube Premium</title>` + strings.Repeat(" ", limit)
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return fixtureResponse(req, 200, "text/html", body), nil
	})}
	result := checkYouTubePremium(context.Background(), client, rule, time.Now())
	if result.Outcome != "unknown" || result.FailurePhase != "response_evidence_limit" || result.BytesRead != limit {
		t.Fatalf("oversize became a verdict or used wrong bound: %+v", result)
	}
}

func TestYouTubeConsentPageStaysUnknown(t *testing.T) {
	rule, _ := RuleFor("youtube_premium")
	result := checkYouTubePremium(context.Background(), youtubeFixture(t, `<h1>Before you continue to YouTube</h1>{"countryCode":"US"}`), rule, time.Now())
	if result.Outcome != "unknown" {
		t.Fatalf("consent page became a verdict: %+v", result)
	}
}
