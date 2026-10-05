package speedtester

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/metacubex/mihomo/constant"
)

type latencyRuntimeProxy struct {
	constant.Proxy
	dial func(context.Context) (constant.Conn, error)
}

func (p *latencyRuntimeProxy) Type() constant.AdapterType { return constant.Socks5 }
func (p *latencyRuntimeProxy) DialContext(ctx context.Context, _ *constant.Metadata) (constant.Conn, error) {
	return p.dial(ctx)
}

func TestLatencyUsesConfiguredTimeoutWhenMaxLatencyIsUnset(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	proxy := &CProxy{Proxy: &latencyRuntimeProxy{dial: func(context.Context) (constant.Conn, error) {
		<-release
		return nil, errors.New("fixture released blocked dial")
	}}}
	tester := &SpeedTester{config: &Config{Timeout: 25 * time.Millisecond, PingCount: 1, LatencyTargetURL: "http://latency.invalid/probe"}, metrics: MetricSet{Latency: true}}
	done := make(chan *Result, 1)
	go func() { done <- tester.TestSingle("fixture", proxy, nil) }()
	select {
	case result := <-done:
		if len(result.LatencySamples) != 1 || result.LatencySamples[0].Success || !strings.Contains(result.LatencySamples[0].Error, "deadline exceeded") {
			t.Fatalf("timeout must remain an explicit failed sample: %+v", result.LatencySamples)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("configured 25ms timeout did not bound the blocked latency request")
	}
}

func TestSingleContextCancelsCurrentLatencyAndStopsFurtherSamples(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	var requests atomic.Int32
	proxy := &CProxy{Proxy: &latencyRuntimeProxy{dial: func(context.Context) (constant.Conn, error) {
		if requests.Add(1) == 1 {
			close(started)
		}
		<-release
		return nil, errors.New("fixture released blocked dial")
	}}}
	tester := &SpeedTester{config: &Config{Timeout: time.Second, PingCount: 3, LatencyTargetURL: "http://latency.invalid/probe"}, metrics: MetricSet{Latency: true}}
	done := make(chan *Result, 1)
	go func() { done <- tester.TestSingleContext(ctx, "fixture", proxy, nil) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("latency request did not start")
	}
	cancel()
	select {
	case result := <-done:
		if len(result.LatencySamples) != 1 || result.LatencySamples[0].Success || !strings.Contains(result.LatencySamples[0].Error, "context canceled") {
			t.Fatalf("current request must retain its cancellation as a failed sample: %+v", result.LatencySamples)
		}
		if requests.Load() != 1 {
			t.Fatalf("cancellation started further latency requests: %d", requests.Load())
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("running latency request did not respond to cancellation")
	}
}

type latencyCancellationBody struct {
	ctx     context.Context
	started chan struct{}
}

func (b *latencyCancellationBody) Read([]byte) (int, error) {
	close(b.started)
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}
func (*latencyCancellationBody) Close() error { return nil }

func TestLatencyCancellationDuringResponseBodyReadIsNotSuccessful(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	client := &http.Client{Transport: downloadRoundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: &latencyCancellationBody{ctx: r.Context(), started: started}}, nil
	})}
	tester := &SpeedTester{config: &Config{LatencyTargetURL: "http://latency.invalid/probe"}}
	done := make(chan *latencyResult, 1)
	go func() { done <- tester.testLatencyWithClientContext(ctx, client, 3) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("response body was not read")
	}
	cancel()
	select {
	case result := <-done:
		if len(result.samples) != 1 || result.samples[0].Success || result.samples[0].Error != context.Canceled.Error() || result.packetLoss != 100 {
			t.Fatalf("cancelled body read must retain a failure without fabricated future samples: %+v", result)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("response body read did not respond to cancellation")
	}
}

func TestLatencyCancellationBetweenSamplesPreservesCompletedSample(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	firstRequest := make(chan struct{})
	var requests atomic.Int32
	client := &http.Client{Transport: downloadRoundTripper(func(*http.Request) (*http.Response, error) {
		if requests.Add(1) == 1 {
			close(firstRequest)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("x"))}, nil
	})}
	tester := &SpeedTester{config: &Config{LatencyTargetURL: "http://latency.invalid/probe"}}
	done := make(chan *latencyResult, 1)
	go func() { done <- tester.testLatencyWithClientContext(ctx, client, 3) }()
	select {
	case <-firstRequest:
	case <-time.After(time.Second):
		t.Fatal("first latency request did not start")
	}
	// Let the completed byte response enter the existing 100ms interval.
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case result := <-done:
		if len(result.samples) != 1 || !result.samples[0].Success || result.packetLoss != 0 || requests.Load() != 1 {
			t.Fatalf("completed sample changed or future samples were fabricated after cancellation: %+v, requests=%d", result, requests.Load())
		}
	case <-time.After(75 * time.Millisecond):
		t.Fatal("cancellation did not interrupt the 100ms interval")
	}
}
