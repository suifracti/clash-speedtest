package monitor

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestProbeWorkerPanicProducesUnexecutedTargets(t *testing.T) {
	runner := NewRunner(RunnerConfig{Dialer: &mockDialer{responses: map[string]func(*http.Request) (*http.Response, error){"node": func(*http.Request) (*http.Response, error) { panic("fixture") }}}})
	samples, success, err := runner.probeNode(context.Background(), "p", MonitoredNode{NodeKey: "node", Type: "http", Server: "example.test", Port: 443}, []TargetSpec{{URL: "https://example.test", ProbeType: "rtt"}}, time.Second, "run", false, 0)
	if err != nil || success || len(samples) != 1 || samples[0].ErrorClass != "not_executed" {
		t.Fatalf("samples=%+v success=%v err=%v", samples, success, err)
	}
}
