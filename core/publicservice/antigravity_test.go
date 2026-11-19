package publicservice

import (
	"context"
	"encoding/json"
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
		{"content", 200, `data: {"response":{"candidates":[{"content":{"parts":[{"text":"READY"}]}}]}}` + "\n\n", "matched"},
		{"empty200", 200, `{}`, "unknown"},
		{"thoughtOnly", 200, `{"candidates":[{"content":{"parts":[{"text":"thinking","thought":true}]}}]}`, "unknown"},
		{"bodyRefusal200", 200, `data: {"response":{"candidates":[{"content":{"parts":[{"text":"This feature is not available in your region."}]}}]}}` + "\n\n", "region_blocked"},
		{"safetyBlock200", 200, `{"promptFeedback":{"blockReason":"SAFETY"}}`, "service_rejected"},
		{"forbidden", 403, `{"error":{"status":"PERMISSION_DENIED"}}`, "permission_denied"},
		{"quota", 429, `{"error":{"status":"RESOURCE_EXHAUSTED"}}`, "rate_limited"},
		{"region", 400, `{"error":{"message":"User location is not supported"}}`, "region_blocked"},
		{"badModel", 400, `{"error":{"status":"INVALID_ARGUMENT"}}`, "unknown"},
		{"lateStreamError", 200, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"READY\"}]}}]}\n\ndata: {\"error\":{\"status\":\"INTERNAL\"}}\n", "unknown"},
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
				if req.Header.Get("Authorization") != "Bearer test-secret" || req.Method != http.MethodPost || req.URL.Host != "daily-cloudcode-pa.googleapis.com" {
					t.Error("credential or isolated request contract mismatch")
				}
				if req.URL.Path == "/v1internal:fetchUserInfo" {
					return fixtureResponse(req, 200, "application/json", `{"regionCode":"JP"}`), nil
				}
				calls++
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
					if !strings.Contains(string(b), "actual-project") || !strings.Contains(string(b), "gemini-fixture-flash") || !strings.Contains(string(b), "READY") || req.URL.Path != "/v1internal:streamGenerateContent" {
						t.Error("model probe did not use discovered configuration")
					}
					return fixtureResponse(req, tc.status, "text/event-stream", tc.body), nil
				}
			})}
			result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
			wantRequests := 5
			if result.Outcome != tc.outcome || calls != 3 || result.RequestCount != wantRequests || result.Model != "gemini-fixture-flash" || result.Details["ip"] != "203.0.113.8" {
				t.Fatalf("got %+v calls %d", result, calls)
			}
			progress := "轻量模型已列出，尚未收到真实回答"
			wantReply := ""
			if tc.outcome == "matched" {
				progress = "真实模型已回答"
				wantReply = "READY"
			}
			if result.Details["antigravity_progress"] != progress || result.Details["checked_model"] != "gemini-fixture-flash" || result.Details["models_listed"] != "1" || result.Details["all_models_count"] != "1" || result.Details["reply_preview"] != wantReply {
				t.Fatalf("model evidence is ambiguous: %+v", result.Details)
			}
			if result.Details["google_region"] != "JP" {
				t.Fatalf("expected google_region JP, got: %+v", result.Details)
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
		case "/v1internal:fetchUserInfo":
			return fixtureResponse(req, 200, "application/json", `{"regionCode":"JP"}`), nil
		case "/v1internal:loadCodeAssist":
			return fixtureResponse(req, 200, "application/json", `{"cloudaicompanionProject":"actual-project"}`), nil
		case "/v1internal:fetchAvailableModels":
			return fixtureResponse(req, 200, "application/json", modelCatalog), nil
		default:
			return fixtureResponse(req, 200, "text/event-stream", "data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"READY\"}]}}]}}\n\n"), nil
		}
	})}
	result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	if result.Outcome != "matched" || result.Model != "gemini-fixture-flash" {
		t.Fatalf("large but complete model catalog must be usable: %+v", result)
	}
}

func TestAntigravityGoogleRegionGeofenceOverridesModel200(t *testing.T) {
	rule, _ := RuleFor("antigravity")
	for _, tc := range []struct {
		code string
		want string
	}{
		{"CN", "CN（送中）"},
		{"HK", "判定为 HK"},
		{"MO", "判定为 MO"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			streamCalled := 0
			checker := Checker{AntigravityToken: "test-secret", ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host == "www.cloudflare.com" {
					return fixtureResponse(req, 200, "text/plain", "ip=203.0.113.8\nloc=JP\ncolo=NRT\n"), nil
				}
				switch req.URL.Path {
				case "/v1internal:fetchUserInfo":
					return fixtureResponse(req, 200, "application/json", `{"regionCode":"`+tc.code+`"}`), nil
				case "/v1internal:loadCodeAssist":
					return fixtureResponse(req, 200, "application/json", `{"cloudaicompanionProject":"actual-project"}`), nil
				case "/v1internal:fetchAvailableModels":
					return fixtureResponse(req, 200, "application/json", `{"models":{"gemini-fixture-flash":{}}}`), nil
				default:
					streamCalled++
					return fixtureResponse(req, 200, "text/event-stream", "data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"READY\"}]}}]}}\n\n"), nil
				}
			})}
			result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
			if result.Outcome != "region_blocked" || result.FailurePhase != "fetchUserInfo" || result.Details["google_region"] != tc.code || !strings.Contains(result.ErrorMessage, tc.want) || streamCalled != 0 || result.Details["reply_preview"] != "" {
				t.Fatalf("geofence %s did not stop before model generation: streamCalled=%d %+v", tc.code, streamCalled, result)
			}
		})
	}
}

func TestAntigravityPrefersNewestIDEFlashOverLegacy25(t *testing.T) {
	rule, _ := RuleFor("antigravity")
	t.Run("prefers gemini-3.8-flash-low and records full catalog", func(t *testing.T) {
		checker := Checker{AntigravityToken: "test-secret", ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "www.cloudflare.com" {
				return fixtureResponse(req, 200, "text/plain", "ip=203.0.113.8\nloc=JP\ncolo=NRT\n"), nil
			}
			switch req.URL.Path {
			case "/v1internal:fetchUserInfo":
				return fixtureResponse(req, 200, "application/json", `{"regionCode":"JP"}`), nil
			case "/v1internal:loadCodeAssist":
				return fixtureResponse(req, 200, "application/json", `{"cloudaicompanionProject":"actual-project"}`), nil
			case "/v1internal:fetchAvailableModels":
				return fixtureResponse(req, 200, "application/json", `{"models":{"gemini-2.5-flash":{},"gemini-3.8-flash-low":{},"gemini-4-neon":{}}}`), nil
			default:
				return fixtureResponse(req, 200, "text/event-stream", "data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"READY\"}]}}]}}\n\n"), nil
			}
		})}
		result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
		if result.Outcome != "matched" || result.Model != "gemini-3.8-flash-low" ||
			result.Details["available_flash_models"] != "gemini-3.8-flash-low, gemini-2.5-flash" ||
			result.Details["all_model_ids"] != "gemini-2.5-flash, gemini-3.8-flash-low, gemini-4-neon" {
			t.Fatalf("expected gemini-3.8-flash-low and sorted catalogs, got %+v", result)
		}
	})
	t.Run("prefers newer major.minor flash descending", func(t *testing.T) {
		models, allIDs := selectAntigravityFlashModels(map[string]json.RawMessage{
			"gemini-2.5-flash":      {},
			"gemini-2.0-flash-lite": {},
			"gemini-3.9-flash":      {},
			"claude-opus-5.5":       {},
		})
		if len(models) != 3 || models[0] != "gemini-3.9-flash" || models[1] != "gemini-2.5-flash" || models[2] != "gemini-2.0-flash-lite" {
			t.Fatalf("unexpected flash ordering: %v", models)
		}
		if len(allIDs) != 4 {
			t.Fatalf("unexpected allIDs: %v", allIDs)
		}
	})
}

func TestAntigravityMultiSampleReusesAccountDiscoveryAndTracksBlockedIPs(t *testing.T) {
	rule, _ := RuleFor("antigravity")
	var loadCalls, modelCalls, geoCalls, streamCalls atomic.Int32
	checker := Checker{DriftSamples: 4, AntigravityToken: "test-secret", ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "www.cloudflare.com" {
			// Alternate exit IP: round 1 & 3 JP, round 2 CN, round 4 JP
			n := geoCalls.Load()
			if n == 2 {
				return fixtureResponse(req, 200, "text/plain", "ip=198.51.100.2\nloc=CN\ncolo=SZX\n"), nil
			}
			return fixtureResponse(req, 200, "text/plain", "ip=203.0.113.8\nloc=JP\ncolo=NRT\n"), nil
		}
		switch req.URL.Path {
		case "/v1internal:loadCodeAssist":
			loadCalls.Add(1)
			return fixtureResponse(req, 200, "application/json", `{"cloudaicompanionProject":"actual-project"}`), nil
		case "/v1internal:fetchAvailableModels":
			modelCalls.Add(1)
			return fixtureResponse(req, 200, "application/json", `{"models":{"gemini-3.8-flash-low":{}}}`), nil
		case "/v1internal:fetchUserInfo":
			n := geoCalls.Add(1)
			if n == 2 {
				return fixtureResponse(req, 200, "application/json", `{"regionCode":"CN"}`), nil
			}
			return fixtureResponse(req, 200, "application/json", `{"regionCode":"JP"}`), nil
		case "/v1internal:streamGenerateContent":
			streamCalls.Add(1)
			return fixtureResponse(req, 200, "text/event-stream", "data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"READY\"}]}}]}}\n\n"), nil
		default:
			return fixtureResponse(req, 404, "text/plain", "not found"), nil
		}
	})}
	result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, 10*time.Second)
	if result.Outcome != "flapping" || result.Details["sample_success"] != "3" || result.Details["sample_failure"] != "1" {
		t.Fatalf("expected flapping 3/4, got %+v", result)
	}
	if loadCalls.Load() != 1 || modelCalls.Load() != 1 || geoCalls.Load() != 4 || streamCalls.Load() != 3 {
		t.Fatalf("unexpected call counts: load=%d model=%d geo=%d stream=%d", loadCalls.Load(), modelCalls.Load(), geoCalls.Load(), streamCalls.Load())
	}
	if result.Details["blocked_ips"] != "198.51.100.2" || result.Details["blocked_regions"] != "CN" || result.Details["google_ttfb_ms"] == "" {
		t.Fatalf("missing blocked IP / TTFB details: %+v", result.Details)
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
