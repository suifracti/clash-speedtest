package speedtester

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultDownloadStreamTimeout = 10 * time.Second
	MaximumDownloadStreamTimeout = 30 * time.Second
	DefaultDownloadStreamBytes   = int64(10 << 20)
	MaximumDownloadStreamBytes   = int64(100 << 20)
	DefaultDownloadSampleBytes   = int64(256 << 10)
)

const defaultDownloadSampleInterval = 100 * time.Millisecond

type DownloadStreamOptions struct {
	MaximumBytes        int64
	MaximumDuration     time.Duration
	SampleEveryBytes    int64
	SampleEveryDuration time.Duration
}

type DownloadStreamSample struct {
	ElapsedNS       int64    `json:"elapsed_ns"`
	IntervalNS      int64    `json:"interval_ns"`
	DeltaBytes      int64    `json:"delta_bytes"`
	CumulativeBytes int64    `json:"cumulative_bytes"`
	SpeedMbps       *float64 `json:"speed_mbps,omitempty"`
}

type DownloadStreamResult struct {
	PhaseTimingsNS     map[string]int64       `json:"phase_timings_ns,omitempty"`
	Phase              string                 `json:"phase,omitempty"`
	ErrorClass         string                 `json:"error_class,omitempty"`
	EndReason          string                 `json:"end_reason,omitempty"`
	PartialMeasurement bool                   `json:"partial_measurement,omitempty"`
	RetryAfterSeconds  int64                  `json:"retry_after_seconds,omitempty"`
	Outcome            string                 `json:"outcome"`
	TargetURL          string                 `json:"target_url"`
	HTTPStatus         *int                   `json:"http_status,omitempty"`
	BytesRead          int64                  `json:"bytes_read"`
	StartedAt          time.Time              `json:"started_at"`
	FinishedAt         time.Time              `json:"finished_at"`
	DurationNS         int64                  `json:"duration_ns"`
	FailurePhase       string                 `json:"failure_phase,omitempty"`
	ErrorMessage       string                 `json:"error_message,omitempty"`
	Samples            []DownloadStreamSample `json:"samples"`
}

type DownloadStreamProgress func(sample DownloadStreamSample, bytesRead int64)

// DownloadStreamTarget derives a fixed download-server endpoint from the
// speedtester target. The request asks for one byte more than the local read
// budget so hitting MaximumBytes is unambiguously a local stop condition.
func (st *SpeedTester) DownloadStreamTarget(maximumBytes int64) (string, error) {
	if st == nil || st.serverMode != serverModeDownloadServer || strings.TrimSpace(st.serverBaseURL) == "" {
		return "", fmt.Errorf("download stream requires the configured download server")
	}
	if maximumBytes < 1 || maximumBytes >= MaximumDownloadStreamBytes+1 {
		return "", fmt.Errorf("download stream byte limit must be between 1 and %d", MaximumDownloadStreamBytes)
	}
	parsed, err := url.Parse(st.serverBaseURL)
	if err != nil {
		return "", fmt.Errorf("parse download server URL: %w", err)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/__down"
	query := parsed.Query()
	query.Set("bytes", strconv.FormatInt(maximumBytes+1, 10))
	parsed.RawQuery = query.Encode()
	parsed.Fragment = ""
	return parsed.String(), nil
}

// NewNodeHTTPClient uses the same per-node proxy dial path as the existing
// speedtester. Callers must still bind every request to a context deadline.
func (st *SpeedTester) NewNodeHTTPClient(proxy *CProxy, timeout time.Duration) (*http.Client, error) {
	if st == nil || proxy == nil || proxy.Proxy == nil {
		return nil, fmt.Errorf("download node proxy is unavailable")
	}
	return st.createClient(proxy.Proxy, timeout), nil
}

// MeasureDownloadStream reads a bounded response through an already isolated
// node client. Samples are emitted only from bytes returned by Body.Read and
// elapsed time from the monotonic component of time.Now; no final-speed
// interpolation is used to create points.
func (st *SpeedTester) MeasureDownloadStream(ctx context.Context, client *http.Client, options DownloadStreamOptions, progress DownloadStreamProgress) DownloadStreamResult {
	return st.measureDownloadStream(ctx, client, options, progress, time.Now)
}

func (st *SpeedTester) measureDownloadStream(ctx context.Context, client *http.Client, options DownloadStreamOptions, progress DownloadStreamProgress, now func() time.Time) DownloadStreamResult {
	if now == nil {
		now = time.Now
	}
	started := now()
	result := DownloadStreamResult{StartedAt: started.UTC(), Samples: make([]DownloadStreamSample, 0)}
	var phaseMu sync.Mutex
	phase := "request_setup"
	phaseStarted := time.Now()
	phases := map[string]int64{}
	setPhase := func(v string) {
		phaseMu.Lock()
		if phase != v {
			phases[phase] += time.Since(phaseStarted).Nanoseconds()
			phaseStarted = time.Now()
			phase = v
		}
		phaseMu.Unlock()
	}
	getPhase := func() string { phaseMu.Lock(); defer phaseMu.Unlock(); return phase }
	finish := func() DownloadStreamResult {
		phaseMu.Lock()
		phases[phase] += time.Since(phaseStarted).Nanoseconds()
		result.Phase = phase
		result.PhaseTimingsNS = make(map[string]int64, len(phases))
		for k, v := range phases {
			result.PhaseTimingsNS[k] = v
		}
		phaseMu.Unlock()
		result.EndReason = result.Outcome
		result.PartialMeasurement = result.Outcome == "time_limit" && result.BytesRead > 0
		ended := now()
		result.FinishedAt = ended.UTC()
		result.DurationNS = ended.Sub(started).Nanoseconds()
		if result.DurationNS < 0 {
			result.DurationNS = 0
		}
		return result
	}
	if st == nil {
		result.Outcome = "connection_failed"
		result.FailurePhase = "request_setup"
		result.ErrorMessage = "下载测试配置不可用"
		return finish()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if options.MaximumBytes < 1 || options.MaximumBytes > MaximumDownloadStreamBytes {
		result.Outcome = "connection_failed"
		result.FailurePhase = "request_setup"
		result.ErrorMessage = "下载读取上限无效"
		return finish()
	}
	if options.MaximumDuration <= 0 || options.MaximumDuration > MaximumDownloadStreamTimeout {
		result.Outcome = "connection_failed"
		result.FailurePhase = "request_setup"
		result.ErrorMessage = "下载时长上限无效"
		return finish()
	}
	if client == nil {
		result.Outcome = "connection_failed"
		result.FailurePhase = "proxy_setup"
		result.ErrorMessage = "无法建立节点隔离下载连接"
		return finish()
	}
	if options.SampleEveryBytes <= 0 {
		options.SampleEveryBytes = DefaultDownloadSampleBytes
	}
	if options.SampleEveryDuration <= 0 {
		options.SampleEveryDuration = defaultDownloadSampleInterval
	}
	target, err := st.DownloadStreamTarget(options.MaximumBytes)
	if err != nil {
		result.Outcome = "connection_failed"
		result.FailurePhase = "request_setup"
		result.ErrorMessage = "无法创建固定下载目标"
		return finish()
	}
	result.TargetURL = target
	runCtx, cancel := context.WithTimeout(ctx, options.MaximumDuration)
	defer cancel()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	request, err := http.NewRequestWithContext(runCtx, http.MethodGet, target, nil)
	if err != nil {
		result.Outcome = "connection_failed"
		result.FailurePhase = "request_setup"
		result.ErrorMessage = "无法创建固定下载请求"
		return finish()
	}
	request.Header.Set("User-Agent", "clash-speedtest")
	request.Header.Set("Accept-Encoding", "identity")
	// A proxy adapter may hide DNS/TCP/handshake. Name the combined span
	// honestly; Darwin's verified socket path supplies finer boundaries.
	setPhase("proxy_connect_and_handshake")
	runCtx = context.WithValue(runCtx, downloadPhaseKey{}, setPhase)
	request = request.WithContext(runCtx)
	localClient := *client
	if transport, ok := client.Transport.(*http.Transport); ok && transport.DialContext != nil {
		clone := transport.Clone()
		dial := clone.DialContext
		clone.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			setPhase("proxy_connect_and_handshake")
			return dial(ctx, network, addr)
		}
		localClient.Transport = clone
		defer clone.CloseIdleConnections()
	}
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) { setPhase("dns") },
		DNSDone: func(info httptrace.DNSDoneInfo) {
			if info.Err == nil {
				setPhase("connection")
			}
		},
		ConnectStart:      func(string, string) { setPhase("connection") },
		TLSHandshakeStart: func() { setPhase("tls") },
		TLSHandshakeDone:  func(_ tls.ConnectionState, _ error) { setPhase("response_headers") },
		GotConn:           func(httptrace.GotConnInfo) { setPhase("response_headers") },
	}))
	response, err := localClient.Do(request)
	if err != nil {
		result.ErrorClass = downloadErrorClass(err)
		if result.ErrorClass == "tls" || result.ErrorClass == "dns" {
			setPhase(result.ErrorClass)
		}
		if ctx.Err() == context.Canceled || errors.Is(err, context.Canceled) {
			result.Outcome = "user_cancelled"
			result.FailurePhase = "cancelled"
			result.ErrorMessage = "下载已由用户取消"
		} else if runCtx.Err() == context.DeadlineExceeded || errors.Is(err, context.DeadlineExceeded) {
			result.Outcome = "time_limit"
			result.FailurePhase = getPhase()
			result.ErrorMessage = "达到下载时长上限"
		} else if result.ErrorClass == "local_path_unverified" {
			result.Outcome = "local_path_unverified"
			result.FailurePhase = "local_egress"
			result.ErrorMessage = "本机物理出口未能确认；不作为节点故障或有效速度读数"
		} else {
			result.Outcome = "connection_failed"
			result.FailurePhase = getPhase()
			result.ErrorMessage = "无法通过所选节点建立下载连接"
		}
		return finish()
	}
	setPhase("response_status")
	defer response.Body.Close()
	status := response.StatusCode
	result.HTTPStatus = &status
	if status >= 300 && status < 400 {
		result.Outcome = "redirect"
		result.FailurePhase = "response_status"
		result.ErrorMessage = "下载目标返回重定向；未跟随"
		return finish()
	}
	if status == http.StatusTooManyRequests {
		result.Outcome = "source_rate_limited"
		result.FailurePhase = "response_status"
		result.ErrorClass = "source_rate_limit"
		result.ErrorMessage = "测速源返回 HTTP 429 限流；未立即重试，不判为节点故障"
		if seconds, e := strconv.ParseInt(response.Header.Get("Retry-After"), 10, 64); e == nil && seconds > 0 {
			result.RetryAfterSeconds = seconds
		} else if at, e := http.ParseTime(response.Header.Get("Retry-After")); e == nil {
			result.RetryAfterSeconds = int64(time.Until(at).Seconds())
			if result.RetryAfterSeconds < 0 {
				result.RetryAfterSeconds = 0
			}
		}
		return finish()
	}
	if status != http.StatusOK {
		result.Outcome = "http_rejected"
		result.FailurePhase = "response_status"
		result.ErrorMessage = fmt.Sprintf("下载目标返回 HTTP %d", status)
		return finish()
	}

	setPhase("response_body")
	sampler := downloadStreamSampler{started: started, everyBytes: options.SampleEveryBytes, everyDuration: options.SampleEveryDuration}
	buffer := make([]byte, 32*1024)
	for {
		remaining := options.MaximumBytes - result.BytesRead
		if remaining <= 0 {
			result.Outcome = "byte_limit"
			result.FailurePhase = ""
			result.ErrorMessage = ""
			cancel()
			break
		}
		readBuffer := buffer
		if int64(len(readBuffer)) > remaining {
			readBuffer = readBuffer[:int(remaining)]
		}
		read, readErr := response.Body.Read(readBuffer)
		if read > 0 {
			result.BytesRead += int64(read)
			current := now()
			if sample, ok := sampler.add(result.BytesRead, current, result.BytesRead >= options.MaximumBytes); ok {
				result.Samples = append(result.Samples, sample)
				if progress != nil {
					progress(sample, result.BytesRead)
				}
			}
			if result.BytesRead >= options.MaximumBytes {
				result.Outcome = "byte_limit"
				result.FailurePhase = ""
				result.ErrorMessage = ""
				cancel()
				break
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				result.ErrorClass = downloadErrorClass(readErr)
			}
			if ctx.Err() == context.Canceled || errors.Is(readErr, context.Canceled) {
				result.Outcome = "user_cancelled"
				result.FailurePhase = "cancelled"
				result.ErrorMessage = "下载已由用户取消"
			} else if runCtx.Err() == context.DeadlineExceeded || errors.Is(readErr, context.DeadlineExceeded) {
				result.Outcome = "time_limit"
				result.FailurePhase = "time_limit"
				result.ErrorMessage = "达到下载时长上限"
			} else if errors.Is(readErr, io.EOF) {
				break
			} else if errors.Is(readErr, context.Canceled) {
				result.Outcome = "user_cancelled"
				result.FailurePhase = "cancelled"
				result.ErrorMessage = "下载已取消"
			} else {
				result.Outcome = "transfer_interrupted"
				result.FailurePhase = "response_body"
				result.ErrorMessage = "下载响应在传输中断开"
			}
			break
		}
		if ctx.Err() == context.Canceled {
			result.Outcome = "user_cancelled"
			result.FailurePhase = "cancelled"
			result.ErrorMessage = "下载已由用户取消"
			break
		}
		if runCtx.Err() == context.DeadlineExceeded {
			result.Outcome = "time_limit"
			result.FailurePhase = "time_limit"
			result.ErrorMessage = "达到下载时长上限"
			break
		}
		if read == 0 {
			continue
		}
	}
	if result.Outcome == "" {
		result.Outcome = "completed"
	}
	if sample, ok := sampler.add(result.BytesRead, now(), true); ok {
		result.Samples = append(result.Samples, sample)
		if progress != nil {
			progress(sample, result.BytesRead)
		}
	}
	result.Samples = append([]DownloadStreamSample(nil), result.Samples...)
	return finish()
}

type downloadStreamSampler struct {
	started       time.Time
	lastAt        time.Time
	lastElapsed   int64
	lastBytes     int64
	everyBytes    int64
	everyDuration time.Duration
}

func (s *downloadStreamSampler) add(totalBytes int64, now time.Time, force bool) (DownloadStreamSample, bool) {
	if totalBytes <= s.lastBytes {
		return DownloadStreamSample{}, false
	}
	elapsed := now.Sub(s.started).Nanoseconds()
	if elapsed < 0 {
		elapsed = 0
	}
	if !force && s.lastBytes == 0 && totalBytes < s.everyBytes && now.Sub(s.started) < s.everyDuration {
		return DownloadStreamSample{}, false
	}
	if !force && s.lastBytes > 0 && totalBytes-s.lastBytes < s.everyBytes && now.Sub(s.lastAt) < s.everyDuration {
		return DownloadStreamSample{}, false
	}
	interval := elapsed - s.lastElapsed
	if interval < 0 {
		interval = 0
	}
	sample := DownloadStreamSample{
		ElapsedNS: elapsed, IntervalNS: interval, DeltaBytes: totalBytes - s.lastBytes, CumulativeBytes: totalBytes,
	}
	if interval > 0 {
		mbps := float64(sample.DeltaBytes) * 8000 / float64(interval)
		sample.SpeedMbps = &mbps
	}
	s.lastAt = now
	s.lastElapsed = elapsed
	s.lastBytes = totalBytes
	return sample, true
}

// Preserve useful typed evidence without storing raw exceptions containing
// proxy addresses, credentials or private URL query parameters.
func downloadErrorClass(err error) string {
	var local *PhysicalPathError
	if errors.As(err, &local) {
		return "local_path_unverified"
	}
	var dns *net.DNSError
	var header tls.RecordHeaderError
	var cert *tls.CertificateVerificationError
	var unknown x509.UnknownAuthorityError
	if errors.As(err, &dns) {
		return "dns"
	}
	if errors.As(err, &header) || errors.As(err, &cert) || errors.As(err, &unknown) {
		return "tls"
	}
	var network net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &network) && network.Timeout()) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "failed to create session"):
		return "proxy_session"
	case strings.Contains(message, "connection reset"):
		return "connection_reset"
	case strings.Contains(message, "connection refused"):
		return "connection_refused"
	case strings.Contains(message, "tls:") || strings.Contains(message, "x509:"):
		return "tls"
	case errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF):
		return "unexpected_eof"
	}
	return "unknown_transport"
}

type downloadPhaseKey struct{}

func markDownloadPhase(ctx context.Context, phase string) {
	if mark, ok := ctx.Value(downloadPhaseKey{}).(func(string)); ok {
		mark(phase)
	}
}
