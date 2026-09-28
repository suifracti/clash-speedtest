package publicservice

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"sync"
	"time"
)

// requestPhases records only timings that the HTTP client can actually see.
// The proxy adapter can perform DNS and TCP work internally, so proxy dial is
// deliberately not called a TCP handshake or a DNS lookup.
type requestPhases struct {
	mu        sync.Mutex
	proxyDial time.Duration
	tls       time.Duration
	dns       time.Duration
	firstByte time.Duration
	dialCount int
	tlsCount  int
	dnsCount  int
	byteCount int
}

func (p *requestPhases) wrap(original http.RoundTripper, proxyDial bool) http.RoundTripper {
	if original == nil {
		original = http.DefaultTransport
	}
	if transport, ok := original.(*http.Transport); ok && proxyDial {
		clone := transport.Clone()
		if dial := clone.DialContext; dial != nil {
			clone.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				started := time.Now()
				conn, err := dial(ctx, network, addr)
				p.mu.Lock()
				p.proxyDial += time.Since(started)
				p.dialCount++
				p.mu.Unlock()
				return conn, err
			}
		}
		original = clone
	}
	return phaseTransport{base: original, phases: p}
}

type phaseTransport struct {
	base   http.RoundTripper
	phases *requestPhases
}

func (t phaseTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	started := time.Now()
	var dnsStart, tlsStart time.Time
	var startMu sync.Mutex
	trace := &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) { startMu.Lock(); dnsStart = time.Now(); startMu.Unlock() },
		DNSDone: func(httptrace.DNSDoneInfo) {
			startMu.Lock()
			start := dnsStart
			startMu.Unlock()
			if start.IsZero() {
				return
			}
			t.phases.mu.Lock()
			t.phases.dns += time.Since(start)
			t.phases.dnsCount++
			t.phases.mu.Unlock()
		},
		TLSHandshakeStart: func() { startMu.Lock(); tlsStart = time.Now(); startMu.Unlock() },
		TLSHandshakeDone: func(_ tls.ConnectionState, _ error) {
			startMu.Lock()
			start := tlsStart
			startMu.Unlock()
			if start.IsZero() {
				return
			}
			t.phases.mu.Lock()
			t.phases.tls += time.Since(start)
			t.phases.tlsCount++
			t.phases.mu.Unlock()
		},
		GotFirstResponseByte: func() {
			t.phases.mu.Lock()
			t.phases.firstByte += time.Since(started)
			t.phases.byteCount++
			t.phases.mu.Unlock()
		},
	}
	return t.base.RoundTrip(request.WithContext(httptrace.WithClientTrace(request.Context(), trace)))
}

func (p *requestPhases) attach(result *Result) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dialCount+p.tlsCount+p.dnsCount+p.byteCount == 0 {
		return
	}
	if result.Details == nil {
		result.Details = map[string]string{}
	}
	format := func(d time.Duration) string {
		return strconv.FormatFloat(float64(d)/float64(time.Millisecond), 'f', 1, 64)
	}
	if p.dialCount > 0 {
		result.Details["proxy_connect_ms"] = format(p.proxyDial)
	}
	if p.tlsCount > 0 {
		result.Details["tls_handshake_ms"] = format(p.tls)
	}
	if p.byteCount > 0 {
		result.Details["first_response_byte_ms"] = format(p.firstByte)
	}
	if p.dnsCount > 0 {
		result.Details["dns_ms"] = format(p.dns)
	} else if p.dialCount > 0 {
		result.Details["dns_timing_note"] = "由节点代理内部处理，无法单独计时"
	}
	result.Details["phase_timing_note"] = "多请求时为阶段累计；首字节时间包含建连与 TLS，不能与其它阶段相加"
}
