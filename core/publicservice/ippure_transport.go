package publicservice

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"

	utls "github.com/metacubex/utls"
	"golang.org/x/net/http2"
)

// Only IPPure's API/summary hosts use this profile. The original node dialer,
// certificate roots, cancellation and socket binding remain in effect.
type ippureBrowserTransport struct {
	base *http.Transport
	h2   *http2.Transport
}

func (t *ippureBrowserTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme == "https" && (r.URL.Host == "api.123169.xyz" || r.URL.Host == "cf.999831.xyz") {
		clone := r.Clone(r.Context())
		clone.Header = r.Header.Clone()
		clone.Header.Set("User-Agent", strings.Replace(browserUserAgent, "Chrome/131.", "Chrome/133.", 1))
		return t.h2.RoundTrip(clone)
	}
	return t.base.RoundTrip(r)
}
func (t *ippureBrowserTransport) CloseIdleConnections() {
	t.base.CloseIdleConnections()
	t.h2.CloseIdleConnections()
}
func ippureBrowserSession(base http.RoundTripper) http.RoundTripper {
	if p, ok := base.(phaseTransport); ok {
		return phaseTransport{base: ippureBrowserSession(p.base), phases: p.phases}
	}
	original, ok := base.(*http.Transport)
	if !ok {
		return base
	}
	tr := original.Clone()
	tr.DisableKeepAlives = false
	// Preserve custom TLS/proxy policies rather than quietly replacing them.
	if tr.DialContext == nil || tr.Proxy != nil || tr.DialTLSContext != nil {
		return tr
	}
	if c := tr.TLSClientConfig; c != nil && (c.InsecureSkipVerify || c.VerifyPeerCertificate != nil || c.VerifyConnection != nil || len(c.Certificates) > 0 || c.GetClientCertificate != nil) {
		return tr
	}
	h2 := &http2.Transport{DisableCompression: tr.DisableCompression, IdleConnTimeout: tr.IdleConnTimeout}
	h2.DialTLSContext = func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
		conn, err := tr.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			conn.Close()
			return nil, err
		}
		config := &utls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
		if c := tr.TLSClientConfig; c != nil {
			config.RootCAs = c.RootCAs
			if c.MinVersion > config.MinVersion {
				config.MinVersion = c.MinVersion
			}
			config.MaxVersion = c.MaxVersion
		}
		u := utls.UClient(conn, config, utls.HelloChrome_133)
		trace := httptrace.ContextClientTrace(ctx)
		if trace != nil && trace.TLSHandshakeStart != nil {
			trace.TLSHandshakeStart()
		}
		handshakeCtx, cancel := context.WithCancel(ctx)
		if tr.TLSHandshakeTimeout > 0 {
			cancel()
			handshakeCtx, cancel = context.WithTimeout(ctx, tr.TLSHandshakeTimeout)
		}
		defer cancel()
		err = u.HandshakeContext(handshakeCtx)
		s := u.ConnectionState()
		if trace != nil && trace.TLSHandshakeDone != nil {
			trace.TLSHandshakeDone(tls.ConnectionState{Version: s.Version, HandshakeComplete: s.HandshakeComplete, DidResume: s.DidResume, CipherSuite: s.CipherSuite, NegotiatedProtocol: s.NegotiatedProtocol, ServerName: s.ServerName, PeerCertificates: s.PeerCertificates, VerifiedChains: s.VerifiedChains}, err)
		}
		if err != nil {
			conn.Close()
			return nil, err
		}
		if s.NegotiatedProtocol != "h2" {
			conn.Close()
			return nil, fmt.Errorf("IPPure API did not negotiate HTTP/2")
		}
		return u, nil
	}
	return &ippureBrowserTransport{base: tr, h2: h2}
}

func ippureUsesBrowserSession(base http.RoundTripper) bool {
	if p, ok := base.(phaseTransport); ok {
		return ippureUsesBrowserSession(p.base)
	}
	_, ok := base.(*ippureBrowserTransport)
	return ok
}
