package publicservice

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
)

func TestCompositeFailureDoesNotReusePreviousHTTPStatus(t *testing.T) {
	for _, service := range []string{"netflix_unlock", "antigravity"} {
		for _, afterHeaders := range []bool{false, true} {
			t.Run(service+map[bool]string{false: "/before_headers", true: "/body"}[afterHeaders], func(t *testing.T) {
				calls := 0
				firstBody := `<html>nflx://www.netflix.com/title/fixture</html>`
				if service == "antigravity" {
					firstBody = `{"cloudaicompanionProject":"fixture-project"}`
				}
				fastBytes := 0
				if service == "netflix_unlock" {
					fastBytes = len(`{"targets":[]}`)
				}
				client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					calls++
					firstCall := 1
					if service == "netflix_unlock" {
						firstCall = 2
						if calls == 1 {
							return fixtureResponse(req, 200, "application/json", `{"targets":[]}`), nil
						}
					}
					if calls == firstCall {
						return fixtureResponse(req, 200, "", firstBody), nil
					}
					if !afterHeaders {
						trace := httptrace.ContextClientTrace(req.Context())
						trace.TLSHandshakeStart()
						trace.TLSHandshakeDone(tls.ConnectionState{}, io.EOF)
						return nil, io.EOF
					}
					response := fixtureResponse(req, 200, "", "")
					response.Body = timeoutBody{}
					return response, nil
				})}
				client.Transport = (&requestPhases{}).wrap(client.Transport, false)
				rule, _ := RuleFor(service)
				var got Result
				if service == "antigravity" {
					got = checkAntigravity(context.Background(), client, "fixture-token", time.Now())
				} else {
					got = (Checker{}).checkOnce(context.Background(), client, monitor.MonitoredNode{}, rule, time.Now())
				}
				wantCalls := 2
				if service == "netflix_unlock" {
					wantCalls = 3
				}
				if calls != wantCalls || got.BytesRead != int64(len(firstBody)+fastBytes) {
					t.Fatalf("prior request evidence lost: calls=%d result=%+v", calls, got)
				}
				if !afterHeaders && (got.HTTPStatus != nil || got.Outcome != "transport_error") {
					t.Fatalf("earlier HTTP 200 attached to a failed request: %+v", got)
				}
				wantStage := "tls_handshake"
				if afterHeaders {
					wantStage = "response_read"
				}
				if got.Details["request_failure_phase"] != wantStage {
					t.Fatalf("failed request stage lost: %+v", got)
				}
				if afterHeaders && (got.HTTPStatus == nil || *got.HTTPStatus != 200 || got.Outcome != "timed_out") {
					t.Fatalf("actual partial response status lost: %+v", got)
				}
				if service == "netflix_unlock" && got.Details["global_title_http_status"] != "200" {
					t.Fatal("completed title status was discarded")
				}
				if service == "antigravity" && got.FailurePhase != "fetchAvailableModels" {
					t.Fatal("failed model catalog step was discarded")
				}
			})
		}
	}
}

func TestAntigravityNetworkTimeoutKeepsOriginAndReadStage(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		response := fixtureResponse(req, 200, "application/json", "")
		response.Body = timeoutBody{}
		return response, nil
	})}
	got := checkAntigravity(context.Background(), client, "fixture-token", time.Now())
	if got.Outcome != "timed_out" || got.FailurePhase != "loadCodeAssist" || got.Details["timeout_origin"] != "transport" || got.Details["request_failure_phase"] != "response_read" || strings.Contains(got.ErrorMessage, "总时长") || calls != 1 {
		t.Fatalf("transport timeout mislabeled as overall deadline or stage lost: %+v", got)
	}
}
