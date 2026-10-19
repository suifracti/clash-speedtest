package publicservice

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
)

const ippureFixtureIP = "8.8.8.8"

func ippureJSONResponse(request *http.Request, status int, body string) *http.Response {
	return fixtureResponse(request, status, "application/json", body)
}

func ippureChecker(run roundTripFunc) Checker {
	return Checker{ClientFactory: func(monitor.MonitoredNode, time.Duration) (*http.Client, error) {
		return &http.Client{Transport: roundTripFunc(run), Timeout: 3 * time.Second}, nil
	}}
}

func historicalIPPureFixtureRule() Rule {
	rule, _ := historicalIPPureWebRule()
	rule.RuleVersion = 2
	rule.TargetURL = "https://my.ippure.com/v1/info"
	rule.MaximumBodyBytes = 8 * 1024
	return rule
}

func ippureSignedTimestamp(t *testing.T, request *http.Request, key, target string) int64 {
	t.Helper()
	if request.Method != http.MethodGet || request.URL.String() != target {
		t.Fatalf("signed request target = %s %s, want GET %s", request.Method, request.URL, target)
	}
	if request.Header.Get("x-k") != key {
		t.Fatalf("x-k = %q, want the in-memory challenge key", request.Header.Get("x-k"))
	}
	parts := strings.SplitN(request.Header.Get("x-t"), "-", 2)
	if len(parts) != 2 {
		t.Fatalf("x-t did not contain timestamp and signature: %q", request.Header.Get("x-t"))
	}
	timestamp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		t.Fatalf("parse request timestamp: %v", err)
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte("GET-" + target + "--" + parts[0]))
	want := parts[0] + "-" + hex.EncodeToString(mac.Sum(nil))
	if request.Header.Get("x-t") != want {
		t.Fatalf("x-t signature = %q, want %q", request.Header.Get("x-t"), want)
	}
	return timestamp
}

func TestIPPureWebUsesBoundedNonceHandshakeAndKeepsZeroRisk(t *testing.T) {
	rule, ok := historicalIPPureWebRule()
	if !ok {
		t.Fatal("missing IPPure rule")
	}
	const key = "ephemeral-fixture-key"
	basicURL := "https://api.123169.xyz/api/info/ip-basic/" + ippureFixtureIP
	riskURL := "https://api.123169.xyz/api/info/ip-risk/" + ippureFixtureIP
	serverTime := time.Now().Add(2 * time.Minute).UnixMilli()
	var calls int
	var basicTimestamp, riskTimestamp int64
	checker := ippureChecker(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.Header.Get("Accept") != "application/json" {
			t.Fatalf("Accept = %q, want application/json", request.Header.Get("Accept"))
		}
		for _, header := range []string{"Authorization", "Cookie", "Proxy-Authorization"} {
			if request.Header.Get(header) != "" {
				t.Fatalf("unexpected credential header %s", header)
			}
		}
		switch calls {
		case 1:
			if request.URL.String() != "https://api.ipify.org?format=json" {
				t.Fatalf("fresh-IP request URL = %s", request.URL)
			}
			return ippureJSONResponse(request, http.StatusOK, `{"ip":"`+ippureFixtureIP+`"}`), nil
		case 2:
			if request.URL.String() != basicURL || request.Header.Get("x-k") != "" || request.Header.Get("x-t") != "" {
				t.Fatalf("first basic request was not unsigned: %#v %s", request.Header, request.URL)
			}
			response := ippureJSONResponse(request, http.StatusOK, `{"ok":false,"message":"signature required"}`)
			response.Header.Set("x-k", key)
			response.Header.Set("x-t", strconv.FormatInt(serverTime, 10))
			return response, nil
		case 3:
			basicTimestamp = ippureSignedTimestamp(t, request, key, basicURL)
			return ippureJSONResponse(request, http.StatusOK, `{"ok":true,"data":{"ip":"8.8.8.8","geo":{"country":"United States","country_code":"US","city":"Mountain View","region":"California"},"asn":{"number":15169,"organization":"Google LLC"},"is_residential":false,"is_broadcast":false}}`), nil
		case 4:
			riskTimestamp = ippureSignedTimestamp(t, request, key, riskURL)
			return ippureJSONResponse(request, http.StatusOK, `{"ok":true,"data":{"ip":"8.8.8.8","risk_score":0}}`), nil
		default:
			t.Fatalf("unexpected extra IPPure request: %s", request.URL)
			return nil, nil
		}
	})

	result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	if result.Outcome != "profiled" || result.RequestCount != 4 || calls != 4 {
		t.Fatalf("result=%+v calls=%d, want one ipify, one challenge, one replay, and one risk request", result, calls)
	}
	if basicTimestamp < serverTime || riskTimestamp < basicTimestamp || riskTimestamp-serverTime > 1000 {
		t.Fatalf("signed timestamps did not use the bounded server clock offset: basic=%d risk=%d server=%d", basicTimestamp, riskTimestamp, serverTime)
	}
	for key, value := range map[string]string{
		"ip": ippureFixtureIP, "queried_ip": ippureFixtureIP, "ip_family": "IPv4",
		"provider": "IPPure", "api_method": "ippure_web_basic_risk_v1", "api_version": "web_v1",
		"fraud_score": "0", "risk_scale_max": "100", "risk_direction": "lower_is_better",
		"country": "United States", "country_code": "US", "city": "Mountain View", "region": "California",
		"asn": "AS15169", "organization": "Google LLC", "is_residential": "false", "is_broadcast": "false",
	} {
		if result.Details[key] != value {
			t.Errorf("Details[%q] = %q, want %q; all details: %#v", key, result.Details[key], value, result.Details)
		}
	}
	if !strings.Contains(result.Summary, "0/100") || !strings.Contains(result.Summary, "越低越好") {
		t.Fatalf("summary did not preserve IPPure zero risk scale: %q", result.Summary)
	}
	if result.Details["origin_type"] != "" {
		t.Fatalf("is_broadcast=false was incorrectly inferred to mean native IP: %+v", result.Details)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{key, "x-k", "x-t", "GET-" + basicURL} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("serialized result leaked request secret/header data %q: %s", secret, encoded)
		}
	}
}

func TestIPPureWebRequiresMatchingIPv4AndExplicitRiskScore(t *testing.T) {
	for _, test := range []struct {
		name      string
		ipify     string
		basicData string
		riskBody  string
		wantCalls int
	}{
		{name: "missing risk score", ipify: ippureFixtureIP, basicData: `{"ip":"8.8.8.8","country":"US"}`, riskBody: `{"ok":true,"data":{"ip":"8.8.8.8"}}`, wantCalls: 3},
		{name: "IPv6 observation", ipify: "2001:4860:4860::8888", wantCalls: 1},
		{name: "basic target mismatch", ipify: ippureFixtureIP, basicData: `{"ip":"1.1.1.1"}`, wantCalls: 2},
		{name: "basic IPv6 address", ipify: ippureFixtureIP, basicData: `{"ip":"2001:4860:4860::8888"}`, wantCalls: 2},
		{name: "risk target mismatch", ipify: ippureFixtureIP, basicData: `{"ip":"8.8.8.8"}`, riskBody: `{"ok":true,"data":{"ip":"1.1.1.1","risk_score":0}}`, wantCalls: 3},
		{name: "malformed basic data", ipify: ippureFixtureIP, basicData: `"not-an-object"`, wantCalls: 2},
		{name: "malformed risk JSON", ipify: ippureFixtureIP, basicData: `{"ip":"8.8.8.8"}`, riskBody: `not-json`, wantCalls: 3},
		{name: "null risk score", ipify: ippureFixtureIP, basicData: `{"ip":"8.8.8.8"}`, riskBody: `{"ok":true,"data":{"risk_score":null}}`, wantCalls: 3},
		{name: "non-integer risk score", ipify: ippureFixtureIP, basicData: `{"ip":"8.8.8.8"}`, riskBody: `{"ok":true,"data":{"risk_score":12.5}}`, wantCalls: 3},
		{name: "out of range risk score", ipify: ippureFixtureIP, basicData: `{"ip":"8.8.8.8"}`, riskBody: `{"ok":true,"data":{"risk_score":101}}`, wantCalls: 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			rule, _ := historicalIPPureWebRule()
			var calls int
			checker := ippureChecker(func(request *http.Request) (*http.Response, error) {
				calls++
				switch request.URL.Host {
				case "api.ipify.org":
					return ippureJSONResponse(request, http.StatusOK, `{"ip":"`+test.ipify+`"}`), nil
				case "api.123169.xyz":
					if strings.HasPrefix(request.URL.Path, "/api/info/ip-basic/") {
						return ippureJSONResponse(request, http.StatusOK, `{"ok":true,"data":`+test.basicData+`}`), nil
					}
					if strings.HasPrefix(request.URL.Path, "/api/info/ip-risk/") {
						body := test.riskBody
						if body == "" {
							body = `{"ok":true,"data":{"ip":"8.8.8.8","risk_score":5}}`
						}
						return ippureJSONResponse(request, http.StatusOK, body), nil
					}
				}
				t.Fatalf("unexpected target %s", request.URL)
				return nil, nil
			})
			result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
			if result.Outcome != "unknown" || calls != test.wantCalls {
				t.Fatalf("result=%+v calls=%d, want unknown and %d requests", result, calls, test.wantCalls)
			}
			if result.Details["fraud_score"] != "" {
				t.Fatalf("invalid/missing risk score became a profile: %+v", result)
			}
			if result.Details["is_residential"] != "" || result.Details["ip_type"] != "" {
				t.Fatalf("missing residential evidence became a false value: %+v", result.Details)
			}
		})
	}
}

func TestIPPureWebStopsOnHTTPRejectionAndRepeatedNonce(t *testing.T) {
	rule, _ := historicalIPPureWebRule()
	t.Run("403 without nonce", func(t *testing.T) {
		var calls int
		checker := ippureChecker(func(request *http.Request) (*http.Response, error) {
			calls++
			if request.URL.Host == "api.ipify.org" {
				return ippureJSONResponse(request, http.StatusOK, `{"ip":"8.8.8.8"}`), nil
			}
			return fixtureResponse(request, http.StatusForbidden, "text/html", "forbidden"), nil
		})
		result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
		if result.Outcome != "http_rejected" || result.HTTPStatus == nil || *result.HTTPStatus != http.StatusForbidden || calls != 2 {
			t.Fatalf("403 result=%+v calls=%d", result, calls)
		}
	})

	t.Run("403 HTML with nonce does not replay", func(t *testing.T) {
		var calls int
		checker := ippureChecker(func(request *http.Request) (*http.Response, error) {
			calls++
			if request.URL.Host == "api.ipify.org" {
				return ippureJSONResponse(request, http.StatusOK, `{"ip":"8.8.8.8"}`), nil
			}
			response := fixtureResponse(request, http.StatusForbidden, "text/html", "challenge page")
			response.Header.Set("x-k", "challenge-key")
			response.Header.Set("x-t", strconv.FormatInt(time.Now().UnixMilli(), 10))
			return response, nil
		})
		result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
		if result.Outcome != "http_rejected" || result.HTTPStatus == nil || *result.HTTPStatus != http.StatusForbidden || calls != 2 || result.RequestCount != 2 {
			t.Fatalf("403 HTML nonce result=%+v calls=%d", result, calls)
		}
	})

	t.Run("JSON nonce with Cloudflare challenge marker does not replay", func(t *testing.T) {
		var calls int
		checker := ippureChecker(func(request *http.Request) (*http.Response, error) {
			calls++
			if request.URL.Host == "api.ipify.org" {
				return ippureJSONResponse(request, http.StatusOK, `{"ip":"8.8.8.8"}`), nil
			}
			response := ippureJSONResponse(request, http.StatusForbidden, `{"ok":false}`)
			response.Header.Set("cf-mitigated", "challenge")
			response.Header.Set("x-k", "challenge-key")
			response.Header.Set("x-t", strconv.FormatInt(time.Now().UnixMilli(), 10))
			return response, nil
		})
		result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
		if result.Outcome != "challenge" || result.HTTPStatus == nil || *result.HTTPStatus != http.StatusForbidden || calls != 2 || result.RequestCount != 2 {
			t.Fatalf("Cloudflare marked JSON nonce result=%+v calls=%d", result, calls)
		}
	})

	t.Run("one replay then challenge stops", func(t *testing.T) {
		const key = "single-key"
		serverTime := time.Now().UnixMilli()
		var calls int
		checker := ippureChecker(func(request *http.Request) (*http.Response, error) {
			calls++
			if request.URL.Host == "api.ipify.org" {
				return ippureJSONResponse(request, http.StatusOK, `{"ip":"8.8.8.8"}`), nil
			}
			if calls == 2 {
				response := ippureJSONResponse(request, http.StatusOK, `{"ok":false,"message":"signature required"}`)
				response.Header.Set("x-k", key)
				response.Header.Set("x-t", strconv.FormatInt(serverTime, 10))
				return response, nil
			}
			response := fixtureResponse(request, http.StatusForbidden, "text/html", "challenge")
			response.Header.Set("x-k", key)
			response.Header.Set("x-t", strconv.FormatInt(serverTime, 10))
			return response, nil
		})
		result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
		if result.Outcome != "challenge" || result.RequestCount != 3 || calls != 3 {
			t.Fatalf("repeat challenge result=%+v calls=%d", result, calls)
		}
	})

	t.Run("429 stops even when nonce is present", func(t *testing.T) {
		serverTime := time.Now().UnixMilli()
		var calls int
		checker := ippureChecker(func(request *http.Request) (*http.Response, error) {
			calls++
			if request.URL.Host == "api.ipify.org" {
				return ippureJSONResponse(request, http.StatusOK, `{"ip":"8.8.8.8"}`), nil
			}
			response := fixtureResponse(request, http.StatusTooManyRequests, "application/json", `{"ok":false}`)
			response.Header.Set("x-k", "do-not-replay")
			response.Header.Set("x-t", strconv.FormatInt(serverTime, 10))
			return response, nil
		})
		result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
		if result.Outcome != "rate_limited" || result.RequestCount != 2 || calls != 2 {
			t.Fatalf("429 result=%+v calls=%d", result, calls)
		}
	})
}

func TestIPPureWebStopsAfterIPifyFailure(t *testing.T) {
	rule, _ := historicalIPPureWebRule()
	var calls int
	checker := ippureChecker(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.URL.String() != ippureWebIPifyURL {
			t.Fatalf("IPPure query continued after IPify failed: %s", request.URL)
		}
		return ippureJSONResponse(request, http.StatusServiceUnavailable, `{"error":"offline"}`), nil
	})
	result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	if result.Outcome != "unknown" || result.RequestCount != 1 || calls != 1 {
		t.Fatalf("IPify failure result=%+v calls=%d", result, calls)
	}
	if result.Details["api_method"] != "ippure_web_basic_risk_v1" || result.Details["queried_ip"] != "" {
		t.Fatalf("failed IP observation lost method provenance or invented queried IP: %+v", result.Details)
	}
}

func TestIPPureWebInvalidNonceClockDoesNotReplay(t *testing.T) {
	rule, _ := historicalIPPureWebRule()
	var calls int
	checker := ippureChecker(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.URL.Host == "api.ipify.org" {
			return ippureJSONResponse(request, http.StatusOK, `{"ip":"8.8.8.8"}`), nil
		}
		response := fixtureResponse(request, http.StatusForbidden, "text/html", "challenge")
		response.Header.Set("x-k", "secret-key")
		response.Header.Set("x-t", "999999999999999999999999999999")
		return response, nil
	})
	result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	if result.Outcome != "unknown" || result.RequestCount != 2 || calls != 2 {
		t.Fatalf("invalid server time was used: result=%+v calls=%d", result, calls)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "secret-key") {
		t.Fatalf("invalid nonce key leaked into result: %s", encoded)
	}
}

func TestIPPureWebHonorsCancellation(t *testing.T) {
	rule, _ := historicalIPPureWebRule()
	started := make(chan struct{})
	var calls int
	checker := ippureChecker(func(request *http.Request) (*http.Response, error) {
		calls++
		close(started)
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	completed := make(chan Result, 1)
	go func() { completed <- checker.Check(ctx, monitor.MonitoredNode{}, rule, DefaultTimeout) }()
	<-started
	cancel()
	result := <-completed
	if result.Outcome != "cancelled" || calls != 1 {
		t.Fatalf("cancel result=%+v calls=%d", result, calls)
	}
}

func historicalIPPureWebRule() (Rule, bool) {
	rule, ok := RuleFor("ippure_ip_quality")
	rule.RuleVersion = 3
	return rule, ok
}

func TestIPPureRuleIsRestoredForManualUseOnly(t *testing.T) {
	rule, ok := RuleFor("ippure_ip_quality")
	if !ok || rule.RuleVersion != 5 || rule.BatchDefault || rule.Name != "IPPure IPv4风险画像" {
		t.Fatalf("IPPure rule metadata = %+v, want v5, on-demand, and IPv4 risk profile", rule)
	}
	for _, listed := range Catalog() {
		if listed.ServiceID == rule.ServiceID {
			return
		}
	}
	t.Fatal("IPPure rule is missing from the restored catalog")
}

func TestIPPureOfficialFallbackAfterBasicChallenge(t *testing.T) {
	for _, tc := range []struct{ name, body, outcome string }{
		{"matching IPv4 and zero", `{"ip":"8.8.8.8","fraudScore":0,"country":"United States","asn":15169}`, "profiled"},
		{"different IPv4", `{"ip":"1.1.1.1","fraudScore":20}`, "unknown"},
		{"IPv6 cannot supply IPv4 risk", `{"ip":"2606:4700::1111","fraudScore":20}`, "unknown"},
		{"missing score", `{"ip":"8.8.8.8"}`, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			checker := ippureChecker(func(req *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return ippureJSONResponse(req, 200, `{"ip":"8.8.8.8"}`), nil
				}
				if calls == 2 {
					r := ippureJSONResponse(req, 403, `{"ok":false}`)
					r.Header.Set("cf-mitigated", "challenge")
					return r, nil
				}
				if calls != 3 || req.URL.String() != "https://my.ippure.com/v1/info" {
					t.Fatalf("unexpected request %d: %s", calls, req.URL)
				}
				if !strings.HasPrefix(req.Header.Get("User-Agent"), "Clash-SpeedTest-Pro/") {
					t.Fatal("official fallback must identify the app")
				}
				return ippureJSONResponse(req, 200, tc.body), nil
			})
			rule, _ := RuleFor("ippure_ip_quality")
			rule.RuleVersion = 4
			r := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
			if r.Outcome != tc.outcome || calls != 3 || r.RequestCount != 3 {
				t.Fatalf("outcome=%s calls=%d result=%+v", r.Outcome, calls, r)
			}
			if r.Details["web_basic_outcome"] != "challenge" {
				t.Fatal("lost original challenge evidence")
			}
			if tc.outcome == "profiled" && (r.Details["fraud_score"] != "0" || r.Details["risk_scale_max"] != "100") {
				t.Fatalf("zero score lost: %+v", r.Details)
			}
		})
	}
}

func TestIPPureUnmarkedForbiddenUsesOfficialAPIOnlyForNewRule(t *testing.T) {
	for _, version := range []int{4, 5} {
		for _, contentType := range []string{"text/html", "application/json"} {
			t.Run(fmt.Sprintf("v%d_%s", version, contentType), func(t *testing.T) {
				calls := 0
				checker := ippureChecker(func(req *http.Request) (*http.Response, error) {
					calls++
					switch calls {
					case 1:
						return ippureJSONResponse(req, 200, `{"ip":"8.8.8.8"}`), nil
					case 2:
						return fixtureResponse(req, 403, contentType, `{"error":"forbidden"}`), nil
					case 3:
						if req.URL.String() != "https://my.ippure.com/v1/info" {
							t.Fatalf("unexpected fallback %s", req.URL)
						}
						return ippureJSONResponse(req, 200, `{"ip":"8.8.8.8","fraudScore":61}`), nil
					default:
						t.Fatal("unexpected extra request")
						return nil, nil
					}
				})
				rule, _ := RuleFor("ippure_ip_quality")
				rule.RuleVersion = version
				result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
				if version == 4 {
					if result.Outcome != "http_rejected" || calls != 2 {
						t.Fatalf("old rule changed: %+v calls=%d", result, calls)
					}
					return
				}
				if result.Outcome != "profiled" || calls != 3 || result.RequestCount != 3 || result.Details["fraud_score"] != "61" || result.Details["web_basic_http_status"] != "403" || result.Details["web_basic_outcome"] != "http_rejected" {
					t.Fatalf("missing plain 403 fallback: %+v calls=%d", result, calls)
				}
			})
		}
	}
}
