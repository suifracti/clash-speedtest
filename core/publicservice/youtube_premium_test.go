package publicservice

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
)

// Synthetic public-page shapes; no provider tokens, account data or copied page.
const premiumOfferFixture = `<script>var ytInitialData = {"contents":{"lpOfferCardOptionViewModel":{"title":{"content":"fixture plan"},"purchaseButtonOverride":{"buttonViewModel":{"onTap":{"innertubeCommand":{"fixture":true}}}}}}};</script>`

func TestYouTubePremiumUsesOfferEvidenceInsteadOfMarketingText(t *testing.T) {
	for _, tc := range []struct{ name, region, body, want string }{
		{"offer after old read limit", "SG", strings.Repeat(" ", MaximumResponseBytes+128) + premiumOfferFixture, "matched"},
		{"generic marketing is not an offer", "SG", `<meta name="description" content="ad-free videos">`, "unknown"},
		{"unrelated Google CN link is not a region", "SG", `<a href="https://www.google.cn/">link</a>` + premiumOfferFixture, "matched"},
		{"explicit CN stays restricted", "CN", premiumOfferFixture, "region_blocked"},
		{"lowercase country code", "cn", premiumOfferFixture, "region_blocked"},
		{"offer title without actionable button", "SG", `<script>var ytInitialData={"lpOfferCardOptionViewModel":{"title":{"content":"fixture plan"}}};</script>`, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule, _ := RuleFor("youtube_premium")
			body := `<script>var cfg={"countryCode":"` + tc.region + `"};</script>` + tc.body
			checker := Checker{ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
				return fixtureResponse(req, 200, "text/html", body), nil
			})}
			r := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
			if r.Outcome != tc.want {
				t.Fatalf("outcome=%s want=%s result=%+v", r.Outcome, tc.want, r)
			}
			if r.Details["service_region"] != strings.ToUpper(tc.region) {
				t.Fatalf("region lost: %+v", r)
			}
		})
	}
}

func TestYouTubePremiumOversizeRemainsUnconfirmed(t *testing.T) {
	rule, _ := RuleFor("youtube_premium")
	const limit = 2 * 1024 * 1024
	body := `{"countryCode":"SG"}` + premiumOfferFixture + strings.Repeat(" ", limit)
	checker := Checker{ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
		return fixtureResponse(req, 200, "text/html", body), nil
	})}
	r := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	if r.Outcome != "unknown" || r.FailurePhase != "response_evidence_limit" || r.BytesRead != limit {
		t.Fatalf("oversize became a verdict or used wrong bound: %+v", r)
	}
}

func TestYouTubeConsentAndConfigTextDoNotBecomeVerdicts(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{`<script>var cfg={"INNERTUBE_CONTEXT_GL":"US"};var ytInitialData={"contents":{"title":{"content":"YouTube Premium is not available in your country"}}};</script>`, "unknown"},
		{`<h1>Before you continue to YouTube</h1>{"countryCode":"US"}` + premiumOfferFixture, "unknown"},
		{`<script>var cfg={"countryCode":"JP","unavailable":"Premium is not available in your country"};</script>` + premiumOfferFixture, "matched"},
	} {
		r, _ := RuleFor("youtube_premium")
		client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Header.Get("Accept-Language") == "" || req.Header.Get("User-Agent") == "" {
				t.Fatal("missing browser headers")
			}
			return fixtureResponse(req, 200, "text/html", tc.body), nil
		})}
		got := checkYouTubePremium(context.Background(), client, r, time.Now())
		if got.Outcome != tc.want {
			t.Fatalf("got %+v", got)
		}
	}
}
