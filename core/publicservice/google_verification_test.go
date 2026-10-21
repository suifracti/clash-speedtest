package publicservice

import (
	"context"
	"github.com/faceair/clash-speedtest/core/monitor"
	"net/http"
	"testing"
	"time"
)

func TestGoogleVerificationRedirect(t *testing.T) {
	for _, id := range []string{"gemini_unlock", "youtube_premium", "google_region"} {
		rule, _ := RuleFor(id)
		client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			response := fixtureResponse(req, 302, "text/html", "")
			response.Header.Set("Location", "https://www.google.com/sorry/index")
			return response, nil
		})}
		result := (Checker{}).checkOnce(context.Background(), client, monitor.MonitoredNode{}, rule, time.Now())
		if result.Outcome != "verification_required" || result.ErrorMessage != googleVerificationReason || result.RequestCount != 1 {
			t.Fatalf("%s: %+v", id, result)
		}
	}
}
func TestGeminiMainlandWithEnglishLanguage(t *testing.T) {
	rule, _ := RuleFor("gemini_unlock")
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Accept-Language") != "en-US,en;q=0.9" {
			t.Fatal("wrong language")
		}
		return fixtureResponse(req, 200, "text/html", `,2,1,200,"CHN"`), nil
	})}
	got := checkAIUnlock(context.Background(), client, rule, time.Now())
	if got.Outcome != "region_blocked" || got.Summary != "Google 判定为中国大陆" || got.Details["region_raw"] != `,2,1,200,"CHN"` {
		t.Fatalf("%+v", got)
	}
}
