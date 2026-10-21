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

func (f aiUnlockRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func aiUnlockFixtureClient(status int, body string) *http.Client {
	return &http.Client{Transport: aiUnlockRoundTripper(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})}
}

func TestAIUnlockProviderPageEvidence(t *testing.T) {
	markers := map[string]string{
		"chatgpt_unlock": `<h1>What are you working on?</h1><textarea id="prompt-textarea"></textarea><button>Chat with ChatGPT</button>`,
		"claude_unlock":  `<main>Claude<textarea placeholder="How can I help you today?"></textarea></main>`,
		"gemini_unlock":  `<main>Google Gemini<textarea placeholder="Ask Gemini"></textarea></main>`,
	}
	for _, rule := range aiUnlockRules() {
		t.Run(rule.ServiceID, func(t *testing.T) {
			rule.TargetURL = "https://fixture.example/"
			result := checkAIUnlock(context.Background(), aiUnlockFixtureClient(http.StatusOK, markers[rule.ServiceID]), rule, time.Now())
			if result.Outcome != "unlocked" {
				t.Fatalf("expected provider page evidence to unlock, got %+v", result)
			}
		})
	}
}

func TestAIUnlockNeedsProviderEvidence(t *testing.T) {
	rule := aiUnlockRules()[0]
	rule.TargetURL = "https://fixture.example/"
	for _, tc := range []struct {
		name, body, outcome string
	}{
		{"generic 200", `<html><title>Welcome</title></html>`, "unknown"},
		{"explicit region refusal", `<main>ChatGPT is not available in your country.</main>`, "region_blocked"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := checkAIUnlock(context.Background(), aiUnlockFixtureClient(http.StatusOK, tc.body), rule, time.Now())
			if result.Outcome != tc.outcome {
				t.Fatalf("expected %s, got %+v", tc.outcome, result)
			}
		})
	}
}
