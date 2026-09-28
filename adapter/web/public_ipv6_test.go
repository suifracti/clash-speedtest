package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExplicitPublicIPv6HostAndOrigin(t *testing.T) {
	const host = "[2001:db8::123]:8999"
	handler := securityMiddlewareForHost(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }), host)
	for _, tc := range []struct {
		host, origin string
		want         int
	}{
		{host, "http://" + host, 204},
		{host, "http://evil.example", 403},
		{"[2001:db8::999]:8999", "", 403},
		{"evil.example:8999", "", 403},
		{"127.0.0.1:8999", "http://127.0.0.1:8999", 204},
	} {
		req := httptest.NewRequest(http.MethodPost, "http://"+tc.host+"/api/test", nil)
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != tc.want {
			t.Fatalf("host=%s origin=%s: got %d want %d", tc.host, tc.origin, res.Code, tc.want)
		}
	}
}

func TestNetworkListenerSupportsTunnelSameOrigin(t *testing.T) {
	handler := securityMiddlewareForHost(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), "*")
	for _, origin := range []string{"https://share.example", "https://evil.example"} {
		req := httptest.NewRequest("POST", "http://share.example/api/test", nil)
		req.Header.Set("Origin", origin)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		want := 204
		if origin == "https://evil.example" {
			want = 403
		}
		if response.Code != want {
			t.Fatalf("origin %s: got %d want %d", origin, response.Code, want)
		}
	}
}
