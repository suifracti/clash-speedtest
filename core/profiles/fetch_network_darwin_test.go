//go:build darwin

package profiles

import "testing"

func TestMacSystemProxySnapshot(t *testing.T) {
	const snapshot = `<dictionary> {
 HTTPEnable : 1
 HTTPProxy : 127.0.0.1
 HTTPPort : 7897
 HTTPSEnable : 1
 HTTPSProxy : ::1
 HTTPSPort : 7898
 }`
	for _, tc := range []struct{ name, scheme, body, want string }{
		{"HTTP", "http", snapshot, "http://127.0.0.1:7897"},
		{"HTTPS", "https", snapshot, "http://[::1]:7898"},
		{"disabled", "http", "HTTPEnable : 0\nHTTPProxy : 127.0.0.1\nHTTPPort : 7897", ""},
		{"bad port", "http", "HTTPEnable : 1\nHTTPProxy : 127.0.0.1\nHTTPPort : 99999", ""},
		{"bad host", "http", "HTTPEnable : 1\nHTTPProxy : user:secret@proxy.invalid\nHTTPPort : 7897", ""},
		{"scoped proxy must not override global", "http", "<dictionary> {\nHTTPEnable : 0\n__SCOPED__ : <dictionary> {\nen1 : <dictionary> {\nHTTPEnable : 1\nHTTPProxy : 127.0.0.1\nHTTPPort : 7897\n}\n}\n}", ""},
		{"PAC not invented", "https", "ProxyAutoConfigEnable : 1\nProxyAutoConfigURLString : https://pac.invalid/config", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := parseSystemProxySnapshot([]byte(tc.body), tc.scheme)
			got := ""
			if p != nil {
				got = p.String()
			}
			if got != tc.want {
				t.Fatalf("got=%s", got)
			}
		})
	}
}
