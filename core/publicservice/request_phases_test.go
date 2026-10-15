package publicservice

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/faceair/clash-speedtest/core/monitor"
)

func TestPhaseTimingsDoNotInventUnobservedDNSOrProxyDial(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	client := server.Client()
	phases := &requestPhases{}
	client.Transport = phases.wrap(client.Transport, false)
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	result := Result{}
	phases.attach(&result)
	if result.Details["tls_handshake_ms"] == "" || result.Details["first_response_byte_ms"] == "" {
		t.Fatalf("missing observed phases: %+v", result.Details)
	}
	if result.Details["proxy_connect_ms"] != "" || result.Details["dns_ms"] != "" {
		t.Fatalf("invented proxy or DNS timing: %+v", result.Details)
	}
}

func TestPhaseTelemetryCountsFailedRequestsAsExecuted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic(http.ErrAbortHandler) }))
	defer server.Close()
	phases := &requestPhases{}
	client := server.Client()
	client.Transport = phases.wrap(client.Transport, false)
	_, err := client.Get(server.URL)
	if err == nil {
		t.Fatal("expected interrupted response")
	}
	result := Result{Outcome: "transport_error"}
	phases.attach(&result)
	if result.RequestCount != 1 || result.Details["execution_status"] != "executed" || result.Details["conclusion"] != "unconfirmed" {
		t.Fatalf("executed failed request lost: %+v", result)
	}
}

func TestRequestPhasesNilSafety(t *testing.T) {
	var nilPhases *requestPhases
	if rt := nilPhases.wrap(nil, false); rt != http.DefaultTransport {
		t.Fatalf("expected wrap on nil phases to return default transport, got %v", rt)
	}
	// attach on nil should be safe
	nilPhases.attach(nil)
	res := &Result{}
	nilPhases.attach(res)

	// attach with non-nil phases and nil result should be safe
	validPhases := &requestPhases{}
	validPhases.attach(nil)

	// RoundTrip on nil request should return error without panic
	pt := phaseTransport{}
	if _, err := pt.RoundTrip(nil); err == nil {
		t.Fatal("expected error from RoundTrip(nil)")
	}

	// timedServiceBody on nil ReadCloser should return EOF without panic
	tb := timedServiceBody{}
	buf := make([]byte, 10)
	n, err := tb.Read(buf)
	if n != 0 || err == nil {
		t.Fatalf("expected 0, EOF from nil timedServiceBody, got n=%d, err=%v", n, err)
	}

	// Check on nil ctx should not panic
	c := Checker{}
	finalRes := c.Check(nil, monitor.MonitoredNode{}, Rule{ServiceID: "invalid"}, 0)
	if finalRes.Outcome == "" {
		t.Fatal("expected non-empty outcome from Check on nil ctx")
	}
}
