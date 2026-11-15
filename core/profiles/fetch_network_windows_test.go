//go:build windows

package profiles

import "testing"

func TestWindowsSystemProxySnapshot(t *testing.T) {
	for _, tc := range []struct {
		name    string
		enabled uint64
		server  string
		scheme  string
		want    string
	}{
		{"unified HTTP", 1, "127.0.0.1:7897", "http", "http://127.0.0.1:7897"},
		{"unified HTTPS", 1, "127.0.0.1:7897", "https", "http://127.0.0.1:7897"},
		{"unified with http scheme prefix", 1, "http://127.0.0.1:7897", "https", "http://127.0.0.1:7897"},
		{"unified IPv6", 1, "[::1]:7897", "https", "http://[::1]:7897"},
		{"per-scheme HTTP", 1, "http=127.0.0.1:7897;https=127.0.0.1:7898", "http", "http://127.0.0.1:7897"},
		{"per-scheme HTTPS", 1, "http=127.0.0.1:7897;https=[::1]:7898", "https", "http://[::1]:7898"},
		{"per-scheme socks only not invented for https", 1, "socks=127.0.0.1:7898", "https", ""},
		{"disabled", 0, "127.0.0.1:7897", "http", ""},
		{"bad port", 1, "127.0.0.1:99999", "http", ""},
		{"bad host credentials", 1, "user:secret@proxy.invalid:7897", "http", ""},
		{"unsupported socks scheme url", 1, "socks5://127.0.0.1:7898", "http", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := parseWindowsProxyServer(tc.enabled, tc.server, tc.scheme)
			got := ""
			if p != nil {
				got = p.String()
			}
			if got != tc.want {
				t.Fatalf("got=%q want=%q", got, tc.want)
			}
		})
	}
}
