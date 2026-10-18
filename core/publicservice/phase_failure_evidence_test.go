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
		result := Result{}
		phases.attach(&result)
		wantFailures, wantWritten := "0", "1"
		if failed {
			wantFailures, wantWritten = "1", "0"
		}
		if result.Details["tls_handshake_attempts"] != "1" || result.Details["tls_handshake_failures"] != wantFailures || result.Details["http_requests_written"] != wantWritten {
			t.Fatalf("TLS success/failure evidence missing: %+v", result.Details)
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
	phases.attach(&result)
	d := result.Details
	if d["proxy_connect_attempts"] != "1" || d["proxy_connect_failures"] != "1" || d["proxy_connect_error"] == "" || strings.Contains(d["proxy_connect_error"], "private-value") || d["tls_handshake_attempts"] != "" || d["http_requests_written"] != "0" {
		t.Fatalf("dial failure evidence missing, unsafe or invented: %+v", d)
	}
}
