package publicservice

import (
	"context"
	"github.com/faceair/clash-speedtest/core/monitor"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAntigravityAvailabilityRequiresModelContent(t *testing.T) {
	rule, _ := RuleFor("antigravity")
	for _, tc := range []struct {
		name          string
		status        int
		body, outcome string
	}{
		{"content", 200, `data: {"response":{"candidates":[{"content":{"parts":[{"text":"OK"}]}}]}}` + "\n\n", "matched"},
		{"empty200", 200, `{}`, "unknown"},
		{"thoughtOnly", 200, `{"candidates":[{"content":{"parts":[{"text":"thinking","thought":true}]}}]}`, "unknown"},
		{"forbidden", 403, `{"error":{"status":"PERMISSION_DENIED"}}`, "permission_denied"},
		{"quota", 429, `{"error":{"status":"RESOURCE_EXHAUSTED"}}`, "rate_limited"},
		{"region", 400, `{"error":{"message":"User location is not supported"}}`, "region_blocked"},
		{"badModel", 400, `{"error":{"status":"INVALID_ARGUMENT"}}`, "unknown"},
		{"lateStreamError", 200, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"OK\"}]}}]}\n\ndata: {\"error\":{\"status\":\"INTERNAL\"}}\n", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			checker := Checker{AntigravityToken: "test-secret", ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host == "www.cloudflare.com" {
					if req.Header.Get("Authorization") != "" || req.Method != http.MethodGet {
						t.Error("credential leaked to exit observer")
					}
					return fixtureResponse(req, 200, "text/plain", "ip=203.0.113.8\nloc=JP\ncolo=NRT\n"), nil
				}
				calls++
				if req.Header.Get("Authorization") != "Bearer test-secret" || req.Method != http.MethodPost || req.URL.Host != "daily-cloudcode-pa.googleapis.com" {
					t.Error("credential or isolated request contract mismatch")
				}
				switch calls {
				case 1:
					if req.URL.Path != "/v1internal:loadCodeAssist" {
						t.Error("missing project discovery")
					}
					return fixtureResponse(req, 200, "application/json", `{"cloudaicompanionProject":"actual-project"}`), nil
				case 2:
					return fixtureResponse(req, 200, "application/json", `{"models":{"gemini-fixture-flash":{}}}`), nil
				default:
					b, _ := io.ReadAll(req.Body)
					if !strings.Contains(string(b), "actual-project") || !strings.Contains(string(b), "gemini-fixture-flash") || req.URL.Path != "/v1internal:streamGenerateContent" {
						t.Error("model probe did not use discovered configuration")
					}
					return fixtureResponse(req, tc.status, "text/event-stream", tc.body), nil
				}
			})}
			result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
			if result.Outcome != tc.outcome || calls != 3 || result.RequestCount != 4 || result.Model != "gemini-fixture-flash" || result.Details["ip"] != "203.0.113.8" {
				t.Fatalf("got %+v calls %d", result, calls)
			}
			progress := "轻量模型已列出，尚未收到真实回答"
			if tc.outcome == "matched" {
				progress = "真实模型已回答"
			}
			if result.Details["antigravity_progress"] != progress || result.Details["checked_model"] != "gemini-fixture-flash" {
				t.Fatalf("model evidence is ambiguous: %+v", result.Details)
			}
			if strings.Contains(result.ErrorMessage, "test-secret") {
				t.Fatal("credential leaked")
			}
		})
	}
}

func TestAntigravityMissingCredentialsDoesNotConnect(t *testing.T) {
	rule, _ := RuleFor("antigravity")
	checker := Checker{ClientFactory: func(monitor.MonitoredNode, time.Duration) (*http.Client, error) {
		t.Fatal("must not create client without credentials")
		return nil, nil
	}}
	result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	if result.Outcome != "credentials_required" || result.RequestCount != 0 {
		t.Fatalf("got %+v", result)
	}
}

func TestAntigravityAcceptsCompleteModelCatalogLargerThanGenericProbeLimit(t *testing.T) {
	rule, _ := RuleFor("antigravity")
	modelCatalog := `{"models":{"gemini-fixture-flash":{}},"padding":"` + strings.Repeat("x", MaximumResponseBytes) + `"}`
	checker := Checker{AntigravityToken: "test-secret", ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "www.cloudflare.com" {
			return fixtureResponse(req, 200, "text/plain", "ip=203.0.113.8\nloc=JP\ncolo=NRT\n"), nil
		}
		switch req.URL.Path {
		case "/v1internal:loadCodeAssist":
			return fixtureResponse(req, 200, "application/json", `{"cloudaicompanionProject":"actual-project"}`), nil
		case "/v1internal:fetchAvailableModels":
			return fixtureResponse(req, 200, "application/json", modelCatalog), nil
		default:
			return fixtureResponse(req, 200, "text/event-stream", "data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"OK\"}]}}]}}\n\n"), nil
		}
	})}
	result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	if result.Outcome != "matched" || result.Model != "gemini-fixture-flash" {
		t.Fatalf("large but complete model catalog must be usable: %+v", result)
	}
}

func TestAntigravityCancellationStopsBeforeNextRequest(t *testing.T) {
	rule, _ := RuleFor("antigravity")
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	checker := Checker{AntigravityToken: "test-secret", ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "www.cloudflare.com" {
			return fixtureResponse(req, 503, "text/plain", "unavailable"), nil
		}
		calls.Add(1)
		cancel()
		return nil, req.Context().Err()
	})}
	result := checker.Check(ctx, monitor.MonitoredNode{}, rule, DefaultTimeout)
	if result.Outcome != "cancelled" || calls.Load() != 1 {
		t.Fatalf("cancellation did not stop: %+v, calls=%d", result, calls.Load())
	}
}
