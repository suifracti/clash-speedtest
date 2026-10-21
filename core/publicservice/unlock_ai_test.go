package publicservice

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type aiUnlockRoundTripper func(*http.Request) (*http.Response, error)

func (f aiUnlockRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func aiUnlockFixtureClient(status int, body string) *http.Client {
	return &http.Client{Transport: aiUnlockRoundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"text/html"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
}
func TestAIUnlockOfficialRegionEvidence(t *testing.T) {
	for _, rule := range aiUnlockRules() {
		t.Run(rule.ServiceID, func(t *testing.T) {
			client := &http.Client{Transport: aiUnlockRoundTripper(func(req *http.Request) (*http.Response, error) {
				if req.Header.Get("User-Agent") == "" || req.Header.Get("Accept-Language") == "" {
					t.Fatal("missing browser headers")
				}
				body := "loc=JP\nip=203.0.113.1\n"
				if req.URL.Host == "api.openai.com" {
					body = `{"cookie_consent_required":false}`
				} else if rule.ServiceID == "gemini_unlock" {
					body = `<script>45631641,null,true,2,1,200,"JPN"</script>`
				}
				return fixtureResponse(req, 200, "text/html", body), nil
			})}
			result := checkAIUnlock(context.Background(), client, rule, time.Now())
			if result.Outcome != "unlocked" || result.Details["service_region"] != "JP" {
				t.Fatalf("%+v", result)
			}
		})
	}
}
func TestAIUnlockDoesNotTreatMissingEvidenceAsSupported(t *testing.T) {
	for _, rule := range aiUnlockRules() {
		result := checkAIUnlock(context.Background(), aiUnlockFixtureClient(200, `<html>Welcome</html>`), rule, time.Now())
		if result.Outcome != "unknown" {
			t.Fatalf("%s: %+v", rule.ServiceID, result)
		}
	}
	rule := aiUnlockRules()[1]
	result := checkAIUnlock(context.Background(), aiUnlockFixtureClient(200, "loc=HK\n"), rule, time.Now())
	if result.Outcome != "region_blocked" {
		t.Fatalf("%+v", result)
	}
}
