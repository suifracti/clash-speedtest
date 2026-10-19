package publicservice

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServiceHTTP2NegotiationPreservesNodeDialAndHTTP1Fallback(t *testing.T) {
	for _, http2 := range []bool{true, false} {
		name := "http1_fallback"
		want := 1
		if http2 {
			name, want = "http2", 2
		}
		t.Run(name, func(t *testing.T) {
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.ProtoMajor != want {
					w.WriteHeader(http.StatusHTTPVersionNotSupported)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			server.EnableHTTP2 = http2
			server.StartTLS()
			defer server.Close()
			client := server.Client()
			tr := client.Transport.(*http.Transport)
			tr.ForceAttemptHTTP2 = false // Custom node dialers suppress Go's automatic HTTP/2 setup.
			tr.DisableKeepAlives = true
			dials := 0
			tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				dials++
				if addr != "example.com:443" {
					t.Errorf("target changed: %s", addr)
				}
				return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
			}
			enableServiceHTTP2(client)
			defer client.CloseIdleConnections()
			response, err := client.Get("https://example.com/")
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != 204 || response.ProtoMajor != want || dials != 1 || !tr.DisableKeepAlives {
				t.Fatalf("protocol/route changed: status=%d protocol=%s dials=%d", response.StatusCode, response.Proto, dials)
			}
		})
	}
}
