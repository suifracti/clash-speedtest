package monitor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type invalidSixDialer struct{}

func (invalidSixDialer) CreateClient(MonitoredNode, time.Duration) (*http.Client, error) {
	return nil, fmt.Errorf("invalid frozen proxy configuration")
}

func TestSixLatencyScopeDoesNotChangeLegacyTargets(t *testing.T) {
	targets := GetProbeTargets(ProbeSetType("latency_six_v1"))
	if len(targets) != 6 {
		t.Fatalf("six-site scope has %d targets", len(targets))
	}
	if len(GetProbeTargets(ProbeSetLight)) != 1 || len(GetProbeTargets(ProbeSetService)) != 3 {
		t.Fatal("legacy scope changed")
	}
	seen := map[string]bool{}
	for _, target := range targets {
		if target.ProbeType != "rtt" || seen[target.URL] {
			t.Fatal("not six independent latency targets")
		}
		seen[target.URL] = true
	}
	if !seen["https://www.gstatic.com/generate_204"] || !seen["https://detectportal.firefox.com/success.txt"] {
		t.Fatal("missing default targets")
	}
}
func TestSixLatencyConfigurationFailurePreservesUnexecutedTargets(t *testing.T) {
	r := NewRunner(RunnerConfig{Dialer: invalidSixDialer{}})
	_, samples, err := r.ExecuteRun(context.Background(), &MonitorJob{ID: "six", ProfileID: "p", ProbeSet: ProbeSetType("latency_six_v1"), Nodes: []MonitoredNode{{NodeKey: "n", NodeIdentityKey: "i", ConfigRevisionKey: "r"}}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 6 {
		t.Fatalf("want six planned unexecuted targets, got %d", len(samples))
	}
	for _, s := range samples {
		if s.Success || s.ErrorClass != "not_executed" || s.Metadata["execution_status"] != "not_executed" {
			t.Fatalf("unexecuted configuration misclassified: %+v", s)
		}
	}
}

func TestSixLatencyHTTPRejectionStillRecordsMeasuredDelay(t *testing.T) {
	d := &mockDialer{responses: map[string]func(*http.Request) (*http.Response, error){"n": func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("limited"))}, nil
	}}}
	r := NewRunner(RunnerConfig{Dialer: d})
	_, ss, err := r.ExecuteRun(context.Background(), &MonitorJob{ID: "six-http", ProfileID: "p", ProbeSet: ProbeSetLatencySix, Timeout: time.Second, Nodes: []MonitoredNode{{NodeKey: "n"}}}, time.Now())
	if err != nil || len(ss) != 6 {
		t.Fatalf("planned six: %v %d", err, len(ss))
	}
	for _, s := range ss {
		if !s.Success || s.Latency <= 0 || s.Metadata["http_status"] != 429 || !strings.Contains(s.ErrorDetail, "429") {
			t.Fatalf("HTTP response lost as unavailable: %+v", s)
		}
	}
}
