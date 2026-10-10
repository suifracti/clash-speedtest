package speedtester

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLatencySuiteMeasuresEverySiteAndKeepsFailures(t *testing.T) {
	counts := map[string]int{}
	var countsMu sync.Mutex
	firstRound := make(chan struct{})
	started := 0
	client := &http.Client{Transport: downloadRoundTripper(func(r *http.Request) (*http.Response, error) {
		countsMu.Lock()
		counts[r.URL.Host]++
		started++
		if started == 6 {
			close(firstRound)
		}
		countsMu.Unlock()
		select {
		case <-firstRound:
		case <-time.After(2 * time.Second):
			t.Error("six sites did not start concurrently")
			return nil, errors.New("concurrent probes blocked")
		}
		if r.URL.Host == "api.github.com" {
			return nil, errors.New("test connection refused")
		}
		return &http.Response{StatusCode: 204, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	tester := &SpeedTester{config: &Config{LatencyTargetURL: "multi://latency-v1"}}
	result := tester.testLatencyWithClient(client, 2)
	for _, host := range []string{"speed.cloudflare.com", "www.gstatic.com", "api.github.com", "captive.apple.com", "www.msftconnecttest.com", "detectportal.firefox.com"} {
		if counts[host] != 2 {
			t.Fatalf("%s received %d probes; want 2", host, counts[host])
		}
	}
	if len(result.samples) != 12 {
		t.Fatalf("want 12 samples, got %d", len(result.samples))
	}
	failures := 0
	for i, sample := range result.samples {
		if sample.Seq != i+1 || sample.Target == "" {
			t.Fatalf("sample attribution lost: %+v", sample)
		}
		if !sample.Success {
			failures++
			if sample.Target != "https://api.github.com/zen" || !strings.Contains(sample.Error, "test connection refused") {
				t.Fatalf("wrong failure: %+v", sample)
			}
		}
	}
	if failures != 2 {
		t.Fatalf("want 2 failures, got %d", failures)
	}
}

func TestLatencyHTTPRefusalKeepsTimingWithoutClaimingBusinessSuccess(t *testing.T) {
	client := &http.Client{Transport: downloadRoundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("limited")), Request: r}, nil
	})}
	tester := &SpeedTester{config: &Config{LatencyTargetURL: "https://api.github.com/zen"}}
	result := tester.testLatencyWithClient(client, 1)
	if len(result.samples) != 1 || !result.samples[0].Success || !strings.Contains(result.samples[0].Error, "HTTP 429") {
		t.Fatalf("response latency and endpoint refusal not separated: %+v", result.samples)
	}
}

func TestLatencyRedirectIsOneRequestAndKeepsFirstResponse(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: downloadRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		h := make(http.Header)
		h.Set("Location", "https://another.example/next")
		return &http.Response{StatusCode: 302, Header: h, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})}
	tester := &SpeedTester{config: &Config{LatencyTargetURL: "https://probe.example/"}}
	result := tester.testLatencyWithClient(client, 1)
	if calls != 1 || len(result.samples) != 1 || !result.samples[0].Success || !strings.Contains(result.samples[0].Error, "HTTP 302") {
		t.Fatalf("redirect made %d requests, samples=%+v", calls, result.samples)
	}
}
