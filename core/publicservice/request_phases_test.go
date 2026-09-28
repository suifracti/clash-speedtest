package publicservice

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
