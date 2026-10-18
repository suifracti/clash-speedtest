package publicservice

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type contextBody struct {
	ctx     context.Context
	started chan struct{}
	once    sync.Once
}

func (b *contextBody) Read([]byte) (int, error) {
	b.once.Do(func() { close(b.started) })
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}

func (*contextBody) Close() error { return nil }

type networkTimeoutError struct{}

func (networkTimeoutError) Error() string   { return "controlled network timeout" }
func (networkTimeoutError) Timeout() bool   { return true }
func (networkTimeoutError) Temporary() bool { return true }

type timeoutBody struct{}

func (timeoutBody) Read([]byte) (int, error) { return 0, networkTimeoutError{} }
func (timeoutBody) Close() error             { return nil }

func fixtureClient(run roundTripFunc) ClientFactory {
	return func(monitor.MonitoredNode, time.Duration) (*http.Client, error) {
		return &http.Client{Transport: roundTripFunc(run)}, nil
	}
}

func fixtureResponse(request *http.Request, status int, contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

func TestCheckerUsesFixedRulesAndNeverSendsBrowserCredentials(t *testing.T) {
	cloudflare, _ := RuleFor("cloudflare_204")
	var calls atomic.Int32
	checker := Checker{ClientFactory: fixtureClient(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		if request.Method != http.MethodGet || request.URL.String() != cloudflare.TargetURL {
			t.Fatalf("request = %s %s", request.Method, request.URL)
		}
		for _, header := range []string{"Authorization", "Cookie", "Proxy-Authorization"} {
			if request.Header.Get(header) != "" {
				t.Fatalf("unexpected sensitive header %s", header)
			}
		}
		return fixtureResponse(request, http.StatusNoContent, "", ""), nil
	})}
	result := checker.Check(context.Background(), monitor.MonitoredNode{NodeKey: "node-a"}, cloudflare, DefaultTimeout)
	if result.Outcome != "matched" || result.HTTPStatus == nil || *result.HTTPStatus != http.StatusNoContent || calls.Load() != 1 {
		t.Fatalf("result = %+v, calls=%d", result, calls.Load())
	}
	if got := len(Catalog()); got < 16 {
		t.Fatalf("catalog length = %d, want expanded catalog", got)
	}
	if _, ok := RuleFor("https://attacker.example"); ok {
		t.Fatal("arbitrary target unexpectedly resolved from the fixed catalog")
	}
}

func TestCheckerRestoresIndependentIPQualityProfiles(t *testing.T) {
	ping0, _ := RuleFor("ping0_ip_quality")
	ippure := historicalIPPureFixtureRule()
	checker := Checker{ClientFactory: fixtureClient(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Host {
		case "ping0.cc":
			return fixtureResponse(request, http.StatusOK, "application/json", `{"ip":"203.0.113.7","location":"Tokyo","country":"Japan","asn":"AS64500","org":"Example Net","isidc":true,"iprisk":17}`), nil
		case "my.ippure.com":
			return fixtureResponse(request, http.StatusOK, "application/json", `{"ip":"203.0.113.7","asn":64500,"asOrganization":"Example Net","country":"Japan","countryCode":"JP","city":"Tokyo","fraudScore":23,"isResidential":false,"isBroadcast":false}`), nil
		default:
			t.Fatalf("unexpected target %s", request.URL)
			return nil, nil
		}
	})}
	ping0Result := checker.Check(context.Background(), monitor.MonitoredNode{}, ping0, DefaultTimeout)
	if ping0Result.Outcome != "profiled" || ping0Result.Details["risk_score"] != "17" || ping0Result.Details["ip_type"] != "机房 IDC" {
		t.Fatalf("unexpected Ping0 result: %+v", ping0Result)
	}
	ippureResult := checker.Check(context.Background(), monitor.MonitoredNode{}, ippure, DefaultTimeout)
	if ippureResult.Outcome != "profiled" || ippureResult.Details["fraud_score"] != "23" || ippureResult.Details["origin_type"] != "原生 IP" {
		t.Fatalf("unexpected IPPure result: %+v", ippureResult)
	}
}

func TestCheckerSeparatesCloudflareChallengeAndNetflixUnlockTier(t *testing.T) {
	grok, _ := RuleFor("grok_web")
	challengeChecker := Checker{ClientFactory: fixtureClient(func(request *http.Request) (*http.Response, error) {
		response := fixtureResponse(request, http.StatusForbidden, "text/html", "challenge")
		response.Header.Set("cf-mitigated", "challenge")
		return response, nil
	})}
	if result := challengeChecker.Check(context.Background(), monitor.MonitoredNode{}, grok, DefaultTimeout); result.Outcome != "challenge" {
		t.Fatalf("expected challenge classification, got %+v", result)
	}

	netflix, _ := RuleFor("netflix_unlock")
	netflixChecker := Checker{ClientFactory: fixtureClient(func(request *http.Request) (*http.Response, error) {
		if strings.HasSuffix(request.URL.Path, "/81280792") {
			return fixtureResponse(request, http.StatusOK, "text/html", `<h1 class="title-title">LEGO Ninjago</h1>`), nil
		}
		return fixtureResponse(request, http.StatusNotFound, "text/html", "Oh no!"), nil
	})}
	client, _ := netflixChecker.ClientFactory(monitor.MonitoredNode{}, DefaultTimeout)
	result := netflixChecker.checkOnce(context.Background(), client, monitor.MonitoredNode{}, netflix, time.Now())
	if result.Outcome != "originals_only" || result.RequestCount != 2 {
		t.Fatalf("expected originals-only tier from independent fixtures, got %+v", result)
	}
}

func TestCheckerClassifiesRegionalStreamingEvidence(t *testing.T) {
	abema, _ := RuleFor("abema_unlock")
	fod, _ := RuleFor("fod_unlock")
	checker := Checker{ClientFactory: fixtureClient(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Host {
		case "api.abema.io":
			return fixtureResponse(request, http.StatusOK, "application/json", `{"isoCountryCode":"JP"}`), nil
		case "geocontrol1.stream.ne.jp":
			return fixtureResponse(request, http.StatusOK, "application/xml", `<RESULT><FLAG TYPE="false" /></RESULT>`), nil
		default:
			t.Fatalf("unexpected regional target %s", request.URL)
			return nil, nil
		}
	})}
	abemaClient, _ := checker.ClientFactory(monitor.MonitoredNode{}, DefaultTimeout)
	if result := checker.checkOnce(context.Background(), abemaClient, monitor.MonitoredNode{}, abema, time.Now()); result.Outcome != "unlocked" || result.Details["service_region"] != "JP" {
		t.Fatalf("unexpected ABEMA result: %+v", result)
	}
	if result := checker.Check(context.Background(), monitor.MonitoredNode{}, fod, DefaultTimeout); result.Outcome != "region_blocked" {
		t.Fatalf("unexpected FOD result: %+v", result)
	}
}

func TestCheckerAntigravityRule(t *testing.T) {
	antigravity, ok := RuleFor("antigravity")
	if !ok {
		t.Fatal("antigravity rule not found in catalog")
	}

	// An authentication response is not evidence of regional availability.
	checkerOk := Checker{AntigravityToken: "test-token", ClientFactory: fixtureClient(func(request *http.Request) (*http.Response, error) {
		return fixtureResponse(request, http.StatusUnauthorized, "application/json", `{"error":{"code":401,"status":"UNAUTHENTICATED"}}`), nil
	})}
	resOk := checkerOk.Check(context.Background(), monitor.MonitoredNode{}, antigravity, DefaultTimeout)
	if resOk.Outcome != "auth_failed" {
		t.Fatalf("authentication must not claim regional availability, got: %+v", resOk)
	}

	// 2. Blocked with location error
	checkerBlocked := Checker{AntigravityToken: "test-token", ClientFactory: fixtureClient(func(request *http.Request) (*http.Response, error) {
		return fixtureResponse(request, http.StatusBadRequest, "application/json", `{"error":{"message":"User location is not supported"}}`), nil
	})}
	resBlocked := checkerBlocked.Check(context.Background(), monitor.MonitoredNode{}, antigravity, DefaultTimeout)
	if resBlocked.Outcome != "region_blocked" {
		t.Fatalf("expected region_blocked for Antigravity, got: %+v", resBlocked)
	}
	for _, status := range []int{http.StatusOK, http.StatusForbidden} {
		t.Run(fmt.Sprintf("read-timeout-%d", status), func(t *testing.T) {
			checker := Checker{AntigravityToken: "test-token", ClientFactory: fixtureClient(func(request *http.Request) (*http.Response, error) {
				response := fixtureResponse(request, status, "application/json", "")
				response.Body = timeoutBody{}
				return response, nil
			})}
			result := checker.Check(context.Background(), monitor.MonitoredNode{}, antigravity, DefaultTimeout)
			if result.Outcome != "timed_out" {
				t.Fatalf("incomplete response must retain timeout, got: %+v", result)
			}
		})
	}
}

func TestCheckerSeparatesGitHubRootRuleAndHTTPOutcomes(t *testing.T) {
	github, _ := RuleFor("github_api_root")
	rootJSON := `{"current_user_url":"https://api.github.com/user","repository_url":"https://api.github.com/repos/{owner}/{repo}"}`
	checker := Checker{ClientFactory: fixtureClient(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://api.github.com/" || request.Header.Get("Accept") != github.Accept || request.Header.Get("X-GitHub-Api-Version") != github.APIVersionHeader {
			t.Fatalf("GitHub root request headers/target incorrect: %#v %s", request.Header, request.URL)
		}
		return fixtureResponse(request, http.StatusOK, "application/json; charset=utf-8", rootJSON), nil
	})}
	matched := checker.Check(context.Background(), monitor.MonitoredNode{}, github, DefaultTimeout)
	if matched.Outcome != "matched" || matched.BytesRead != int64(len(rootJSON)) {
		t.Fatalf("GitHub root result = %+v", matched)
	}

	var redirectCalls atomic.Int32
	google, _ := RuleFor("google_204")
	redirectChecker := Checker{ClientFactory: fixtureClient(func(request *http.Request) (*http.Response, error) {
		redirectCalls.Add(1)
		return fixtureResponse(request, http.StatusFound, "text/plain", "redirect body"), nil
	})}
	redirect := redirectChecker.Check(context.Background(), monitor.MonitoredNode{}, google, DefaultTimeout)
	if redirect.Outcome != "redirect" || redirect.HTTPStatus == nil || *redirect.HTTPStatus != http.StatusFound || redirectCalls.Load() != 1 {
		t.Fatalf("redirect result = %+v, calls=%d", redirect, redirectCalls.Load())
	}

	rateLimitedChecker := Checker{ClientFactory: fixtureClient(func(request *http.Request) (*http.Response, error) {
		return fixtureResponse(request, http.StatusTooManyRequests, "application/json", `{"message":"rate limit"}`), nil
	})}
	rateLimited := rateLimitedChecker.Check(context.Background(), monitor.MonitoredNode{}, github, DefaultTimeout)
	if rateLimited.Outcome != "rate_limited" || rateLimited.BytesRead != 0 {
		t.Fatalf("rate limit result = %+v", rateLimited)
	}
	forbiddenChecker := Checker{ClientFactory: fixtureClient(func(request *http.Request) (*http.Response, error) {
		return fixtureResponse(request, http.StatusForbidden, "application/json", `{"message":"forbidden"}`), nil
	})}
	forbidden := forbiddenChecker.Check(context.Background(), monitor.MonitoredNode{}, github, DefaultTimeout)
	if forbidden.Outcome != "http_rejected" || forbidden.HTTPStatus == nil || *forbidden.HTTPStatus != http.StatusForbidden {
		t.Fatalf("forbidden response = %+v", forbidden)
	}
	rateLimitedForbiddenChecker := Checker{ClientFactory: fixtureClient(func(request *http.Request) (*http.Response, error) {
		response := fixtureResponse(request, http.StatusForbidden, "application/json", `{"message":"secondary rate limit"}`)
		response.Header.Set("X-RateLimit-Remaining", "0")
		return response, nil
	})}
	rateLimitedForbidden := rateLimitedForbiddenChecker.Check(context.Background(), monitor.MonitoredNode{}, github, DefaultTimeout)
	if rateLimitedForbidden.Outcome != "rate_limited" || rateLimitedForbidden.HTTPStatus == nil || *rateLimitedForbidden.HTTPStatus != http.StatusForbidden {
		t.Fatalf("rate-limited forbidden response = %+v", rateLimitedForbidden)
	}

	badJSONChecker := Checker{ClientFactory: fixtureClient(func(request *http.Request) (*http.Response, error) {
		return fixtureResponse(request, http.StatusOK, "text/html", "<html>login</html>"), nil
	})}
	badJSON := badJSONChecker.Check(context.Background(), monitor.MonitoredNode{}, github, DefaultTimeout)
	if badJSON.Outcome != "criteria_mismatch" || badJSON.FailurePhase != "content_type" {
		t.Fatalf("content type mismatch = %+v", badJSON)
	}
	transportChecker := Checker{ClientFactory: fixtureClient(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("Authorization: Bearer fixture-secret")
	})}
	transport := transportChecker.Check(context.Background(), monitor.MonitoredNode{}, github, DefaultTimeout)
	if transport.Outcome != "transport_error" || strings.Contains(transport.ErrorMessage, "fixture-secret") {
		t.Fatalf("transport failure was not safely categorized: %+v", transport)
	}
}

func TestCheckerCapsBodyReadsAndDistinguishesTimeoutFromCancellation(t *testing.T) {
	github, _ := RuleFor("github_api_root")
	largeBody := strings.Repeat(" ", MaximumResponseBytes+4096)
	largeChecker := Checker{ClientFactory: fixtureClient(func(request *http.Request) (*http.Response, error) {
		return fixtureResponse(request, http.StatusOK, "application/json", largeBody), nil
	})}
	large := largeChecker.Check(context.Background(), monitor.MonitoredNode{}, github, DefaultTimeout)
	if large.BytesRead != MaximumResponseBytes || large.Outcome != "criteria_mismatch" || large.FailurePhase != "response_limit" {
		t.Fatalf("bounded read result = %+v", large)
	}

	exactBody := `{"current_user_url":"https://api.github.com/user","repository_url":"https://api.github.com/repos/a/b","pad":"` + strings.Repeat("a", MaximumResponseBytes-len(`{"current_user_url":"https://api.github.com/user","repository_url":"https://api.github.com/repos/a/b","pad":""}`)) + `"}`
	if len(exactBody) != MaximumResponseBytes {
		t.Fatalf("exactBody len = %d, want %d", len(exactBody), MaximumResponseBytes)
	}
	exactChecker := Checker{ClientFactory: fixtureClient(func(request *http.Request) (*http.Response, error) {
		return fixtureResponse(request, http.StatusOK, "application/json", exactBody), nil
	})}
	exact := exactChecker.Check(context.Background(), monitor.MonitoredNode{}, github, DefaultTimeout)
	if exact.Outcome != "matched" {
		t.Fatalf("exact 64KiB response should be matched, got %+v", exact)
	}

	waitingRequest := make(chan struct{}, 1)
	blockedChecker := Checker{ClientFactory: fixtureClient(func(request *http.Request) (*http.Response, error) {
		waitingRequest <- struct{}{}
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	ctx, cancel := context.WithCancel(context.Background())
	completed := make(chan Result, 1)
	go func() { completed <- blockedChecker.Check(ctx, monitor.MonitoredNode{}, github, time.Minute) }()
	<-waitingRequest
	cancel()
	if result := <-completed; result.Outcome != "cancelled" {
		t.Fatalf("cancel result = %+v", result)
	}

	timeoutChecker := Checker{ClientFactory: fixtureClient(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	timedOut := timeoutChecker.Check(context.Background(), monitor.MonitoredNode{}, github, 15*time.Millisecond)
	if timedOut.Outcome != "timed_out" {
		t.Fatalf("timeout result = %+v, err=%s", timedOut, fmt.Sprint(timedOut.ErrorMessage))
	}

	bodyTimeoutStarted := make(chan struct{})
	bodyTimeoutChecker := Checker{ClientFactory: fixtureClient(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       &contextBody{ctx: request.Context(), started: bodyTimeoutStarted},
			Request:    request,
		}, nil
	})}
	bodyTimeoutResults := make(chan Result, 1)
	go func() {
		bodyTimeoutResults <- bodyTimeoutChecker.Check(context.Background(), monitor.MonitoredNode{}, github, 15*time.Millisecond)
	}()
	<-bodyTimeoutStarted
	if result := <-bodyTimeoutResults; result.Outcome != "timed_out" || result.FailurePhase != "timeout" {
		t.Fatalf("body timeout result = %+v", result)
	}

	bodyCancelStarted := make(chan struct{})
	bodyCancelChecker := Checker{ClientFactory: fixtureClient(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       &contextBody{ctx: request.Context(), started: bodyCancelStarted},
			Request:    request,
		}, nil
	})}
	bodyCancelCtx, bodyCancel := context.WithCancel(context.Background())
	bodyCancelResults := make(chan Result, 1)
	go func() {
		bodyCancelResults <- bodyCancelChecker.Check(bodyCancelCtx, monitor.MonitoredNode{}, github, time.Minute)
	}()
	<-bodyCancelStarted
	bodyCancel()
	if result := <-bodyCancelResults; result.Outcome != "cancelled" || result.FailurePhase != "cancelled" {
		t.Fatalf("body cancellation result = %+v", result)
	}

	readTimeoutChecker := Checker{ClientFactory: fixtureClient(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       timeoutBody{},
			Request:    request,
		}, nil
	})}
	if result := readTimeoutChecker.Check(context.Background(), monitor.MonitoredNode{}, github, DefaultTimeout); result.Outcome != "timed_out" || result.FailurePhase != "timeout" {
		t.Fatalf("body network timeout result = %+v", result)
	}
}

func TestCheckerDistinguishesTransportTimeoutFromCheckDeadline(t *testing.T) {
	cloudflare, _ := RuleFor("cloudflare_204")
	github, _ := RuleFor("github_api_root")
	for _, tt := range []struct {
		name string
		err  error
	}{
		{"network timeout before checker deadline", networkTimeoutError{}},
		{"wrapped inner deadline", fmt.Errorf("inner handshake: %w", context.DeadlineExceeded)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			checker := Checker{ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.Context().Err() != nil {
					t.Fatal("fixture checker already expired")
				}
				return nil, tt.err
			})}
			result := checker.Check(context.Background(), monitor.MonitoredNode{}, cloudflare, 15*time.Second)
			if result.Outcome != "timed_out" || result.FailurePhase != "timeout" || result.Details["timeout_origin"] != "transport" || result.ErrorMessage == "请求超过设定超时" || calls != 1 || result.Details["conclusion"] != "unconfirmed" {
				t.Fatalf("inner timeout incorrectly attributed to checker or node verdict: calls=%d result=%+v", calls, result)
			}
		})
	}
	t.Run("body network timeout", func(t *testing.T) {
		checker := Checker{ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: timeoutBody{}, Request: req}, nil
		})}
		result := checker.Check(context.Background(), monitor.MonitoredNode{}, github, 15*time.Second)
		if result.Outcome != "timed_out" || result.Details["timeout_origin"] != "transport" || result.ErrorMessage == "请求超过设定超时" || result.HTTPStatus == nil || *result.HTTPStatus != 200 || result.Details["conclusion"] != "unconfirmed" {
			t.Fatalf("body timeout incorrectly classified: %+v", result)
		}
	})
	for _, body := range []bool{false, true} {
		t.Run(fmt.Sprintf("actual checker deadline body=%t", body), func(t *testing.T) {
			checker := Checker{ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
				if body {
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: &contextBody{ctx: req.Context(), started: make(chan struct{})}, Request: req}, nil
				}
				<-req.Context().Done()
				return nil, req.Context().Err()
			})}
			rule := cloudflare
			if body {
				rule = github
			}
			result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, 15*time.Millisecond)
			if result.Outcome != "timed_out" || result.Details["timeout_origin"] != "checker_deadline" || result.ErrorMessage != "请求超过设定超时" {
				t.Fatalf("actual checker deadline lost: %+v", result)
			}
		})
	}
}

func TestCheckerMultiSampleDriftAndFlappingDetection(t *testing.T) {
	rule, _ := RuleFor("cloudflare_204")

	t.Run("detects flapping when some probes fail and some succeed", func(t *testing.T) {
		var calls atomic.Int32
		checker := Checker{
			DriftSamples: 3,
			ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
				c := calls.Add(1)
				if c == 1 {
					// First probe succeeds
					return fixtureResponse(req, http.StatusNoContent, "", ""), nil
				}
				// A real HTTP rejection, rather than a transport error or server outage.
				return fixtureResponse(req, http.StatusForbidden, "", "forbidden"), nil
			}),
		}
		result := checker.Check(context.Background(), monitor.MonitoredNode{NodeKey: "node-a"}, rule, DefaultTimeout)
		if result.Outcome != "flapping" {
			t.Fatalf("outcome = %s, want flapping", result.Outcome)
		}
		if result.Details["flapping_detected"] != "true" {
			t.Fatalf("flapping_detected = %s, want true", result.Details["flapping_detected"])
		}
		if result.Details["sample_total"] != "3" || result.Details["sample_success"] != "1" || result.Details["sample_failure"] != "2" {
			t.Fatalf("sample counts mismatch: %+v", result.Details)
		}
		if calls.Load() != 3 {
			t.Fatalf("calls = %d, want 3", calls.Load())
		}
	})

	t.Run("detects IP drift across reconnected probes", func(t *testing.T) {
		traceRule, _ := RuleFor("cloudflare_trace")
		var calls atomic.Int32
		checker := Checker{
			DriftSamples: 3,
			ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
				c := calls.Add(1)
				ip := "198.51.100.1"
				colo := "SIN"
				if c > 1 {
					ip = "198.51.100.2"
					colo = "HKG"
				}
				return fixtureResponse(req, http.StatusOK, "text/plain", fmt.Sprintf("ip=%s\ncolo=%s\nloc=SG\n", ip, colo)), nil
			}),
		}
		result := checker.Check(context.Background(), monitor.MonitoredNode{NodeKey: "node-a"}, traceRule, DefaultTimeout)
		if result.Outcome != "profiled" {
			t.Fatalf("outcome = %s, want profiled", result.Outcome)
		}
		if result.Details["ip_drift_detected"] != "true" {
			t.Fatalf("ip_drift_detected = %s, want true", result.Details["ip_drift_detected"])
		}
		if !strings.Contains(result.Details["observed_ips"], "198.51.100.1") || !strings.Contains(result.Details["observed_ips"], "198.51.100.2") {
			t.Fatalf("observed_ips = %s, want both IPs", result.Details["observed_ips"])
		}
	})

	t.Run("solid consistent success marks full pass", func(t *testing.T) {
		var calls atomic.Int32
		checker := Checker{
			DriftSamples: 3,
			ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
				calls.Add(1)
				return fixtureResponse(req, http.StatusNoContent, "", ""), nil
			}),
		}
		result := checker.Check(context.Background(), monitor.MonitoredNode{NodeKey: "node-a"}, rule, DefaultTimeout)
		if result.Outcome != "matched" {
			t.Fatalf("outcome = %s, want matched", result.Outcome)
		}
		if result.Details["flapping_detected"] != "false" || result.Details["sample_success"] != "3" {
			t.Fatalf("result = %+v", result)
		}
	})
}

func TestRestoreIPPureAsOnDemandIPv4Rule(t *testing.T) {
	rule, ok := RuleFor("ippure_ip_quality")
	if !ok || rule.RuleVersion != 3 || rule.BatchDefault || rule.Name != "IPPure IPv4风险画像" {
		t.Fatalf("IPPure rule = %+v, want restored v3 on-demand IPv4 rule", rule)
	}
	for _, rule := range Catalog() {
		if rule.ServiceID == "ippure_ip_quality" && rule.RuleVersion == 3 {
			return
		}
	}
	t.Fatal("restored IPPure rule not found in new catalog")
}
