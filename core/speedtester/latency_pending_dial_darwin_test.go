//go:build darwin

package speedtester

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/metacubex/mihomo/constant"
)

// No socket, DNS or node: the adapter only waits for the context supplied by
// the real net/http Transport. Closing only at the end of a six-round node is
// too late: abandoned connection attempts must not survive into later rounds.
func TestLatencyTimeoutCancelsPendingDialBeforeNextSample(t *testing.T) {
	var mu sync.Mutex
	var contexts []context.Context
	overlapping := false
	p := &physicalFixtureProxy{connect: func(ctx context.Context, _ *constant.Metadata) (constant.Conn, error) {
		mu.Lock()
		for _, old := range contexts {
			if old.Err() == nil {
				overlapping = true
			}
		}
		contexts = append(contexts, ctx)
		mu.Unlock()
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	st := &SpeedTester{config: &Config{LatencyTargetURL: "http://fixture.invalid/probe"}}
	client := st.createClient(p, 25*time.Millisecond)
	client.Transport.(*http.Transport).DisableKeepAlives = true
	defer client.CloseIdleConnections()
	result := st.testLatencyWithClientContext(context.Background(), client, 2)
	mu.Lock()
	defer mu.Unlock()
	if len(contexts) != 2 || len(result.samples) != 2 {
		t.Fatalf("planned samples changed: dial=%d samples=%d", len(contexts), len(result.samples))
	}
	if overlapping {
		t.Error("next sample began while the timed-out proxy dial was still uncancelled")
	}
	for _, ctx := range contexts {
		if ctx.Err() == nil {
			t.Error("HTTP returned timeout but pending proxy connection remains alive")
		}
	}
	for _, s := range result.samples {
		if s.Success {
			t.Error("timed-out attempt must remain a failed sample")
		}
	}
}

func TestLatencyTimeoutCancelsPendingTargetTLS(t *testing.T) {
	closed := make(chan struct{}, 2)
	p := &physicalFixtureProxy{connect: func(ctx context.Context, _ *constant.Metadata) (constant.Conn, error) {
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			defer func() { closed <- struct{}{} }()
			_, _ = io.Copy(io.Discard, server)
		}()
		return &fixtureHTTPConn{pipe: client}, nil
	}}
	st := &SpeedTester{config: &Config{LatencyTargetURL: "https://fixture.invalid/probe"}}
	client := st.createClient(p, 25*time.Millisecond)
	client.Transport.(*http.Transport).DisableKeepAlives = true
	defer client.CloseIdleConnections()
	result := st.testLatencyWithClientContext(context.Background(), client, 2)
	if len(result.samples) != 2 {
		t.Fatalf("samples=%d", len(result.samples))
	}
	for i := 0; i < 2; i++ {
		select {
		case <-closed:
		case <-time.After(300 * time.Millisecond):
			t.Fatal("target TLS connection survived its HTTP timeout")
		}
	}
	if p.closed.Load() != 0 {
		t.Fatal("per-sample cleanup must not close the shared proxy adapter")
	}
}

// An expired probe must release only abandoned dials. Another live probe may
// still be establishing its stream on this same transport and shared adapter.
func TestLatencyTimeoutCleanupPreservesAnotherWaitingProbe(t *testing.T) {
	started := make(chan context.Context, 1)
	release := make(chan struct{})
	defer close(release)
	p := &physicalFixtureProxy{connect: func(ctx context.Context, m *constant.Metadata) (constant.Conn, error) {
		if m.Host != "waiting.invalid" {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		started <- ctx
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
		}
		c, s := net.Pipe()
		go func() {
			defer s.Close()
			_, err := http.ReadRequest(bufio.NewReader(s))
			if err == nil {
				fmt.Fprint(s, "HTTP/1.1 204 No Content\r\nConnection: close\r\n\r\n")
			}
		}()
		return &fixtureHTTPConn{pipe: c}, nil
	}}
	st := &SpeedTester{config: &Config{LatencyTargetURL: "http://timeout.invalid/probe"}}
	client := st.createClient(p, 25*time.Millisecond)
	client.Transport.(*http.Transport).DisableKeepAlives = true
	defer client.CloseIdleConnections()
	other := *client
	other.Timeout = time.Second
	done := make(chan error, 1)
	go func() {
		r, e := other.Get("http://waiting.invalid/probe")
		if r != nil {
			r.Body.Close()
		}
		done <- e
	}()
	var waiting context.Context
	select {
	case waiting = <-started:
	case <-time.After(time.Second):
		t.Fatal("sibling probe did not start")
	}
	result := st.testLatencyWithClientContext(context.Background(), client, 1)
	if len(result.samples) != 1 || result.samples[0].Success {
		t.Fatal("expired probe must stay failed")
	}
	if waiting.Err() != nil {
		t.Fatal("cleanup cancelled another live probe")
	}
	release <- struct{}{}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("sibling failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("sibling did not complete")
	}
	if p.closed.Load() != 0 {
		t.Fatal("cleanup closed shared proxy")
	}
}
