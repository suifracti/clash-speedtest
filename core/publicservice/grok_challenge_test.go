package publicservice

import (
	"context"
	"github.com/faceair/clash-speedtest/core/monitor"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGrokBoundedNonSuccessEvidence(t *testing.T) {
	rule, _ := RuleFor("grok_web")
	for _, tc := range []struct {
		name          string
		status        int
		body, outcome string
	}{
		{"explicit", 403, `<script src="/cdn-cgi/challenge-platform/test"></script>`, "challenge"},
		{"suspected", 403, `<title>Just a moment...</title>`, "suspected_challenge"},
		{"limited", 429, `<title>Just a moment...</title>`, "rate_limited"},
		{"denied", 403, `<h1>Forbidden</h1>`, "http_rejected"},
		{"large", 403, strings.Repeat("x", 16384) + `cf-chl-`, "http_rejected"},
		{"entry", 200, `<title>Grok</title>`, "reachable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			reader := &countReader{reader: strings.NewReader(tc.body)}
			c := Checker{ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
				requests++
				r := fixtureResponse(req, tc.status, "text/html", "")
				r.Body = io.NopCloser(reader)
				return r, nil
			})}
			result := c.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
			if result.Outcome != tc.outcome {
				t.Fatalf("got %+v", result)
			}
			if requests != 1 || reader.bytes > 8192 {
				t.Fatalf("requests=%d read=%d", requests, reader.bytes)
			}
			if tc.status == 403 && result.BytesRead != int64(reader.bytes) {
				t.Fatalf("missing bounded body evidence: %+v", result)
			}
		})
	}
}

type countReader struct {
	reader io.Reader
	bytes  int
}

func (r *countReader) Read(p []byte) (int, error) {
	n, e := r.reader.Read(p)
	r.bytes += n
	return n, e
}

func TestGrok429WinsOverChallengeHeader(t *testing.T) {
	rule, _ := RuleFor("grok_web")
	c := Checker{ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
		r := fixtureResponse(req, 429, "text/html", "challenge")
		r.Header.Set("cf-mitigated", "challenge")
		return r, nil
	})}
	r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	if r.Outcome != "rate_limited" {
		t.Fatalf("429 must not enter business failure denominator: %+v", r)
	}
}
