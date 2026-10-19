package publicservice

import (
	"context"
	"github.com/faceair/clash-speedtest/core/monitor"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestResourceProbesKeepChallengesAndRejectHTMLInsteadOfImages(t *testing.T) {
	rule, ok := RuleFor("resource_wechat")
	if !ok {
		t.Fatal("WeChat resource missing")
	}
	for _, tc := range []struct {
		name, body, outcome string
		status              int
		challenge           bool
	}{
		{"icon", "\x00\x00\x01\x00fixture", "reachable", 200, false},
		{"HTML fallback", "<!DOCTYPE html><html>login</html>", "criteria_mismatch", 200, false},
		{"empty", "", "criteria_mismatch", 200, false},
		{"denied", "denied", "http_rejected", 403, false},
		{"redirect", "", "redirect", 302, false},
		{"challenge", "challenge", "challenge", 403, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &http.Client{Transport: resourceTestTransport(func(r *http.Request) (*http.Response, error) {
				h := make(http.Header)
				if tc.challenge {
					h.Set("cf-mitigated", "challenge")
				}
				return &http.Response{StatusCode: tc.status, Header: h, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			got := (Checker{}).checkOnce(context.Background(), c, monitor.MonitoredNode{}, rule, time.Now())
			if got.Outcome != tc.outcome {
				t.Fatalf("%+v", got)
			}
		})
	}
}

type resourceTestTransport func(*http.Request) (*http.Response, error)

func (f resourceTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestResourceCatalogIsSelectableWithoutChangingExistingAI(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range Catalog() {
		seen[r.ServiceID] = true
	}
	for _, id := range []string{"resource_wechat", "resource_qq", "resource_telegram", "resource_deepseek", "resource_qwen", "resource_kimi", "resource_perplexity", "resource_chatgpt", "resource_claude", "resource_grok", "resource_gemini", "resource_bilibili", "resource_douyin", "resource_xiaohongshu", "chatgpt_web", "claude_web", "grok_web", "gemini_web"} {
		if !seen[id] {
			t.Errorf("missing %s", id)
		}
	}
	if seen["microsoft_connect"] || seen["firefox_portal"] {
		t.Fatal("retired targets in active picker")
	}
}

func TestExistingAIWebRecognizesChallengeBodyOnHTTPRefusal(t *testing.T) {
	for _, id := range []string{"chatgpt_web", "claude_web", "gemini_web"} {
		t.Run(id, func(t *testing.T) {
			rule, _ := RuleFor(id)
			c := Checker{ClientFactory: fixtureClient(func(r *http.Request) (*http.Response, error) {
				return fixtureResponse(r, 403, "text/html", `<html><title>Just a moment...</title><script src="/cdn-cgi/challenge-platform/test"></script></html>`), nil
			})}
			got := c.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
			if got.Outcome != "challenge" {
				t.Fatalf("challenge lost: %+v", got)
			}
		})
	}
}
