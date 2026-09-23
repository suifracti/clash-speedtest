package speedtester

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultDownloadStreamTimeout = 10 * time.Second
	MaximumDownloadStreamTimeout = 30 * time.Second
	DefaultDownloadStreamBytes   = int64(20 << 20)
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
	Outcome      string                 `json:"outcome"`
	TargetURL    string                 `json:"target_url"`
	HTTPStatus   *int                   `json:"http_status,omitempty"`
	BytesRead    int64                  `json:"bytes_read"`
	StartedAt    time.Time              `json:"started_at"`
	FinishedAt   time.Time              `json:"finished_at"`
	DurationNS   int64                  `json:"duration_ns"`
	FailurePhase string                 `json:"failure_phase,omitempty"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	Samples      []DownloadStreamSample `json:"samples"`
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
	finish := func() DownloadStreamResult {
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
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() == context.Canceled || errors.Is(err, context.Canceled) {
			result.Outcome = "user_cancelled"
			result.FailurePhase = "cancelled"
			result.ErrorMessage = "下载已由用户取消"
		} else if runCtx.Err() == context.DeadlineExceeded || errors.Is(err, context.DeadlineExceeded) {
			result.Outcome = "time_limit"
			result.FailurePhase = "time_limit"
			result.ErrorMessage = "达到下载时长上限"
		} else {
			result.Outcome = "connection_failed"
			result.FailurePhase = "connection"
			result.ErrorMessage = "无法通过所选节点建立下载连接"
		}
		return finish()
	}
	defer response.Body.Close()
	status := response.StatusCode
	result.HTTPStatus = &status
	if status >= 300 && status < 400 {
		result.Outcome = "redirect"
		result.FailurePhase = "response_status"
		result.ErrorMessage = "下载目标返回重定向；未跟随"
		return finish()
	}
	if status != http.StatusOK {
		result.Outcome = "http_rejected"
		result.FailurePhase = "response_status"
		result.ErrorMessage = fmt.Sprintf("下载目标返回 HTTP %d", status)
		return finish()
	}

	sampler := downloadStreamSampler{started: started, everyBytes: options.SampleEveryBytes, everyDuration: options.SampleEveryDuration}
	buffer := make([]byte, 32*1024)
	for {
		remaining := options.MaximumBytes - result.BytesRead
		if remaining <= 0 {
			result.Outcome = "byte_limit"
			result.FailurePhase = "byte_limit"
			result.ErrorMessage = "达到下载响应体读取上限"
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
				result.FailurePhase = "byte_limit"
				result.ErrorMessage = "达到下载响应体读取上限"
				cancel()
				break
			}
		}
		if readErr != nil {
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
