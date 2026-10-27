//go:build darwin

package speedtester

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	C "github.com/metacubex/mihomo/constant"
)

type familyConnectTimeout struct{}

func (familyConnectTimeout) Error() string   { return "fixture handshake timeout" }
func (familyConnectTimeout) Timeout() bool   { return true }
func (familyConnectTimeout) Temporary() bool { return false }

func familyPath(family string) *PhysicalDownloadEgress {
	e := newPhysicalDownloadEgress()
	e.iface = "en1"
	e.index = 15
	e.addressFamily = family
	e.resolutionSource = "physical_" + family + "_dns"
	e.dnsMode = "physical_interface_dns_v1"
	e.dnsVerified = true
	return e
}
func familyMemoryStream() (C.Conn, net.Conn) { c, s := net.Pipe(); return &fixtureHTTPConn{pipe: c}, s }

func TestPhysicalFamilyFallbackWritesOneHTTPAndPinsIPv6(t *testing.T) {
	p4, p6 := familyPath("ipv4"), familyPath("ipv6")
	ipv4 := &physicalFixtureProxy{connect: func(_ context.Context, metadata *C.Metadata) (C.Conn, error) {
		metadata.Host = "mutated.invalid"
		return nil, familyConnectTimeout{}
	}}
	var requests atomic.Int32
	done := make(chan struct{})
	ipv6 := &physicalFixtureProxy{connect: func(ctx context.Context, m *C.Metadata) (C.Conn, error) {
		if m.Host != "fixture.invalid" {
			t.Errorf("target changed: %s", m.Host)
		}
		p6.mu.Lock()
		p6.udp++
		p6.mu.Unlock()
		conn, server := familyMemoryStream()
		go func() {
			defer close(done)
			defer server.Close()
			req, err := http.ReadRequest(bufio.NewReader(server))
			if err != nil {
				return
			}
			requests.Add(1)
			if req.Method != "GET" || req.URL.Path != "/204" {
				t.Error("request changed")
			}
			fmt.Fprint(server, "HTTP/1.1 204 No Content\r\nConnection: close\r\n\r\n")
		}()
		return conn, nil
	}}
	primary := &CProxy{Proxy: ipv4, Config: map[string]any{"type": "hysteria2", "server": "fixture.invalid", "password": "fixture-only", "sni": "tls.invalid"}}
	alternate := &CProxy{Proxy: ipv6, Config: primary.Config}
	wrapped := wrapPhysicalFamilies(primary, p4, alternate, p6, time.Second)
	client, path, cleanup, err := openPhysicalProbeClient(context.Background(), primary, time.Second, 2, func(context.Context, *CProxy, int) (*CProxy, *PhysicalDownloadEgress, error) { return wrapped, p4, nil })
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	response, err := client.Get("http://fixture.invalid/204")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	<-done
	cleanup()
	cleanup()
	evidence := path()
	if response.StatusCode != 204 || requests.Load() != 1 || ipv4.calls.Load() != 1 || ipv6.calls.Load() != 1 || ipv4.closed.Load() != 1 || ipv6.closed.Load() != 1 || evidence.AddressFamily != "ipv6" || evidence.Method == "physical_socket_v1" || !evidence.SocketBindVerified {
		t.Fatalf("fallback replayed HTTP, lost selection or leaked proxy: requests=%d p4=%d p6=%d close4=%d close6=%d path=%+v", requests.Load(), ipv4.calls.Load(), ipv6.calls.Load(), ipv4.closed.Load(), ipv6.closed.Load(), evidence)
	}
	if primary.Config["server"] != "fixture.invalid" || primary.Config["password"] != "fixture-only" || primary.Config["sni"] != "tls.invalid" {
		t.Fatal("original scope mutated")
	}
}

func TestPhysicalFamilySelectionDoesNotSwitchAfterSuccessfulStream(t *testing.T) {
	calls := 0
	p4 := familyPath("ipv4")
	first := &physicalFixtureProxy{connect: func(context.Context, *C.Metadata) (C.Conn, error) {
		calls++
		if calls > 1 {
			return nil, familyConnectTimeout{}
		}
		p4.mu.Lock()
		p4.udp++
		p4.mu.Unlock()
		c, s := familyMemoryStream()
		s.Close()
		return c, nil
	}}
	second := &physicalFixtureProxy{dial: func(context.Context) error { return errors.New("must not dial alternate") }}
	wrapped := wrapPhysicalFamilies(&CProxy{Proxy: first}, p4, &CProxy{Proxy: second}, familyPath("ipv6"), time.Second)
	defer wrapped.Close()
	conn, err := wrapped.DialContext(context.Background(), &C.Metadata{Host: "fixture.invalid", DstPort: 443})
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	_, err = wrapped.DialContext(context.Background(), &C.Metadata{Host: "fixture.invalid", DstPort: 443})
	if err == nil || second.calls.Load() != 0 {
		t.Fatal("successful round silently changed family")
	}
}

func TestPhysicalFamilyDoesNotRetryCredentialsBindingOrCancellation(t *testing.T) {
	for _, cause := range []error{errors.New("fixture authentication rejected"), &PhysicalPathError{Cause: errors.New("binding rejected")}, context.Canceled} {
		t.Run(cause.Error(), func(t *testing.T) {
			first := &physicalFixtureProxy{dial: func(context.Context) error { return cause }}
			second := &physicalFixtureProxy{dial: func(context.Context) error { return errors.New("must not run") }}
			wrapped := wrapPhysicalFamilies(&CProxy{Proxy: first}, familyPath("ipv4"), &CProxy{Proxy: second}, familyPath("ipv6"), time.Second)
			_, err := wrapped.DialContext(context.Background(), &C.Metadata{Host: "fixture.invalid"})
			wrapped.Close()
			if !errors.Is(err, cause) || second.calls.Load() != 0 || first.closed.Load() != 1 || second.closed.Load() != 1 {
				t.Fatalf("non-connect error retried or cleanup leaked: %v", err)
			}
		})
	}
}

func TestPhysicalFamilySharesOneDeadlineAndCloseCancelsDial(t *testing.T) {
	t.Run("deadline", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
		defer cancel()
		absolute, _ := ctx.Deadline()
		var firstDeadline, secondDeadline time.Time
		first := &physicalFixtureProxy{dial: func(c context.Context) error { firstDeadline, _ = c.Deadline(); <-c.Done(); return c.Err() }}
		second := &physicalFixtureProxy{dial: func(c context.Context) error { secondDeadline, _ = c.Deadline(); return familyConnectTimeout{} }}
		wrapped := wrapPhysicalFamilies(&CProxy{Proxy: first}, familyPath("ipv4"), &CProxy{Proxy: second}, familyPath("ipv6"), time.Second)
		defer wrapped.Close()
		_, err := wrapped.DialContext(ctx, &C.Metadata{Host: "fixture.invalid"})
		if err == nil || second.calls.Load() != 1 || !firstDeadline.Before(absolute) || !secondDeadline.Equal(absolute) {
			t.Fatalf("fresh deadline or missing fallback: first=%v second=%v absolute=%v", firstDeadline, secondDeadline, absolute)
		}
	})
	t.Run("close", func(t *testing.T) {
		entered := make(chan struct{})
		done := make(chan error, 1)
		first := &physicalFixtureProxy{dial: func(c context.Context) error { close(entered); <-c.Done(); return c.Err() }}
		second := &physicalFixtureProxy{dial: func(context.Context) error { return errors.New("must not run") }}
		wrapped := wrapPhysicalFamilies(&CProxy{Proxy: first}, familyPath("ipv4"), &CProxy{Proxy: second}, familyPath("ipv6"), time.Second)
		go func() {
			_, err := wrapped.DialContext(context.Background(), &C.Metadata{Host: "fixture.invalid"})
			done <- err
		}()
		<-entered
		wrapped.Close()
		wrapped.Close()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) || second.calls.Load() != 0 || first.closed.Load() != 1 || second.closed.Load() != 1 {
				t.Fatalf("close leaked or retried: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("close failed to cancel connection")
		}
	})
}

func TestPhysicalAutoPreparationRespectsScopeAndFamily(t *testing.T) {
	for _, version := range []string{"", "dual", "ipv4", "ipv6"} {
		t.Run("version="+version, func(t *testing.T) {
			original := &CProxy{Config: map[string]any{"type": "hysteria2", "server": "node.invalid", "password": "fixture-only", "sni": "tls.invalid", "ip-version": version}}
			var calls int
			p4, p6 := &physicalFixtureProxy{}, &physicalFixtureProxy{}
			prepared, path, err := prepareAutoPhysicalProxyUsing(context.Background(), original, time.Second, 4, func(ctx context.Context, node *CProxy, n int) (*CProxy, *PhysicalDownloadEgress, error) {
				calls++
				if ctx.Err() != nil || n != 4 || node.Config["server"] != "node.invalid" || node.Config["password"] != "fixture-only" || node.Config["sni"] != "tls.invalid" {
					t.Fatal("scope/deadline/budget changed")
				}
				if calls == 1 {
					return &CProxy{Proxy: p4, Config: node.Config}, familyPath("ipv4"), nil
				}
				if node.Config["ip-version"] != "ipv6" {
					t.Fatal("alternate not IPv6")
				}
				return &CProxy{Proxy: p6, Config: node.Config}, familyPath("ipv6"), nil
			}, func(*PhysicalDownloadEgress) bool { return true })
			if err != nil {
				t.Fatal(err)
			}
			prepared.Close()
			want := 1
			if version == "" || version == "dual" {
				want = 2
			}
			if calls != want || original.Config["ip-version"] != version || p4.closed.Load() != 1 || (want == 2 && p6.closed.Load() != 1) {
				t.Fatalf("scope mutated or explicit family overridden: calls=%d path=%+v", calls, path.Snapshot())
			}
		})
	}
	for _, config := range []map[string]any{
		{"type": "hysteria2", "server": "node.invalid"},
		{"type": "vless", "server": "node.invalid"},
		{"type": "vmess", "server": "node.invalid"},
		{"type": "trojan", "server": "node.invalid"},
		{"type": "ss", "server": "node.invalid"},
		{"type": "tuic", "server": "node.invalid"},
		{"type": "hysteria", "server": "node.invalid"},
	} {
		if !autoFamilyConfig(config) {
			t.Fatalf("supported protocol rejected: %v", config)
		}
	}
	for _, config := range []map[string]any{
		{"type": "hysteria2", "server": "203.0.113.9"},
		{"type": "hysteria2", "server": "2001:db8::9"},
		{"type": "wireguard", "server": "node.invalid"},
		{"type": "direct", "server": "node.invalid"},
	} {
		if autoFamilyConfig(config) {
			t.Fatal("literal/protocol outside current fallback scope")
		}
	}
}

func TestPhysicalAutoPreparationKeepsUsableIPv4WhenAAAAUnavailable(t *testing.T) {
	primary := &physicalFixtureProxy{}
	calls := 0
	p, path, err := prepareAutoPhysicalProxyUsing(context.Background(), &CProxy{Config: map[string]any{"type": "hysteria2", "server": "node.invalid"}}, time.Second, 4, func(context.Context, *CProxy, int) (*CProxy, *PhysicalDownloadEgress, error) {
		calls++
		e := familyPath("ipv4")
		if calls == 2 {
			e.addressFamily = "unknown"
			e.failureReason = "aaaa_nodata"
			return nil, e, errors.New("fixture AAAA NODATA")
		}
		return &CProxy{Proxy: primary}, e, nil
	}, func(*PhysicalDownloadEgress) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	evidence := path.Snapshot()
	if calls != 2 || evidence.AddressFamily != "ipv4" || evidence.AddressFallback != "ipv6_unavailable" || evidence.AddressFallbackFailure != "aaaa_nodata" {
		t.Fatalf("usable IPv4 discarded or missing alternate evidence: %+v", evidence)
	}
}

func TestPhysicalAutoPreparationRejectsInterfaceSwitch(t *testing.T) {
	first, second := &physicalFixtureProxy{}, &physicalFixtureProxy{}
	calls := 0
	_, _, err := prepareAutoPhysicalProxyUsing(context.Background(), &CProxy{Config: map[string]any{"type": "hysteria2", "server": "node.invalid"}}, time.Second, 4, func(context.Context, *CProxy, int) (*CProxy, *PhysicalDownloadEgress, error) {
		calls++
		e := familyPath("ipv4")
		p := first
		if calls == 2 {
			e.addressFamily = "ipv6"
			e.iface = "en2"
			e.index = 16
			p = second
		}
		return &CProxy{Proxy: p}, e, nil
	}, func(*PhysicalDownloadEgress) bool { return true })
	var local *PhysicalPathError
	if !errors.As(err, &local) || first.closed.Load() != 1 || second.closed.Load() != 1 || first.calls.Load() != 0 || second.calls.Load() != 0 {
		t.Fatalf("interface drift accepted: %v", err)
	}
}

func TestPhysicalFamilyRefusesUnboundAlternateBeforeTargetHTTP(t *testing.T) {
	first := &physicalFixtureProxy{dial: func(context.Context) error { return familyConnectTimeout{} }}
	done := make(chan struct{})
	var requests atomic.Int32
	second := &physicalFixtureProxy{connect: func(context.Context, *C.Metadata) (C.Conn, error) {
		conn, server := familyMemoryStream()
		go func() {
			defer close(done)
			defer server.Close()
			if _, err := http.ReadRequest(bufio.NewReader(server)); err == nil {
				requests.Add(1)
			}
		}()
		return conn, nil
	}}
	path := familyPath("ipv4")
	wrapped := wrapPhysicalFamilies(&CProxy{Proxy: first}, path, &CProxy{Proxy: second}, familyPath("ipv6"), time.Second)
	client, _, cleanup, err := openPhysicalProbeClient(context.Background(), wrapped, time.Second, 2, func(context.Context, *CProxy, int) (*CProxy, *PhysicalDownloadEgress, error) {
		return wrapped, path, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	_, err = client.Get("http://fixture.invalid/204")
	<-done
	var local *PhysicalPathError
	if !errors.As(err, &local) || requests.Load() != 0 {
		t.Fatalf("unbound fallback reached target HTTP: err=%v requests=%d", err, requests.Load())
	}
}

func TestPhysicalFamilyKeepsSharedSocketBudgetAndCountsItOnce(t *testing.T) {
	p4, p6 := familyPath("ipv4"), familyPath("ipv6")
	p4.udpDials = newPhysicalRequestBudget(2)
	p6.udpDials = p4.udpDials
	first := &physicalFixtureProxy{dial: func(ctx context.Context) error {
		if err := p4.reserveDialBudget(ctx, true); err != nil {
			return err
		}
		return familyConnectTimeout{}
	}}
	second := &physicalFixtureProxy{connect: func(ctx context.Context, _ *C.Metadata) (C.Conn, error) {
		if err := p6.reserveDialBudget(ctx, true); err != nil {
			return nil, err
		}
		p6.mu.Lock()
		p6.udp++
		p6.mu.Unlock()
		conn, server := familyMemoryStream()
		server.Close()
		return conn, nil
	}}
	wrapped := wrapPhysicalFamilies(&CProxy{Proxy: first}, p4, &CProxy{Proxy: second}, p6, time.Second)
	defer wrapped.Close()
	conn, err := wrapped.DialContext(context.Background(), &C.Metadata{Host: "fixture.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	evidence := p4.Snapshot()
	if evidence.UDPDialAttempts != 2 || len(evidence.FamilyAttempts) != 2 || evidence.FamilyAttempts[0].Outcome != "timeout" || evidence.FamilyAttempts[1].Outcome != "connected" || evidence.AddressFallback != "used_ipv6" {
		t.Fatalf("lost or double-counted path evidence: %+v", evidence)
	}
	if err := p6.reserveDialBudget(context.Background(), true); !errors.Is(err, errPhysicalDownloadBudgetExceeded) {
		t.Fatalf("fallback expanded original socket budget: %v", err)
	}
}

func TestPhysicalFamilyFallbackConnectionReset(t *testing.T) {
	p4, p6 := familyPath("ipv4"), familyPath("ipv6")
	ipv4 := &physicalFixtureProxy{connect: func(context.Context, *C.Metadata) (C.Conn, error) {
		return nil, fmt.Errorf("node TLS policy v1: %w", syscall.ECONNRESET)
	}}
	ipv6 := &physicalFixtureProxy{connect: func(context.Context, *C.Metadata) (C.Conn, error) {
		p6.mu.Lock()
		p6.tcp++
		p6.mu.Unlock()
		conn, server := familyMemoryStream()
		server.Close()
		return conn, nil
	}}
	wrapped := wrapPhysicalFamilies(&CProxy{Proxy: ipv4, Config: map[string]any{"type": "vless", "server": "fixture.invalid"}}, p4, &CProxy{Proxy: ipv6}, p6, time.Second)
	defer wrapped.Close()
	conn, err := wrapped.DialContext(context.Background(), &C.Metadata{Host: "fixture.invalid"})
	if err != nil {
		t.Fatalf("expected successful fallback on connection reset: %v", err)
	}
	conn.Close()
	evidence := p4.Snapshot()
	if evidence.AddressFallback != "used_ipv6" || len(evidence.FamilyAttempts) != 2 || evidence.FamilyAttempts[0].Outcome != "connection_failed" || evidence.FamilyAttempts[1].Outcome != "connected" {
		t.Fatalf("unexpected evidence: %+v", evidence)
	}
}

func TestPhysicalFamilyFallbackVLESSConnectionRefused(t *testing.T) {
	p4, p6 := familyPath("ipv4"), familyPath("ipv6")
	ipv4 := &physicalFixtureProxy{connect: func(context.Context, *C.Metadata) (C.Conn, error) {
		return nil, fmt.Errorf("dial tcp4: %w", syscall.ECONNREFUSED)
	}}
	ipv6 := &physicalFixtureProxy{connect: func(context.Context, *C.Metadata) (C.Conn, error) {
		p6.mu.Lock()
		p6.tcp++
		p6.mu.Unlock()
		conn, server := familyMemoryStream()
		server.Close()
		return conn, nil
	}}
	wrapped := wrapPhysicalFamilies(&CProxy{Proxy: ipv4, Config: map[string]any{"type": "vless", "server": "fixture.invalid"}}, p4, &CProxy{Proxy: ipv6}, p6, time.Second)
	defer wrapped.Close()
	conn, err := wrapped.DialContext(context.Background(), &C.Metadata{Host: "fixture.invalid"})
	if err != nil {
		t.Fatalf("expected successful fallback on connection refused: %v", err)
	}
	conn.Close()
	evidence := p4.Snapshot()
	if evidence.AddressFallback != "used_ipv6" || len(evidence.FamilyAttempts) != 2 || evidence.FamilyAttempts[0].Outcome != "connection_failed" || evidence.FamilyAttempts[1].Outcome != "connected" {
		t.Fatalf("unexpected evidence: %+v", evidence)
	}
}

func TestPhysicalFamilyBothFailNextSampleStartsWithIPv4(t *testing.T) {
	var order []string
	ipv4 := &physicalFixtureProxy{dial: func(context.Context) error { order = append(order, "ipv4"); return io.EOF }}
	ipv6 := &physicalFixtureProxy{dial: func(context.Context) error { order = append(order, "ipv6"); return io.EOF }}
	wrapped := wrapPhysicalFamilies(&CProxy{Proxy: ipv4}, familyPath("ipv4"), &CProxy{Proxy: ipv6}, familyPath("ipv6"), time.Second)
	defer wrapped.Close()
	for i := 0; i < 2; i++ {
		if _, err := wrapped.DialContext(context.Background(), &C.Metadata{Host: "fixture.invalid", DstPort: 443}); err == nil {
			t.Fatal("failed adapters returned success")
		}
	}
	if strings.Join(order, ",") != "ipv4,ipv6,ipv4,ipv6" || ipv4.closed.Load() != 0 {
		t.Fatalf("failed family was pinned or closed: order=%v close4=%d", order, ipv4.closed.Load())
	}
}
