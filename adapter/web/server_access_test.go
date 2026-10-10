package web

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faceair/clash-speedtest/core/profiles"
)

const accessTestPassword = "synthetic-test-password"

func newAccessTestServer(t *testing.T, cfg ServerConfig) *Server {
	t.Helper()
	if cfg.ProfilePaths.Dir == "" {
		cfg.ProfilePaths = profiles.Paths{Dir: t.TempDir()}
	}
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	return server
}

func TestNewServerDefaultsToLoopbackWithoutAuthentication(t *testing.T) {
	server := newAccessTestServer(t, ServerConfig{Port: 8999})
	if got := server.config.ListenAddress; got != "127.0.0.1" {
		t.Fatalf("default ListenAddress = %q, want 127.0.0.1", got)
	}

	for _, tc := range []struct {
		host string
		want int
	}{
		{"127.0.0.1:8999", http.StatusOK},
		{"console.example:8999", http.StatusForbidden},
	} {
		req := httptest.NewRequest(http.MethodGet, "http://"+tc.host+"/api/auth/status", nil)
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, req)
		if recorder.Code != tc.want {
			t.Errorf("Host %q: got %d, want %d", tc.host, recorder.Code, tc.want)
		}
	}
}

func TestNewServerRejectsRemoteAccessWithoutStrongPasswordBeforeStorage(t *testing.T) {
	for _, tc := range []struct {
		name     string
		password string
	}{
		{name: "missing"},
		{name: "short", password: "short"},
		{name: "whitespace", password: "            "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profileDir := filepath.Join(t.TempDir(), "must-not-be-created")
			server, err := NewServer(ServerConfig{
				Port:          8999,
				ListenAddress: "192.0.2.10",
				WebPassword:   tc.password,
				ProfilePaths:  profiles.Paths{Dir: profileDir},
			})
			if err == nil {
				_ = server.Close()
				t.Fatal("expected remote access validation error")
			}
			if !strings.Contains(err.Error(), "web-password") {
				t.Fatalf("expected web-password validation error, got: %v", err)
			}
			if _, statErr := os.Stat(profileDir); !os.IsNotExist(statErr) {
				t.Fatalf("storage path was touched before access validation: %v", statErr)
			}
		})
	}
}

func TestNewServerRejectsRemoteWildcardListeners(t *testing.T) {
	for _, address := range []string{"0.0.0.0", "::"} {
		t.Run(address, func(t *testing.T) {
			server, err := NewServer(ServerConfig{
				ListenAddress: address,
				WebPassword:   strings.Repeat("x", 12),
				ProfilePaths:  profiles.Paths{Dir: t.TempDir()},
			})
			if err == nil {
				_ = server.Close()
				t.Fatal("expected wildcard listener to be rejected")
			}
		})
	}
}

func TestRemoteAccessRequiresExactHostAndOriginWithAuthentication(t *testing.T) {
	const host = "192.0.2.10:8999"
	server := newAccessTestServer(t, ServerConfig{
		Port:          8999,
		ListenAddress: "192.0.2.10",
		WebPassword:   accessTestPassword,
	})

	for _, tc := range []struct {
		name   string
		method string
		host   string
		origin string
		want   int
	}{
		{name: "matching host and origin", method: http.MethodPost, host: host, origin: "http://" + host, want: http.StatusOK},
		{name: "wrong host", method: http.MethodPost, host: "console.example:8999", origin: "http://console.example:8999", want: http.StatusForbidden},
		{name: "loopback host on remote listener", method: http.MethodPost, host: "127.0.0.1:8999", origin: "http://127.0.0.1:8999", want: http.StatusForbidden},
		{name: "wrong origin", method: http.MethodPost, host: host, origin: "http://evil.example:8999", want: http.StatusForbidden},
		{name: "wrong origin on read request", method: http.MethodGet, host: host, origin: "http://evil.example:8999", want: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := "/api/auth/status"
			var req *http.Request
			if tc.method == http.MethodPost {
				path = "/api/auth/login"
				body := bytes.NewReader([]byte(fmt.Sprintf(`{"password":%q}`, accessTestPassword)))
				req = httptest.NewRequest(tc.method, "http://"+tc.host+path, body)
			} else {
				req = httptest.NewRequest(tc.method, "http://"+tc.host+path, nil)
			}
			req.Host = tc.host
			req.Header.Set("Origin", tc.origin)
			recorder := httptest.NewRecorder()
			server.Handler().ServeHTTP(recorder, req)
			if recorder.Code != tc.want {
				t.Fatalf("got %d, want %d", recorder.Code, tc.want)
			}
		})
	}
}

func TestRemotePasswordLengthCountsUnicodeCharacters(t *testing.T) {
	server := newAccessTestServer(t, ServerConfig{
		ListenAddress: "192.0.2.10",
		WebPassword:   strings.Repeat("界", minRemoteWebPasswordLength),
	})
	if got := server.config.ListenAddress; got != "192.0.2.10" {
		t.Fatalf("ListenAddress = %q, want 192.0.2.10", got)
	}
}

func TestPublicIPv6RequiresAuthenticationAndBindsSelectedAddress(t *testing.T) {
	const publicIPv6 = "2001:4860:4860::8888"
	unauthenticated, err := NewServer(ServerConfig{
		PublicIPv6:   publicIPv6,
		ProfilePaths: profiles.Paths{Dir: filepath.Join(t.TempDir(), "unauthenticated")},
	})
	if err == nil {
		_ = unauthenticated.Close()
		t.Fatal("expected unauthenticated public IPv6 to be rejected")
	}

	server := newAccessTestServer(t, ServerConfig{
		Port:        8999,
		PublicIPv6:  publicIPv6,
		WebPassword: accessTestPassword,
	})
	if server.config.ListenAddress != publicIPv6 {
		t.Fatalf("public IPv6 bind address = %q, want %q", server.config.ListenAddress, publicIPv6)
	}

	host := "[" + publicIPv6 + "]:8999"
	req := httptest.NewRequest(http.MethodPost, "http://"+host+"/api/auth/login", strings.NewReader(fmt.Sprintf(`{"password":%q}`, accessTestPassword)))
	req.Host = host
	req.Header.Set("Origin", "http://"+host)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("matching public IPv6 Host/Origin was rejected: status %d", recorder.Code)
	}
}
