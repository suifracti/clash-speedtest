//go:build darwin

package speedtester

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http/httptrace"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestNodeTCPConnectionEvidenceAndExistingHTTPTrace(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var observedStart, observedDone atomic.Int32
	ctx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{ConnectStart: func(string, string) { observedStart.Add(1) }, ConnectDone: func(string, string, error) { observedDone.Add(1) }})
	e := newPhysicalDownloadEgress()
	conn, err := e.dialNodeTCP(ctx, "tcp4", listener.Addr().String(), (&net.Dialer{}).DialContext)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	path := e.Snapshot()
	if path.TCPConnectStarted != 1 || path.TCPConnectSucceeded != 1 || path.TCPConnectFailed != 0 || path.TCPConnectDurationNS <= 0 || path.LastTCPConnectOutcome != "connected" || path.LastTCPConnectStartedAt == "" || path.LastTCPConnectFinishedAt == "" {
		t.Fatalf("TCP stage was not observed: %+v", path)
	}
	if observedStart.Load() != 1 || observedDone.Load() != 1 {
		t.Fatal("outer HTTP trace was replaced or duplicated")
	}
	if path.SocketBindVerified || path.TUNEvidence != "packet_route_not_observed" {
		t.Fatal("connection timing falsely proves physical routing")
	}
	encoded, _ := json.Marshal(path)
	if strings.Contains(string(encoded), listener.Addr().String()) || strings.Contains(string(encoded), "127.0.0.1") {
		t.Fatal("connection evidence leaked a socket address")
	}
}

func TestNodeTCPRefusedKeepsActualErrnoWithoutAddress(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	e := newPhysicalDownloadEgress()
	conn, err := e.dialNodeTCP(context.Background(), "tcp4", address, (&net.Dialer{}).DialContext)
	if conn != nil || err == nil {
		t.Fatal("closed port was not refused")
	}
	path := e.Snapshot()
	if path.TCPConnectStarted != 1 || path.TCPConnectFailed != 1 || path.LastTCPConnectErrno != int(syscall.ECONNREFUSED) || path.LastTCPConnectOutcome != "refused" {
		t.Fatalf("real errno lost: %+v", path)
	}
	encoded, _ := json.Marshal(path)
	if strings.Contains(string(encoded), address) {
		t.Fatal("raw dial error leaked address")
	}
}

func TestNodeTCPContextDeadlineDoesNotInventKernelTimeout(t *testing.T) {
	e := newPhysicalDownloadEgress()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Millisecond)
	defer cancel()
	calls := 0
	_, err := e.dialNodeTCP(ctx, "tcp4", "fixture.invalid:443", func(ctx context.Context, _, _ string) (net.Conn, error) { calls++; <-ctx.Done(); return nil, ctx.Err() })
	if !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
		t.Fatal("context did not reach dial")
	}
	path := e.Snapshot()
	if path.TCPConnectStarted != 0 || path.TCPConnectFailed != 1 || path.LastTCPConnectErrno != 0 || path.LastTCPConnectOutcome != "deadline_exceeded" {
		t.Fatalf("deadline became a made-up kernel errno: %+v", path)
	}
}

func TestNodeTCPCancelledBeforeDialAddsNoEvidenceOrRequest(t *testing.T) {
	e := newPhysicalDownloadEgress()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := e.dialNodeTCP(ctx, "tcp4", "fixture.invalid:443", func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("cancelled dial entered transport")
		return nil, nil
	})
	p := e.Snapshot()
	if !errors.Is(err, context.Canceled) || p.TCPConnectStarted != 0 || p.TCPConnectFailed != 0 || p.LastTCPConnectStartedAt != "" {
		t.Fatal("pre-cancelled task was counted as a connection failure")
	}
}
