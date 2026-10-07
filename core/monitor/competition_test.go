package monitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMonitorCompetitionPreservesRealProbe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer server.Close()
	r := NewRunner(RunnerConfig{ProbeActivity: func(string) func() []string { return func() []string { return []string{"download"} } }})
	sample, err := r.executeSingleProbe(context.Background(), server.Client(), "p", MonitoredNode{}, TargetSpec{ProbeType: "rtt", URL: server.URL}, time.Second, "run")
	if err != nil || !sample.Success || sample.Latency <= 0 || sample.Metadata["quality_caution"] != "shared_resources_overlap_possible" {
		t.Fatalf("competition mark changed/lost probe: %+v %v", sample, err)
	}
}
