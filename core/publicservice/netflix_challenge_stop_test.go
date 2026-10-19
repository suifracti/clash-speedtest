package publicservice

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
)

type netflixInterruptedBody struct {
	reads  int
	closed bool
}

func (b *netflixInterruptedBody) Read([]byte) (int, error) { b.reads++; return 0, io.ErrUnexpectedEOF }
func (b *netflixInterruptedBody) Close() error             { b.closed = true; return nil }

func TestNetflixStopsOnChallengeHeadersBeforeReadingBody(t *testing.T) {
	for _, title := range []string{"first", "second"} {
		t.Run(title, func(t *testing.T) {
			rule, _ := RuleFor("netflix_unlock")
			body := &netflixInterruptedBody{}
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if title == "second" && calls == 1 {
					return fixtureResponse(req, 200, "text/html", `<html>nflx://www.netflix.com/title/fixture</html>`), nil
				}
				response := fixtureResponse(req, 403, "text/html", "")
				response.Header.Set("cf-mitigated", "challenge")
				response.Body = body
				return response, nil
			})}
			// Exercise the current rule's title chain after DNS preparation; the
			// selected-node DNS/TLS path has separate local-server coverage.
			got := (Checker{}).checkOnce(context.Background(), client, monitor.MonitoredNode{}, rule, time.Now())
			wantCalls := 1
			if title == "second" {
				wantCalls = 2
			}
			if got.Outcome != "challenge" || got.HTTPStatus == nil || *got.HTTPStatus != 403 || got.RequestCount != wantCalls || calls != wantCalls || body.reads != 0 || !body.closed {
				t.Fatalf("challenge became body/connection failure: reads=%d closed=%v calls=%d result=%+v", body.reads, body.closed, calls, got)
			}
		})
	}
}

func TestNetflixFirstPageBarrierIsNotHiddenByLaterConnectionFailure(t *testing.T) {
	for _, tc := range []struct{ name, body, outcome string }{
		{"explicit", `<html><script src="/cdn-cgi/challenge-platform/test"></script></html>`, "challenge"},
		{"suspected", `<html><title>Just a moment...</title></html>`, "suspected_challenge"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule, _ := RuleFor("netflix_unlock")
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return fixtureResponse(req, 200, "text/html", tc.body), nil
				}
				return nil, io.EOF
			})}
			got := (Checker{}).checkOnce(context.Background(), client, monitor.MonitoredNode{}, rule, time.Now())
			if got.Outcome != tc.outcome || calls != 1 || got.RequestCount != 1 || got.HTTPStatus == nil || *got.HTTPStatus != 200 || got.BytesRead != int64(len(tc.body)) {
				t.Fatalf("first page evidence discarded or extra request made: calls=%d result=%+v", calls, got)
			}
			if tc.outcome == "suspected_challenge" && isFailureOutcome(got) {
				t.Fatal("weak challenge evidence became confirmed rejection")
			}
		})
	}
}
