package publicservice

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestAntigravityQuotaDoesNotBecomeBodyConnectionFailure(t *testing.T) {
	for _, step := range []string{"loadCodeAssist", "fetchAvailableModels", "streamGenerateContent?alt=sse"} {
		t.Run(step, func(t *testing.T) {
			calls := 0
			body := &archiveFixtureBody{}
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.URL.String() == antigravityBase+step {
					response := fixtureResponse(req, 429, "application/json", "")
					response.Header.Set("Retry-After", "60")
					response.Body = body
					return response, nil
				}
				if calls == 1 {
					return fixtureResponse(req, 200, "application/json", `{"cloudaicompanionProject":"fixture-project"}`), nil
				}
				return fixtureResponse(req, 200, "application/json", `{"models":{"gemini-fixture-flash":{}}}`), nil
			})}
			got := checkAntigravity(context.Background(), client, "fixture-token", time.Now())
			wantCalls := map[string]int{"loadCodeAssist": 1, "fetchAvailableModels": 2, "streamGenerateContent?alt=sse": 3}[step]
			if got.Outcome != "rate_limited" || got.FailurePhase != step || got.HTTPStatus == nil || *got.HTTPStatus != 429 || got.Details["retry_after_seconds"] != "60" || calls != wantCalls || got.RequestCount != wantCalls || body.reads != 0 || !body.closed {
				t.Fatalf("quota evidence replaced or more requests made: calls=%d reads=%d closed=%v result=%+v", calls, body.reads, body.closed, got)
			}
		})
	}
}
