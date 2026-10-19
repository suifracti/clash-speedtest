package publicservice

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"testing"
)

func TestLaterRequestTLSFailureKeepsItsOwnStage(t *testing.T) {
	phases := &requestPhases{}
	calls := 0
	client := &http.Client{Transport: phases.wrap(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		trace := httptrace.ContextClientTrace(req.Context())
		trace.TLSHandshakeStart()
		if calls == 1 {
			trace.TLSHandshakeDone(tls.ConnectionState{}, nil)
			trace.WroteRequest(httptrace.WroteRequestInfo{})
			return fixtureResponse(req, 200, "text/plain", "ok"), nil
		}
		trace.TLSHandshakeDone(tls.ConnectionState{}, io.ErrUnexpectedEOF)
		return nil, io.ErrUnexpectedEOF
	}), false)}
	response, err := client.Get("https://fixture.invalid/first")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	_, err = client.Get("https://fixture.invalid/second")
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("underlying error lost: %v", err)
	}
	result := Result{}
	classifyRequestError(&result, context.Background(), err)
	phases.attach(&result)
	if result.FailurePhase != "tls_handshake" || result.Details["request_failure_phase"] != "tls_handshake" || !strings.Contains(result.ErrorMessage, "TLS 握手失败") || result.Details["http_requests_written"] != "1" {
		t.Fatalf("earlier success hid the later TLS failure: %+v", result)
	}
}

func TestBodyInterruptionKeepsHTTPStatusBytesAndReadStage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.Write([]byte("partial"))
	}))
	defer server.Close()
	phases := &requestPhases{}
	client := server.Client()
	client.Transport = phases.wrap(client.Transport, false)
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("error = %v", err)
	}
	status := response.StatusCode
	result := Result{HTTPStatus: &status, BytesRead: int64(len(body))}
	classifyRequestError(&result, context.Background(), err)
	phases.attach(&result)
	if result.FailurePhase != "response_read" || result.Details["request_failure_phase"] != "response_read" || *result.HTTPStatus != 200 || result.BytesRead != 7 {
		t.Fatalf("partial response mislabeled: %+v", result)
	}
}

func TestHeaderTimeoutRetainsTimeoutAndRequestStage(t *testing.T) {
	phases := &requestPhases{}
	client := &http.Client{Transport: phases.wrap(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		httptrace.ContextClientTrace(req.Context()).WroteRequest(httptrace.WroteRequestInfo{})
		return nil, context.DeadlineExceeded
	}), false)}
	_, err := client.Get("https://fixture.invalid/")
	result := Result{}
	classifyRequestError(&result, context.Background(), err)
	if result.Outcome != "timed_out" || result.FailurePhase != "timeout" || result.Details["request_failure_phase"] != "response_headers" || result.Details["timeout_origin"] != "transport" {
		t.Fatalf("timeout lost cause or stage: %+v", result)
	}
}
