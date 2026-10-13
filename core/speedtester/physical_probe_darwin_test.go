//go:build darwin

package speedtester

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/metacubex/mihomo/constant"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type physicalFixtureProxy struct {
	constant.Proxy
	calls, closed atomic.Int32
	dial          func(context.Context) error
	connect       func(context.Context, *constant.Metadata) (constant.Conn, error)
}

func (p *physicalFixtureProxy) Type() constant.AdapterType { return constant.Socks5 }
func (p *physicalFixtureProxy) Close() error               { p.closed.Add(1); return nil }
func (p *physicalFixtureProxy) DialContext(ctx context.Context, metadata *constant.Metadata) (constant.Conn, error) {
	p.calls.Add(1)
	if p.connect != nil {
		return p.connect(ctx, metadata)
	}
	return nil, p.dial(ctx)
}

type fixtureHTTPConn struct {
	constant.Conn
	pipe net.Conn
}

func (c *fixtureHTTPConn) Read(b []byte) (int, error)         { return c.pipe.Read(b) }
func (c *fixtureHTTPConn) Write(b []byte) (int, error)        { return c.pipe.Write(b) }
func (c *fixtureHTTPConn) Close() error                       { return c.pipe.Close() }
func (c *fixtureHTTPConn) LocalAddr() net.Addr                { return c.pipe.LocalAddr() }
func (c *fixtureHTTPConn) RemoteAddr() net.Addr               { return c.pipe.RemoteAddr() }
func (c *fixtureHTTPConn) SetDeadline(t time.Time) error      { return c.pipe.SetDeadline(t) }
func (c *fixtureHTTPConn) SetReadDeadline(t time.Time) error  { return c.pipe.SetReadDeadline(t) }
func (c *fixtureHTTPConn) SetWriteDeadline(t time.Time) error { return c.pipe.SetWriteDeadline(t) }

func TestPreparedPhysicalClientMakesOneHTTP204OnMemoryProxyAndClosesOnce(t *testing.T) {
	original := &physicalFixtureProxy{dial: func(context.Context) error { return errors.New("default route forbidden") }}
	done := make(chan struct{})
	bound := &physicalFixtureProxy{}
	e := newPhysicalDownloadEgress()
	bound.connect = func(ctx context.Context, metadata *constant.Metadata) (constant.Conn, error) {
		// Synthetic acknowledgement of the binding callback, not host route proof.
		e.mu.Lock()
		e.tcp++
		e.mu.Unlock()
		if ctx.Err() != nil || metadata.Host != "fixture.invalid" {
			t.Errorf("target/context changed: %v %s", ctx.Err(), metadata.Host)
		}
		client, server := net.Pipe()
		go func() {
			defer close(done)
			defer server.Close()
			req, err := http.ReadRequest(bufio.NewReader(server))
			if err != nil {
				t.Error(err)
				return
			}
			if req.Method != http.MethodGet || req.URL.Path != "/204" {
				t.Errorf("unexpected request: %s %s", req.Method, req.URL)
			}
			fmt.Fprint(server, "HTTP/1.1 204 No Content\r\nConnection: close\r\n\r\n")
		}()
		return &fixtureHTTPConn{pipe: client}, nil
	}
	client, path, closeClient, err := openPhysicalProbeClient(context.Background(), &CProxy{Proxy: original}, time.Second, 1, func(context.Context, *CProxy, int) (*CProxy, *PhysicalDownloadEgress, error) {
		return &CProxy{Proxy: bound}, e, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get("http://fixture.invalid/204")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	closeClient()
	closeClient()
	<-done
	if resp.StatusCode != 204 || bound.calls.Load() != 1 || original.calls.Load() != 0 || bound.closed.Load() != 1 || !path().SocketBindVerified {
		t.Fatal("request/fallback/cleanup or synthetic binding evidence incorrect")
	}
}

func TestPhysicalClientRejectsAdapterThatNeverUsedItsBoundDialerBeforeHTTPWrite(t *testing.T) {
	done := make(chan struct{})
	var requests atomic.Int32
	p := &physicalFixtureProxy{connect: func(context.Context, *constant.Metadata) (constant.Conn, error) {
		c, s := net.Pipe()
		go func() {
			defer close(done)
			defer s.Close()
			_, err := http.ReadRequest(bufio.NewReader(s))
			if err == nil {
				requests.Add(1)
				fmt.Fprint(s, "HTTP/1.1 204 No Content\r\nConnection: close\r\n\r\n")
			}
		}()
		return &fixtureHTTPConn{pipe: c}, nil
	}}
	client, _, cleanup, err := openPhysicalProbeClient(context.Background(), &CProxy{Proxy: p}, time.Second, 1, func(context.Context, *CProxy, int) (*CProxy, *PhysicalDownloadEgress, error) {
		return &CProxy{Proxy: p}, newPhysicalDownloadEgress(), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	_, err = client.Get("http://fixture.invalid/204")
	<-done
	var local *PhysicalPathError
	if !errors.As(err, &local) || requests.Load() != 0 {
		t.Fatalf("unbound adapter wrote HTTP or silently succeeded: err=%v requests=%d", err, requests.Load())
	}
}

func TestPhysicalLatencySixSitesUsePreparedProxyAndCloseWithoutFallback(t *testing.T) {
	original := &physicalFixtureProxy{dial: func(context.Context) error { return errors.New("must not use default route") }}
	bound := &physicalFixtureProxy{dial: func(context.Context) error { return errors.New("fixture node TCP failed") }}
	e := newPhysicalDownloadEgress()
	st := &SpeedTester{config: &Config{PingCount: 1, Timeout: time.Second, LatencyTargetURL: "multi://latency-v1"}}
	got := st.testSinglePhysicalContext(context.Background(), "selected", &CProxy{Proxy: original, Config: map[string]any{"server": "node.example", "sni": "tls.example"}}, nil, func(ctx context.Context, p *CProxy, limit int) (*CProxy, *PhysicalDownloadEgress, error) {
		if limit != 6 || p.Config["sni"] != "tls.example" || ctx.Err() != nil {
			t.Fatalf("frozen plan/preparation changed: limit=%d", limit)
		}
		return &CProxy{Proxy: bound, Config: p.Config}, e, nil
	})
	if len(got.LatencySamples) != 6 || bound.calls.Load() != 6 || original.calls.Load() != 0 || bound.closed.Load() != 1 || got.NetworkPath == nil {
		t.Fatalf("six-site coverage, cleanup, path or fallback mismatch: samples=%d bound=%d original=%d closes=%d", len(got.LatencySamples), bound.calls.Load(), original.calls.Load(), bound.closed.Load())
	}
	for _, s := range got.LatencySamples {
		if s.Success || s.Error == "" {
			t.Fatal("real fixture TCP failures lost")
		}
	}
	if got.PacketLoss != 100 || got.LatencyErrorClass != "" {
		t.Fatal("TCP failure must remain a measured failure, not a preparation skip")
	}
}

func TestPhysicalLatencyPreparationFailureAndCancellationSendZeroTargetRequests(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "preparation_failure", true: "cancelled_before_prepare"}[cancelled], func(t *testing.T) {
			p := &physicalFixtureProxy{dial: func(context.Context) error { return errors.New("must not dial") }}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if cancelled {
				cancel()
			}
			prepared := 0
			st := &SpeedTester{config: &Config{PingCount: 1, Timeout: time.Second, LatencyTargetURL: "multi://latency-v1"}}
			got := st.testSinglePhysicalContext(ctx, "selected", &CProxy{Proxy: p}, nil, func(context.Context, *CProxy, int) (*CProxy, *PhysicalDownloadEgress, error) {
				prepared++
				e := newPhysicalDownloadEgress()
				e.setFailure("physical_dns_unavailable")
				return nil, e, errors.New("fixture unavailable")
			})
			if p.calls.Load() != 0 || len(got.LatencySamples) != 0 || got.PacketLoss != 0 || got.LatencyErrorClass != "physical_path" || (cancelled && prepared != 0) {
				t.Fatalf("preparation issued target traffic or blamed node: %+v", got)
			}
		})
	}
}

func TestPhysicalProbeConnectionBudgetFollowsSixSitesWithoutChangingDownloadLimit(t *testing.T) {
	if AutoDetectPhysicalInterface() == "" {
		t.Skip("no physical local interface")
	}
	config := &CProxy{Config: map[string]any{"name": "fixture", "type": "socks5", "server": "203.0.113.9", "port": 443}}
	p, e, err := PreparePhysicalProbeProxy(context.Background(), config, 6)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	for i := 0; i < 6; i++ {
		if err := e.reserveDialBudget(context.Background(), false); err != nil {
			t.Fatalf("sixth site prematurely blocked: %v", err)
		}
	}
	if err := e.reserveDialBudget(context.Background(), false); !errors.Is(err, errPhysicalDownloadBudgetExceeded) {
		t.Fatal("seventh connection not bounded")
	}
	if e.Snapshot().TCPDialAttempts != 6 || e.Snapshot().SocketBindVerified {
		t.Fatal("reservation invented socket binding")
	}
	download := newPhysicalDownloadEgress()
	for i := 0; i < 4; i++ {
		if err := download.reserveDialBudget(context.Background(), false); err != nil {
			t.Fatal(err)
		}
	}
	if err := download.reserveDialBudget(context.Background(), false); !errors.Is(err, errPhysicalDownloadBudgetExceeded) {
		t.Fatal("existing download limit changed")
	}
}

func TestPhysicalLatencyBindingFailureIsUnexecutedSample(t *testing.T) {
	p := &physicalFixtureProxy{dial: func(context.Context) error {
		return &PhysicalPathError{Cause: errors.New("fixture binding unavailable")}
	}}
	st := &SpeedTester{config: &Config{PingCount: 1, Timeout: time.Second, LatencyTargetURL: "https://latency.invalid"}}
	got := st.testSinglePhysicalContext(context.Background(), "selected", &CProxy{Proxy: p}, nil, func(context.Context, *CProxy, int) (*CProxy, *PhysicalDownloadEgress, error) {
		return &CProxy{Proxy: p}, newPhysicalDownloadEgress(), nil
	})
	if len(got.LatencySamples) != 1 || !strings.Contains(got.LatencySamples[0].Error, "未执行") {
		t.Fatalf("binding failure incorrectly blamed on node: %+v", got.LatencySamples)
	}
}

func TestPhysicalProbeExhaustedTCPAndUDPBudgetIsALocalConstraint(t *testing.T) {
	e := newPhysicalDownloadEgress()
	e.tcpDials = newPhysicalRequestBudget(0)
	e.udpDials = newPhysicalRequestBudget(0)
	_, tcpErr := e.DialContext(context.Background(), "tcp", "203.0.113.9:443")
	_, udpErr := e.ListenPacket(context.Background(), "udp", ":0", netip.MustParseAddrPort("203.0.113.9:443"))
	for _, err := range []error{tcpErr, udpErr} {
		var path *PhysicalPathError
		if !errors.As(err, &path) || !errors.Is(err, errPhysicalDownloadBudgetExceeded) {
			t.Fatalf("local budget exhaustion disguised as node failure: %v", err)
		}
	}
	if e.Snapshot().SocketBindVerified {
		t.Fatal("exhausted budget fabricated a bound socket")
	}
}
