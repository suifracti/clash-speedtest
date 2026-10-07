package publicservice

import (
	"context"
	"crypto/tls"
	"io"
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
	requests  int
	written   int
	network   time.Duration
	body      time.Duration
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
	t.phases.mu.Lock()
	t.phases.requests++
	t.phases.mu.Unlock()
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
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err == nil {
				t.phases.mu.Lock()
				t.phases.written++
				t.phases.mu.Unlock()
			}
		},
		GotFirstResponseByte: func() {
			t.phases.mu.Lock()
			t.phases.firstByte += time.Since(started)
			t.phases.byteCount++
			t.phases.mu.Unlock()
		},
	}
	response, err := t.base.RoundTrip(request.WithContext(httptrace.WithClientTrace(request.Context(), trace)))
	t.phases.mu.Lock()
	t.phases.network += time.Since(started)
	t.phases.mu.Unlock()
	if response != nil && response.Body != nil {
		response.Body = timedServiceBody{ReadCloser: response.Body, phases: t.phases}
	}
	return response, err
}

func (p *requestPhases) attach(result *Result) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.requests == 0 {
		return
	}
	if result.Details == nil {
		result.Details = map[string]string{}
	}
	result.RequestCount = p.requests
	result.Details["request_count_kind"] = "http_transport_attempts_including_pre_header_failure"
	result.Details["http_requests_written"] = strconv.Itoa(p.written)
	result.Details["execution_status"] = "executed"
	result.Details["conclusion"] = serviceEvidenceConclusion(*result)
	format := func(d time.Duration) string {
		return strconv.FormatFloat(float64(d)/float64(time.Millisecond), 'f', 1, 64)
	}
	result.Details["request_headers_network_ms"] = format(p.network)
	result.Details["response_body_read_ms"] = format(p.body)
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

type timedServiceBody struct {
	io.ReadCloser
	phases *requestPhases
}

func (b timedServiceBody) Read(buffer []byte) (int, error) {
	start := time.Now()
	n, err := b.ReadCloser.Read(buffer)
	b.phases.mu.Lock()
	b.phases.body += time.Since(start)
	b.phases.mu.Unlock()
	return n, err
}
func serviceEvidenceConclusion(r Result) string {
	switch r.Outcome {
	case "reachable":
		return "http_reachable_business_and_unlock_unverified"
	case "matched":
		return "endpoint_rule_matched_only"
	case "profiled":
		return "endpoint_profile_observed_only"
	case "unlocked":
		return "unlock_rule_matched_not_playback_verified"
	default:
		return "unconfirmed"
	}
}
