package publicservice

import (
	"context"
	"crypto/tls"
	"errors"
	"github.com/faceair/clash-speedtest/core/speedtester"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"sync"
	"time"
)

// Keep the failed request's stage with its error. Aggregate telemetry cannot
// identify which request failed in a composite or concurrent service check.
type serviceRequestError struct {
	phase string
	err   error
}

func (e *serviceRequestError) Error() string { return e.err.Error() }
func (e *serviceRequestError) Unwrap() error { return e.err }
func (e *serviceRequestError) Timeout() bool {
	var networkErr net.Error
	return errors.As(e.err, &networkErr) && networkErr.Timeout()
}
func (e *serviceRequestError) Temporary() bool {
	var networkErr net.Error
	return errors.As(e.err, &networkErr) && networkErr.Temporary()
}

func atRequestPhase(err error, phase string) error {
	if err == nil || phase == "" {
		return err
	}
	var existing *serviceRequestError
	if errors.As(err, &existing) {
		return err
	}
	return &serviceRequestError{phase: phase, err: err}
}

// requestPhases records only timings that the HTTP client can actually see.
// The proxy adapter can perform DNS and TCP work internally, so proxy dial is
// deliberately not called a TCP handshake or a DNS lookup.
type requestPhases struct {
	mu                 sync.Mutex
	proxyDial          time.Duration
	tls                time.Duration
	dns                time.Duration
	firstByte          time.Duration
	dialCount          int
	dialPending        int
	dialFailures       int
	dialError          string
	tlsCount           int
	tlsPending         int
	tlsFailures        int
	tlsError           string
	dnsCount           int
	byteCount          int
	connections        int
	reusedConnections  int
	requests           int
	written            int
	network            time.Duration
	body               time.Duration
	physicalConstraint bool
}

func (p *requestPhases) wrap(original http.RoundTripper, proxyDial bool) http.RoundTripper {
	if p == nil {
		if original == nil {
			return http.DefaultTransport
		}
		return original
	}
	if original == nil {
		original = http.DefaultTransport
	}
	if transport, ok := original.(*http.Transport); ok && proxyDial {
		clone := transport.Clone()
		if dial := clone.DialContext; dial != nil {
			clone.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				started := time.Now()
				// net/http may return on request cancellation before this dial ends.
				// Count admission now so the saved snapshot can show pending work.
				p.mu.Lock()
				p.dialCount++
				p.dialPending++
				p.mu.Unlock()
				conn, err := dial(ctx, network, addr)
				p.mu.Lock()
				p.proxyDial += time.Since(started)
				p.dialPending--
				if err != nil {
					p.dialFailures++
					p.dialError = sanitizeTransportError(err)
				}
				p.mu.Unlock()
				return conn, atRequestPhase(err, "proxy_connect")
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

// Preserve the HTTP client's explicit reconnect/cleanup operation through telemetry.
func (t phaseTransport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func (t phaseTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	if t.phases == nil {
		return base.RoundTrip(request)
	}
	started := time.Now()
	t.phases.mu.Lock()
	t.phases.requests++
	t.phases.mu.Unlock()
	var dnsStart, tlsStart time.Time
	var tlsFailed, wroteRequest, writeFailed bool
	var startMu sync.Mutex
	trace := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			t.phases.mu.Lock()
			t.phases.connections++
			if info.Reused {
				t.phases.reusedConnections++
			}
			t.phases.mu.Unlock()
		},
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
		TLSHandshakeStart: func() {
			startMu.Lock()
			tlsStart = time.Now()
			tlsFailed, wroteRequest, writeFailed = false, false, false
			startMu.Unlock()
			t.phases.mu.Lock()
			t.phases.tlsCount++
			t.phases.tlsPending++
			t.phases.mu.Unlock()
		},
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			startMu.Lock()
			start := tlsStart
			tlsStart = time.Time{}
			tlsFailed = err != nil
			startMu.Unlock()
			if start.IsZero() {
				return
			}
			t.phases.mu.Lock()
			t.phases.tls += time.Since(start)
			t.phases.tlsPending--
			if err != nil {
				t.phases.tlsFailures++
				t.phases.tlsError = sanitizeTransportError(err)
			}
			t.phases.mu.Unlock()
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			startMu.Lock()
			wroteRequest, writeFailed = info.Err == nil, info.Err != nil
			startMu.Unlock()
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
	response, err := base.RoundTrip(request.WithContext(httptrace.WithClientTrace(request.Context(), trace)))
	if err != nil {
		startMu.Lock()
		phase := ""
		switch {
		case !tlsStart.IsZero() || tlsFailed:
			phase = "tls_handshake"
		case writeFailed:
			phase = "request_write"
		case wroteRequest:
			phase = "response_headers"
		}
		startMu.Unlock()
		err = atRequestPhase(err, phase)
	}
	t.phases.mu.Lock()
	var physical *speedtester.PhysicalPathError
	if errors.As(err, &physical) {
		t.phases.physicalConstraint = true
	}
	t.phases.network += time.Since(started)
	t.phases.mu.Unlock()
	if response != nil && response.Body != nil {
		response.Body = timedServiceBody{ReadCloser: response.Body, phases: t.phases}
	}
	return response, err
}

func (p *requestPhases) attach(result *Result) {
	if p == nil || result == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.requests == 0 {
		return
	}
	if result.Details == nil {
		result.Details = map[string]string{}
	}
	// A recorded failed handshake before any HTTP write identifies the stage,
	// even when net/http reports the outer error as a generic EOF.
	if result.Outcome == "transport_error" && result.Details["request_failure_phase"] == "" && p.requests == 1 && p.written == 0 && p.tlsFailures > 0 && p.tlsPending == 0 {
		result.FailurePhase = "tls_handshake"
		result.ErrorMessage = "TLS 握手失败：" + p.tlsError
	}
	result.RequestCount = p.requests
	result.Details["request_count_kind"] = "http_transport_attempts_including_pre_header_failure"
	result.Details["http_requests_written"] = strconv.Itoa(p.written)
	result.Details["http_connections_observed"] = strconv.Itoa(p.connections)
	result.Details["http_connections_reused"] = strconv.Itoa(p.reusedConnections)
	result.Details["connection_observation_scope"] = "http_connections_not_node_sessions"
	result.Details["connection_observation_note"] = "按 HTTP 请求获取连接的事件计数，不是独立连接数；不证明节点底层会话重连"
	if p.physicalConstraint {
		result.Details["physical_constraint_failure"] = "true"
	}
	result.Details["execution_status"] = "executed"
	result.Details["conclusion"] = serviceEvidenceConclusion(*result)
	format := func(d time.Duration) string {
		return strconv.FormatFloat(float64(d)/float64(time.Millisecond), 'f', 1, 64)
	}
	result.Details["request_headers_network_ms"] = format(p.network)
	result.Details["response_body_read_ms"] = format(p.body)
	if p.dialCount > 0 {
		if p.dialCount > p.dialPending {
			result.Details["proxy_connect_ms"] = format(p.proxyDial)
		}
		result.Details["proxy_connect_attempts"] = strconv.Itoa(p.dialCount)
		result.Details["proxy_connect_pending"] = strconv.Itoa(p.dialPending)
		result.Details["proxy_connect_failures"] = strconv.Itoa(p.dialFailures)
		if p.dialError != "" {
			result.Details["proxy_connect_error"] = p.dialError
		}
	}
	if p.tlsCount > 0 {
		if p.tlsCount > p.tlsPending {
			result.Details["tls_handshake_ms"] = format(p.tls)
		}
		result.Details["tls_handshake_attempts"] = strconv.Itoa(p.tlsCount)
		result.Details["tls_handshake_pending"] = strconv.Itoa(p.tlsPending)
		result.Details["tls_handshake_failures"] = strconv.Itoa(p.tlsFailures)
		if p.tlsError != "" {
			result.Details["tls_handshake_error"] = p.tlsError
		}
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
	if p.dialPending > 0 {
		result.Details["phase_timing_note"] += "；快照时仍有拨号未结束，其耗时和结果尚未计入"
	}
	if p.tlsPending > 0 {
		result.Details["phase_timing_note"] += "；快照时仍有 TLS 握手未结束，其耗时和结果尚未计入"
	}
}

type timedServiceBody struct {
	io.ReadCloser
	phases *requestPhases
}

func (b timedServiceBody) Read(buffer []byte) (int, error) {
	if b.ReadCloser == nil {
		return 0, io.EOF
	}
	start := time.Now()
	n, err := b.ReadCloser.Read(buffer)
	if b.phases != nil {
		b.phases.mu.Lock()
		b.phases.body += time.Since(start)
		b.phases.mu.Unlock()
	}
	if err != nil && err != io.EOF {
		err = atRequestPhase(err, "response_read")
	}
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
