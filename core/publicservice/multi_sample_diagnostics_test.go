package publicservice

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptrace"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/speedtester"
)

func TestMultiSampleDiagnosticStatusBelongsToReportedSample(t *testing.T) {
	for _, tc := range []struct {
		name, outcome, scope, phase, origin string
		statuses                            []int
		wantStatus, calls                   int
	}{
		{"later TLS failure", "unknown", "first_unconfirmed_sample", "tls_handshake", "", []int{204, 0, 204}, 0, 3},
		{"later body timeout", "unknown", "first_unconfirmed_sample", "response_read", "transport", []int{204, 200, 204}, 200, 3},
		{"later server error", "unknown", "first_unconfirmed_sample", "", "", []int{204, 500, 204}, 500, 3},
		{"later cancellation", "cancelled", "first_cancelled_sample", "response_headers", "", []int{204, -1}, 0, 2},
		{"quota after timeout", "rate_limited", "first_rate_limited_sample", "", "", []int{-2, 429}, 429, 2},
		{"later local binding failure", "unknown", "first_unconfirmed_sample", "", "", []int{204, -3}, 0, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			rule, _ := RuleFor("cloudflare_204")
			if tc.name == "later body timeout" {
				rule, _ = RuleFor("github_api_root")
			}
			checker := Checker{DriftSamples: 3, ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
				if calls >= len(tc.statuses) {
					t.Fatal("unexpected request after cancellation/quota")
				}
				status := tc.statuses[calls]
				calls++
				trace := httptrace.ContextClientTrace(req.Context())
				switch status {
				case 0:
					trace.TLSHandshakeStart()
					trace.TLSHandshakeDone(tls.ConnectionState{}, io.EOF)
					return nil, io.EOF
				case -1:
					trace.WroteRequest(httptrace.WroteRequestInfo{})
					return nil, context.Canceled
				case -2:
					trace.WroteRequest(httptrace.WroteRequestInfo{})
					return nil, networkTimeoutError{}
				case -3:
					return nil, &speedtester.PhysicalPathError{Cause: errors.New("fixture binding unavailable")}
				}
				trace.WroteRequest(httptrace.WroteRequestInfo{})
				if tc.name == "later body timeout" && calls != 2 {
					status = 200
				}
				response := fixtureResponse(req, status, "application/json", `{"current_user_url":"https://api.github.com/user","repository_url":"https://api.github.com/repos/{owner}/{repo}"}`)
				if tc.name == "later body timeout" && calls == 2 {
					response.Body = timeoutBody{}
				}
				return response, nil
			})}
			got := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, time.Second)
			if got.Outcome != tc.outcome || calls != tc.calls || got.RequestCount != tc.calls {
				t.Fatalf("sample outcome/count changed: %+v calls=%d", got, calls)
			}
			if tc.wantStatus == 0 && got.HTTPStatus != nil || tc.wantStatus != 0 && (got.HTTPStatus == nil || *got.HTTPStatus != tc.wantStatus) {
				t.Fatalf("status belongs to another sample: %+v", got)
			}
			if got.Details["http_status_scope"] != tc.scope || got.Details["request_failure_phase"] != tc.phase || got.Details["timeout_origin"] != tc.origin {
				t.Fatalf("failure diagnostic lost or mixed: %+v", got)
			}
			if tc.name == "later TLS failure" && (got.Details["first_sample_http_status"] != "204" || got.Details["sample_success"] != "2" || got.Details["sample_unknown"] != "1" || got.Details["sample_failure"] != "0") {
				t.Fatalf("previous successes were discarded: %+v", got)
			}
		})
	}
}
