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
func TestGeminiBlockedCountriesFollowSubsCheckTable(t *testing.T) {
	rule, _ := RuleFor("gemini_unlock")
	for _, tc := range []struct{ code, region, summary string }{
		{"CHN", "CN", "Google 判定为中国大陆"},
		{"RUS", "RU", "不可用（RU）"},
		{"HKG", "HK", "不可用（HK）"},
		{"MAC", "MO", "不可用（MO）"},
		{"BLR", "BY", "不可用（BY）"},
	} {
		body := `<script>,2,1,200,"` + tc.code + `",45631641,null,true</script>`
		result := checkAIUnlock(context.Background(), aiUnlockFixtureClient(200, body), rule, time.Now())
		if result.Outcome != "region_blocked" || result.Details["service_region"] != tc.region || !strings.Contains(result.Summary, tc.summary) {
			t.Fatalf("%s: %+v", tc.code, result)
		}
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

func TestGeminiSupportsEscapedBootstrapFlagWithoutRelaxingCountryBlock(t *testing.T) {
	rule, ok := RuleFor("gemini_unlock")
	if !ok {
		t.Fatal("missing gemini_unlock")
	}
	unlockedBody := `<script>var x = ',2,1,200,"SGP"'; var y = "\"45631641\":true";</script>`
	got := checkAIUnlock(context.Background(), aiUnlockFixtureClient(200, unlockedBody), rule, time.Now())
	if got.Outcome != "unlocked" || got.Details["service_region"] != "SG" {
		t.Fatalf("escaped flag failed: %+v", got)
	}

	blockedBody := `<script>var x = ',2,1,200,"HKG"'; var y = "\"45631641\":true";</script>`
	blocked := checkAIUnlock(context.Background(), aiUnlockFixtureClient(200, blockedBody), rule, time.Now())
	if blocked.Outcome != "region_blocked" || blocked.Details["service_region"] != "HK" {
		t.Fatalf("blocked country leaked: %+v", blocked)
	}

	falseBody := `<script>var x = ',2,1,200,"SGP"'; var y = "\"45631641\":false";</script>`
	unknown := checkAIUnlock(context.Background(), aiUnlockFixtureClient(200, falseBody), rule, time.Now())
	if unknown.Outcome != "unknown" {
		t.Fatalf("false flag unlocked: %+v", unknown)
	}
}
