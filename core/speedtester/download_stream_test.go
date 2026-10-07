package speedtester

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type downloadRoundTripper func(*http.Request) (*http.Response, error)

func (f downloadRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func newDownloadStreamTester(t *testing.T) *SpeedTester {
	t.Helper()
	st, err := New(&Config{ServerURL: DefaultSpeedServer, Mode: SpeedModeDownload, Metrics: MetricSet{Download: true}, Concurrent: 1})
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestMeasureDownloadStreamUsesActualBytesAndBitsPerSecond(t *testing.T) {
	st := newDownloadStreamTester(t)
	payload := strings.Repeat("x", 1_000_000)
	client := &http.Client{Transport: downloadRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.String() != DefaultSpeedServer+"/__down?bytes=2097153" || r.Header.Get("Accept-Encoding") != "identity" {
			t.Fatalf("unexpected fixed request: %s %s headers=%v", r.Method, r.URL, r.Header)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(payload)), Request: r}, nil
	})}
	base := time.Unix(1_000, 0)
	clockCalls := 0
	now := func() time.Time {
		clockCalls++
		if clockCalls == 1 {
			return base
		}
		return base.Add(time.Second)
	}
	result := st.measureDownloadStream(context.Background(), client, DownloadStreamOptions{MaximumBytes: 2 << 20, MaximumDuration: time.Second, SampleEveryBytes: 2 << 20, SampleEveryDuration: 24 * time.Hour}, nil, now)
	if result.Outcome != "completed" || result.BytesRead != int64(len(payload)) {
		t.Fatalf("response body measurement mismatch: outcome=%s bytes=%d", result.Outcome, result.BytesRead)
	}
	if len(result.Samples) != 1 || result.Samples[0].CumulativeBytes != int64(len(payload)) || result.Samples[0].DeltaBytes != int64(len(payload)) {
		t.Fatalf("sample was not derived from the actual body bytes: %+v", result.Samples)
	}
	if got := *result.Samples[0].SpeedMbps; got != 8 {
		t.Fatalf("1,000,000 bytes over one second must be 8 Mbps, got %v", got)
	}
}

func TestMeasureDownloadStreamStopsAtByteLimitAndDoesNotFollowRedirect(t *testing.T) {
	st := newDownloadStreamTester(t)
	var calls int
	client := &http.Client{Transport: downloadRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"https://other.example/file"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(strings.Repeat("y", 2000))), Request: r}, nil
	})}
	redirect := st.MeasureDownloadStream(context.Background(), client, DownloadStreamOptions{MaximumBytes: 1000, MaximumDuration: time.Second}, nil)
	if redirect.Outcome != "redirect" || calls != 1 || redirect.BytesRead != 0 {
		t.Fatalf("redirect must be reported without a follow-up request: outcome=%s calls=%d bytes=%d", redirect.Outcome, calls, redirect.BytesRead)
	}
	client = &http.Client{Transport: downloadRoundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(strings.Repeat("z", 2000))), Request: r}, nil
	})}
	limited := st.MeasureDownloadStream(context.Background(), client, DownloadStreamOptions{MaximumBytes: 1000, MaximumDuration: time.Second}, nil)
	if limited.Outcome != "byte_limit" || limited.BytesRead != 1000 {
		t.Fatalf("read must stop exactly at byte cap: outcome=%s bytes=%d", limited.Outcome, limited.BytesRead)
	}
}

type contextBlockingBody struct {
	ctx     context.Context
	entered chan struct{}
	once    sync.Once
}

func (b *contextBlockingBody) Read([]byte) (int, error) {
	b.once.Do(func() { close(b.entered) })
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}
func (*contextBlockingBody) Close() error { return nil }

func TestMeasureDownloadStreamCancelAbortsBodyRead(t *testing.T) {
	st := newDownloadStreamTester(t)
	callerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	client := &http.Client{Transport: downloadRoundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: &contextBlockingBody{ctx: r.Context(), entered: entered}, Request: r}, nil
	})}
	done := make(chan DownloadStreamResult, 1)
	go func() {
		done <- st.MeasureDownloadStream(callerCtx, client, DownloadStreamOptions{MaximumBytes: 100, MaximumDuration: 5 * time.Second}, nil)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("request body read did not start")
	}
	cancel()
	select {
	case result := <-done:
		if result.Outcome != "user_cancelled" || result.BytesRead != 0 {
			t.Fatalf("cancellation must stop the actual response read: %+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled response read did not return")
	}
}

func TestMeasureDownloadStreamClassifiesDurationLimit(t *testing.T) {
	st := newDownloadStreamTester(t)
	entered := make(chan struct{})
	client := &http.Client{Transport: downloadRoundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: &contextBlockingBody{ctx: r.Context(), entered: entered}, Request: r}, nil
	})}
	result := make(chan DownloadStreamResult, 1)
	go func() {
		result <- st.MeasureDownloadStream(context.Background(), client, DownloadStreamOptions{MaximumBytes: 100, MaximumDuration: 30 * time.Millisecond}, nil)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("request body read did not start")
	}
	select {
	case got := <-result:
		if got.Outcome != "time_limit" || got.BytesRead != 0 {
			t.Fatalf("duration cap must stop the actual response read: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("duration-capped response read did not return")
	}
}

type failingDownloadBody struct{}

func (failingDownloadBody) Read([]byte) (int, error) {
	return 0, errors.New("transport detail must not be exposed")
}
func (failingDownloadBody) Close() error { return nil }

func TestMeasureDownloadStreamSeparatesHTTPRejectionConnectionAndBodyFailure(t *testing.T) {
	st := newDownloadStreamTester(t)
	client := &http.Client{Transport: downloadRoundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusForbidden, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})}
	rejected := st.MeasureDownloadStream(context.Background(), client, DownloadStreamOptions{MaximumBytes: 100, MaximumDuration: time.Second}, nil)
	if rejected.Outcome != "http_rejected" || rejected.HTTPStatus == nil || *rejected.HTTPStatus != http.StatusForbidden {
		t.Fatalf("HTTP refusal result = %+v", rejected)
	}
	client = &http.Client{Transport: downloadRoundTripper(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("proxy credentials must not be exposed")
	})}
	connection := st.MeasureDownloadStream(context.Background(), client, DownloadStreamOptions{MaximumBytes: 100, MaximumDuration: time.Second}, nil)
	if connection.Outcome != "connection_failed" || strings.Contains(connection.ErrorMessage, "credentials") {
		t.Fatalf("connection failure must have a safe category: %+v", connection)
	}
	client = &http.Client{Transport: downloadRoundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: failingDownloadBody{}, Request: r}, nil
	})}
	interrupted := st.MeasureDownloadStream(context.Background(), client, DownloadStreamOptions{MaximumBytes: 100, MaximumDuration: time.Second}, nil)
	if interrupted.Outcome != "transfer_interrupted" || interrupted.FailurePhase != "response_body" || strings.Contains(interrupted.ErrorMessage, "transport detail") {
		t.Fatalf("body failure must be distinguished and sanitized: %+v", interrupted)
	}
}

func TestDownloadDiagnosticSeparatesSourceRateLimitAndTypedTLSFailure(t *testing.T) {
	st := newDownloadStreamTester(t)
	calls := 0
	client := &http.Client{Transport: downloadRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"120"}}, Body: io.NopCloser(strings.NewReader("limited")), Request: r}, nil
	})}
	got := st.MeasureDownloadStream(context.Background(), client, DownloadStreamOptions{MaximumBytes: 10 << 20, MaximumDuration: time.Second}, nil)
	if got.Outcome != "source_rate_limited" || got.RetryAfterSeconds != 120 || got.BytesRead != 0 || calls != 1 {
		t.Fatalf("rate limit misreported or retried: %+v calls=%d", got, calls)
	}
	client = &http.Client{Transport: downloadRoundTripper(func(r *http.Request) (*http.Response, error) {
		return nil, tls.RecordHeaderError{Msg: "remote private URL must not leak"}
	})}
	got = st.MeasureDownloadStream(context.Background(), client, DownloadStreamOptions{MaximumBytes: 10 << 20, MaximumDuration: time.Second}, nil)
	if got.ErrorClass != "tls" || got.Phase != "tls" || strings.Contains(got.ErrorMessage, "private") {
		t.Fatalf("TLS diagnosis lost or leaked: %+v", got)
	}
}
func TestDownloadPartialTimeoutPreservesBytesAndMeasurementCondition(t *testing.T) {
	st := newDownloadStreamTester(t)
	client := &http.Client{Transport: downloadRoundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: structReadCloser{Reader: io.MultiReader(strings.NewReader("abcd"), &contextBlockingBody{ctx: r.Context(), entered: make(chan struct{})})}, Request: r}, nil
	})}
	got := st.MeasureDownloadStream(context.Background(), client, DownloadStreamOptions{MaximumBytes: 10 << 20, MaximumDuration: 15 * time.Millisecond}, nil)
	if got.Outcome != "time_limit" || !got.PartialMeasurement || got.BytesRead != 4 || got.Phase != "response_body" || got.EndReason != "time_limit" {
		t.Fatalf("partial timed read lost: %+v", got)
	}
}

type structReadCloser struct{ io.Reader }

func (structReadCloser) Close() error { return nil }

func TestFullReadBudgetIsNormalCompletionWithoutFailure(t *testing.T) {
	st := newDownloadStreamTester(t)
	client := &http.Client{Transport: downloadRoundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(strings.Repeat("x", 1024))), Request: r}, nil
	})}
	result := st.MeasureDownloadStream(context.Background(), client, DownloadStreamOptions{MaximumBytes: 512, MaximumDuration: time.Second}, nil)
	if result.Outcome != "byte_limit" || result.BytesRead != 512 || result.FailurePhase != "" || result.ErrorMessage != "" {
		t.Fatalf("normal bounded completion marked as failure: %+v", result)
	}
}

func TestDownloadRecordsObservableStageDurations(t *testing.T) {
	st := newDownloadStreamTester(t)
	client := &http.Client{Transport: downloadRoundTripper(func(r *http.Request) (*http.Response, error) {
		time.Sleep(5 * time.Millisecond)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("x")), Request: r}, nil
	})}
	result := st.MeasureDownloadStream(context.Background(), client, DownloadStreamOptions{MaximumBytes: 512, MaximumDuration: time.Second}, nil)
	encoded, _ := json.Marshal(result)
	var record map[string]any
	json.Unmarshal(encoded, &record)
	phases, ok := record["phase_timings_ns"].(map[string]any)
	if !ok || len(phases) == 0 {
		t.Fatalf("missing stage durations: %s", encoded)
	}
	if _, exists := phases["dns"]; exists {
		t.Fatal("must not invent hidden DNS duration")
	}
}

func TestDownloadProxyHandshakeFailureRetainsObservedPhase(t *testing.T) {
	st := newDownloadStreamTester(t)
	client := &http.Client{Transport: downloadRoundTripper(func(r *http.Request) (*http.Response, error) {
		markDownloadPhase(r.Context(), "proxy_handshake")
		return nil, errors.New("failed to create session")
	})}
	got := st.MeasureDownloadStream(context.Background(), client, DownloadStreamOptions{MaximumBytes: 100, MaximumDuration: time.Second}, nil)
	if got.FailurePhase != "proxy_handshake" || got.Phase != "proxy_handshake" || got.ErrorClass != "proxy_session" {
		t.Fatalf("observed handshake overwritten: %+v", got)
	}
}
