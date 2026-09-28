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
