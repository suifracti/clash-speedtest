package web

import (
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
)

// Only synthetic ServerConfig and in-memory handlers: no network listener,
// real subscription URL, production database or external HTTP call.
func TestW12iIntegratedSubscriptionRouteRespectsWebAuthentication(t *testing.T) {
    const authority = "192.0.2.10:8999"
    server := newAccessTestServer(t, ServerConfig{
        Port: 8999, ListenAddress: "192.0.2.10", WebPassword: accessTestPassword,
    })
    handler := server.Handler()
    tests := []struct {
        name string
        method string
        host string
        origin string
        bearer string
        want int
    }{
        {"remote unauthenticated list", http.MethodGet, authority, "", "", http.StatusUnauthorized},
        {"remote authenticated list", http.MethodGet, authority, "http://" + authority, accessTestPassword, http.StatusOK},
        {"remote spoofed Host", http.MethodGet, "evil.example:8999", "", accessTestPassword, http.StatusForbidden},
        {"remote untrusted Origin", http.MethodGet, authority, "http://evil.example:8999", accessTestPassword, http.StatusForbidden},
        {"remote unauthenticated mutation", http.MethodPost, authority, "", "", http.StatusUnauthorized},
        {"remote authenticated invalid mutation", http.MethodPost, authority, "http://" + authority, accessTestPassword, http.StatusBadRequest},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            req := httptest.NewRequest(tc.method, "http://"+tc.host+"/api/subscription-refresh-jobs", strings.NewReader(""))
            req.Host = tc.host
            if tc.origin != "" { req.Header.Set("Origin", tc.origin) }
            if tc.bearer != "" { req.Header.Set("Authorization", "Bearer "+tc.bearer) }
            reply := httptest.NewRecorder()
            handler.ServeHTTP(reply, req)
            if reply.Code != tc.want {
                t.Fatalf("%s %s returned %d, want %d: %s", tc.method, tc.host, reply.Code, tc.want, reply.Body.String())
            }
        })
    }
    local := newAccessTestServer(t, ServerConfig{Port: 8998})
    req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8998/api/subscription-refresh-jobs", nil)
    reply := httptest.NewRecorder()
    local.Handler().ServeHTTP(reply, req)
    if reply.Code != http.StatusOK {
        t.Fatalf("local refresh listing returned %d, want 200: %s", reply.Code, reply.Body.String())
    }
}
