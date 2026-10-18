package publicservice

import (
	"context"
	"net/http"
	"testing"

	"github.com/faceair/clash-speedtest/core/monitor"
)

type archiveFixtureBody struct {
	reads  int
	closed bool
}

func (b *archiveFixtureBody) Read(p []byte) (int, error) { b.reads++; return 0, networkTimeoutError{} }
func (b *archiveFixtureBody) Close() error               { b.closed = true; return nil }

func TestCodeloadOnlyRequestsArchiveHeaders(t *testing.T) {
	rule, _ := RuleFor("github_codeload")
	body := &archiveFixtureBody{}
	method := ""
	c := Checker{ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
		method = req.Method
		r := fixtureResponse(req, 200, "application/zip", "")
		r.Body = body
		return r, nil
	})}
	r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
	if method != http.MethodHead || r.Outcome != "reachable" || r.BytesRead != 0 || body.reads != 0 || !body.closed {
		t.Fatalf("method=%s outcome=%s bytes=%d reads=%d closed=%v", method, r.Outcome, r.BytesRead, body.reads, body.closed)
	}
}
