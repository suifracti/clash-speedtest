package publicservice

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"testing"
	"time"
)

func TestPhaseEvidenceSeparatesTLSFailureFromSuccess(t *testing.T) {
	for _, failed := range []bool{false, true} {
		phases := &requestPhases{}
		client := &http.Client{Transport: phases.wrap(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			trace := httptrace.ContextClientTrace(req.Context())
			trace.TLSHandshakeStart()
			if failed {
				trace.TLSHandshakeDone(tls.ConnectionState{}, errors.New("fixture TLS error access_token=private-value"))
				return nil, io.EOF
			}
			trace.TLSHandshakeDone(tls.ConnectionState{}, nil)
			trace.WroteRequest(httptrace.WroteRequestInfo{})
			return fixtureResponse(req, 204, "", ""), nil
		}), false)}
		response, _ := client.Get("https://fixture.invalid/")
		if response != nil {
			response.Body.Close()
		}
		result := Result{Outcome: "transport_error", FailurePhase: "transport"}
		phases.attach(&result)
		wantFailures, wantWritten := "0", "1"
		if failed {
			wantFailures, wantWritten = "1", "0"
		}
		if result.Details["tls_handshake_attempts"] != "1" || result.Details["tls_handshake_failures"] != wantFailures || result.Details["http_requests_written"] != wantWritten {
			t.Fatalf("TLS success/failure evidence missing: %+v", result.Details)
		}
		if failed && (result.FailurePhase != "tls_handshake" || !strings.Contains(result.ErrorMessage, "TLS 握手失败")) {
			t.Fatalf("TLS failure still generic: %+v", result)
		}
		if failed && (result.Details["tls_handshake_error"] == "" || strings.Contains(result.Details["tls_handshake_error"], "private-value")) {
			t.Fatalf("TLS error missing or unsafe: %+v", result.Details)
		}
	}
}

func TestPhaseEvidenceRecordsProxyDialFailureWithoutNetwork(t *testing.T) {
	phases := &requestPhases{}
	transport := &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("fixture dial access_token=private-value")
	}}
	client := &http.Client{Transport: phases.wrap(transport, true)}
	defer client.CloseIdleConnections()
	_, err := client.Get("http://fixture.invalid/")
	if err == nil {
		t.Fatal("expected fixture dial failure")
	}
	result := Result{}
	classifyRequestError(&result, context.Background(), err)
	phases.attach(&result)
	d := result.Details
	if result.FailurePhase != "proxy_connect" || d["request_failure_phase"] != "proxy_connect" || strings.Contains(result.ErrorMessage, "private-value") {
		t.Fatalf("proxy failure stage lost or unsafe: %+v", result)
	}
	if d["proxy_connect_attempts"] != "1" || d["proxy_connect_failures"] != "1" || d["proxy_connect_error"] == "" || strings.Contains(d["proxy_connect_error"], "private-value") || d["tls_handshake_attempts"] != "" || d["http_requests_written"] != "0" {
		t.Fatalf("dial failure evidence missing, unsafe or invented: %+v", d)
	}
}

func TestCancelledRequestRetainsPendingDialEvidence(t *testing.T) {
	started, stopped := make(chan struct{}), make(chan struct{})
	phases := &requestPhases{}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		close(started)
		<-ctx.Done()
		close(stopped)
		return nil, ctx.Err()
	}}
	client := &http.Client{Transport: phases.wrap(transport, true)}
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://fixture.invalid/", nil)
	done := make(chan error, 1)
	go func() { _, err := client.Do(request); done <- err }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("fixture dial did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("request error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("request cancellation waited for detached dial")
	}
	result := Result{}
	phases.attach(&result)
	d := result.Details
	if d["proxy_connect_attempts"] != "1" || d["proxy_connect_pending"] != "1" || d["proxy_connect_failures"] != "0" || d["proxy_connect_ms"] != "" {
		t.Errorf("cancelled request lost or completed a pending dial: %+v", d)
	}
	client.CloseIdleConnections()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("CloseIdleConnections did not cancel fixture dial")
	}
}

func TestCancelledRequestRetainsPendingTLSHandshakeEvidence(t *testing.T) {
	phases := &requestPhases{}
	var lateDone func(tls.ConnectionState, error)
	client := &http.Client{Transport: phases.wrap(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		trace := httptrace.ContextClientTrace(req.Context())
		trace.TLSHandshakeStart()
		lateDone = trace.TLSHandshakeDone
		return nil, context.DeadlineExceeded
	}), false)}
	_, err := client.Get("https://fixture.invalid/")
	if err == nil || lateDone == nil {
		t.Fatal("fixture never started its TLS handshake")
	}
	result := Result{}
	phases.attach(&result)
	d := result.Details
	if d["tls_handshake_attempts"] != "1" || d["tls_handshake_pending"] != "1" || d["tls_handshake_failures"] != "0" || d["tls_handshake_ms"] != "" || d["http_requests_written"] != "0" {
		t.Fatalf("pending TLS disappeared or received a fabricated completed duration: %v", d)
	}
	lateDone(tls.ConnectionState{}, context.DeadlineExceeded)
	completed := Result{}
	phases.attach(&completed)
	if completed.Details["tls_handshake_attempts"] != "1" || completed.Details["tls_handshake_pending"] != "0" || completed.Details["tls_handshake_failures"] != "1" || completed.Details["tls_handshake_ms"] == "" || d["tls_handshake_pending"] != "1" {
		t.Fatalf("late completion was lost or mutated the old snapshot: old=%v new=%v", d, completed.Details)
	}
}
