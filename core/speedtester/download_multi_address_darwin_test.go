//go:build darwin

package speedtester

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// The DNS wire fixture stays in memory: no resolver, network socket, or node.
func multiAddressDNSFixture(t *testing.T, records []string) *PhysicalDownloadEgress {
	t.Helper()
	e := newPhysicalDownloadEgress()
	e.dnsDial = func(ctx context.Context, network string) (net.Conn, error) {
		c, peer := net.Pipe()
		go func() {
			defer peer.Close()
			dc := &dns.Conn{Conn: peer}
			q, err := dc.ReadMsg()
			if err != nil {
				return
			}
			r := new(dns.Msg)
			r.SetReply(q)
			for _, record := range records {
				rr, err := dns.NewRR(record)
				if err != nil {
					panic(err)
				}
				if rr.Header().Rrtype == q.Question[0].Qtype {
					r.Answer = append(r.Answer, rr)
				}
			}
			_ = dc.WriteMsg(r)
		}()
		return c, nil
	}
	return e
}

func preparedMultiAddressFixture(t *testing.T, kind string, limit int) *PhysicalDownloadEgress {
	t.Helper()
	e := multiAddressDNSFixture(t, []string{
		"node.example. 60 IN A 203.0.113.9",
		"node.example. 60 IN A 203.0.113.10",
	})
	ip, err := e.resolve(context.Background(), "node.example")
	if err != nil {
		t.Fatal(err)
	}
	e.freezeNodeAddresses("node.example", ip, kind, limit)
	return e
}

type multiAddressFixtureConn struct {
	countingNetConn
	closed bool
}

func (c *multiAddressFixtureConn) Close() error { c.closed = true; return nil }

func TestPhysicalMultiAddressTCPFallbackSharesDeadlineAndKeepsConnection(t *testing.T) {
	e := preparedMultiAddressFixture(t, "vless", 6)
	ctx, cancel := context.WithTimeout(context.Background(), 160*time.Millisecond)
	defer cancel()
	deadline, _ := ctx.Deadline()
	var deadlines []time.Time
	var addresses []string
	good := &multiAddressFixtureConn{}
	conn, err := e.dialPhysicalTCP(ctx, "tcp", "node.example:443", func(ctx context.Context, _, address string) (net.Conn, error) {
		d, ok := ctx.Deadline()
		if !ok {
			t.Fatal("unbounded physical dial")
		}
		deadlines = append(deadlines, d)
		addresses = append(addresses, address)
		if len(addresses) == 1 {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return good, nil
	})
	if err != nil || conn != good || good.closed {
		t.Fatalf("fallback connection was lost: %v closed=%v", err, good.closed)
	}
	if len(addresses) != 2 || addresses[0] != "203.0.113.9:443" || addresses[1] != "203.0.113.10:443" {
		t.Fatalf("unexpected candidates: %v", addresses)
	}
	if !deadlines[0].Before(deadline.Add(-40*time.Millisecond)) || deadlines[1].After(deadline) {
		t.Fatalf("first candidate consumed or extended shared deadline: %v", deadlines)
	}
	if _, err := good.Write([]byte("later TLS handshake")); err != nil || good.closed {
		t.Fatal("dial child cancellation closed the returned connection")
	}
	p := e.Snapshot()
	if p.Method != "physical_socket_v1" || p.AddressSource != "physical_ipv4_dns_multi_v2" || p.TCPDialAttempts != 2 || p.TCPConnectFailed != 1 || p.TCPConnectSucceeded != 1 || p.FailureReason != "" || p.DNSRequests != 1 {
		t.Fatalf("fallback success evidence is wrong: %+v", p)
	}
}

func TestPhysicalMultiAddressTCPStopsAtFirstSuccessAndFiveSeconds(t *testing.T) {
	e := preparedMultiAddressFixture(t, "vless", 6)
	before := time.Now()
	calls := 0
	conn, err := e.dialPhysicalTCP(context.Background(), "tcp", "node.example:443", func(ctx context.Context, _, _ string) (net.Conn, error) {
		calls++
		deadline, ok := ctx.Deadline()
		if !ok || deadline.After(before.Add(3*time.Second)) || deadline.Before(before.Add(2*time.Second)) {
			t.Fatal("first of two candidates did not receive half the 5s ceiling")
		}
		return &multiAddressFixtureConn{}, nil
	})
	if err != nil || conn == nil || calls != 1 || e.Snapshot().TCPDialAttempts != 1 {
		t.Fatalf("success triggered an extra connection: calls=%d err=%v", calls, err)
	}
}

func TestPhysicalMultiAddressTCPFrozenFirstIPKeepsFallback(t *testing.T) {
	e := preparedMultiAddressFixture(t, "vless", 6)
	var addresses []string
	_, err := e.dialPhysicalTCP(context.Background(), "tcp", "203.0.113.9:443", func(_ context.Context, _, address string) (net.Conn, error) {
		addresses = append(addresses, address)
		if address == "203.0.113.9:443" {
			return nil, syscall.ECONNREFUSED
		}
		return &multiAddressFixtureConn{}, nil
	})
	if err != nil || len(addresses) != 2 || addresses[1] != "203.0.113.10:443" {
		t.Fatalf("frozen first-IP alias lost its DNS fallback: %v %v", addresses, err)
	}
}

func TestPhysicalMultiAddressDoesNotExpandAAAAFallback(t *testing.T) {
	e := multiAddressDNSFixture(t, []string{"node.example. 60 IN AAAA 2001:db8::9", "node.example. 60 IN AAAA 2001:db8::10"})
	ip, err := e.resolve(context.Background(), "node.example")
	if err != nil || ip != netip.MustParseAddr("2001:db8::9") {
		t.Fatalf("existing AAAA selection changed: %v %v", ip, err)
	}
	e.freezeNodeAddresses("node.example", ip, "vless", 4)
	if _, err := e.resolve(context.Background(), "2001:db8::10"); err == nil {
		t.Fatal("IPv6 gained an unverified second address")
	}
	p := e.Snapshot()
	if p.Method != "physical_socket_v1" || p.DNSRequests != 2 || p.AddressFamily != "ipv6" {
		t.Fatalf("existing NODATA->AAAA policy changed: %+v", p)
	}
}

func TestPhysicalMultiAddressTCPCancellationAndLateConnection(t *testing.T) {
	for _, lateSuccess := range []bool{false, true} {
		e := preparedMultiAddressFixture(t, "vless", 6)
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		late := &multiAddressFixtureConn{}
		conn, err := e.dialPhysicalTCP(ctx, "tcp", "node.example:443", func(context.Context, string, string) (net.Conn, error) {
			calls++
			cancel()
			if lateSuccess {
				return late, nil
			}
			return nil, context.Canceled
		})
		cancel()
		if conn != nil || !errors.Is(err, context.Canceled) || calls != 1 || (lateSuccess && !late.closed) {
			t.Fatalf("cancel retried or leaked a late connection: late=%v calls=%d err=%v closed=%v", lateSuccess, calls, err, late.closed)
		}
	}
}

func TestPhysicalMultiAddressTCPAllFailedWithinOriginalDeadline(t *testing.T) {
	e := preparedMultiAddressFixture(t, "vless", 6)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	deadline, _ := ctx.Deadline()
	calls := 0
	_, err := e.dialPhysicalTCP(ctx, "tcp", "node.example:443", func(ctx context.Context, _, _ string) (net.Conn, error) {
		calls++
		d, _ := ctx.Deadline()
		if d.After(deadline) {
			t.Fatal("fallback extended the original deadline")
		}
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) || calls != 2 || e.Snapshot().TCPDialAttempts != 2 {
		t.Fatalf("all-failed result: %v calls=%d", err, calls)
	}
}

func TestPhysicalMultiAddressTCPBudgetCountsEveryAttemptConcurrently(t *testing.T) {
	e := preparedMultiAddressFixture(t, "vless", 6)
	var calls atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = e.dialPhysicalTCP(context.Background(), "tcp", "node.example:443", func(context.Context, string, string) (net.Conn, error) {
				calls.Add(1)
				return nil, syscall.ECONNREFUSED
			})
		}()
	}
	wg.Wait()
	if calls.Load() != 12 || e.Snapshot().TCPDialAttempts != 12 {
		t.Fatalf("physical attempts exceeded or missed 2x limit: %d %+v", calls.Load(), e.Snapshot())
	}
	_, err := e.dialPhysicalTCP(context.Background(), "tcp", "node.example:443", func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("exhausted budget reached socket boundary")
		return nil, nil
	})
	var pathErr *PhysicalPathError
	if !errors.As(err, &pathErr) || !errors.Is(err, errPhysicalDownloadBudgetExceeded) {
		t.Fatalf("budget exhaustion misreported as node failure: %v", err)
	}
	for i := 0; i < 6; i++ {
		if err := e.reserveDialBudget(context.Background(), true); err != nil {
			t.Fatal(err)
		}
	}
	if !errors.Is(e.reserveDialBudget(context.Background(), true), errPhysicalDownloadBudgetExceeded) {
		t.Fatal("TCP fallback increased UDP budget")
	}
}

func TestPhysicalMultiAddressQUICAndLiteralRemainSingle(t *testing.T) {
	for _, kind := range []string{"hysteria2", "hysteria", "tuic"} {
		e := preparedMultiAddressFixture(t, kind, 4)
		if _, err := e.resolve(context.Background(), "203.0.113.10"); err == nil {
			t.Fatalf("%s gained an alternate UDP destination", kind)
		}
		if e.Snapshot().Method != "physical_socket_v1" {
			t.Fatal("QUIC method changed")
		}
		for i := 0; i < 4; i++ {
			if err := e.reserveDialBudget(context.Background(), false); err != nil {
				t.Fatal(err)
			}
		}
		if !errors.Is(e.reserveDialBudget(context.Background(), false), errPhysicalDownloadBudgetExceeded) {
			t.Fatal("QUIC budget changed")
		}
	}
	e := newPhysicalDownloadEgress()
	e.freezeNodeAddresses("203.0.113.9", netip.MustParseAddr("203.0.113.9"), "vless", 4)
	if e.Snapshot().Method != "physical_socket_v1" {
		t.Fatal("single address method changed")
	}
}

func TestPhysicalMultiAddressDNSFreezesTwoUniqueCandidates(t *testing.T) {
	e := multiAddressDNSFixture(t, []string{
		"node.example. 60 IN A 198.18.0.9",
		"node.example. 60 IN A 203.0.113.9",
		"node.example. 60 IN A 203.0.113.9",
		"node.example. 60 IN A 203.0.113.10",
		"node.example. 60 IN A 203.0.113.11",
	})
	ip, err := e.resolve(context.Background(), "node.example")
	if err != nil || ip != netip.MustParseAddr("203.0.113.9") {
		t.Fatalf("first candidate: %v %v", ip, err)
	}
	e.host, e.nodeIP = "node.example", ip
	got, err := e.resolve(context.Background(), "203.0.113.10")
	if err != nil || got != netip.MustParseAddr("203.0.113.10") {
		t.Fatalf("second validated A was discarded: %v %v", got, err)
	}
	for _, outside := range []string{"203.0.113.11", "198.18.0.9", "unrelated.example"} {
		if _, err := e.resolve(context.Background(), outside); err == nil {
			t.Fatalf("accepted destination outside frozen pair: %s", outside)
		}
	}
	if p := e.Snapshot(); p.DNSRequests != 1 || p.DNSDialAttempts != 1 {
		t.Fatalf("candidate reuse performed extra DNS: %+v", p)
	}
}

// Download rules freeze the binding method before DNS. DNS address selection
// must retain that method and version its address source so valid results remain
// comparable without mixing old single-address conditions with the new policy.
func TestPhysicalMultiAddressPathRetainsPlannedBindingMethod(t *testing.T) {
	planned := newPhysicalDownloadEgress().Snapshot()
	actual := preparedMultiAddressFixture(t, "vless", 4).Snapshot()
	if actual.Method != planned.Method {
		t.Fatalf("multi-address path no longer matches the persisted download rule: plan=%s actual=%s", planned.Method, actual.Method)
	}
	if actual.AddressSource != "physical_ipv4_dns_multi_v2" {
		t.Fatalf("multi-address DNS policy cannot be distinguished from old results: %s", actual.AddressSource)
	}
}

func TestPhysicalMultiAddressRemembersConnectedIP(t *testing.T) {
	e := preparedMultiAddressFixture(t, "vless", 6)
	var calls []string
	dial := func(_ context.Context, _, address string) (net.Conn, error) {
		calls = append(calls, address)
		if address == "203.0.113.9:443" {
			return nil, syscall.ECONNREFUSED
		}
		return &multiAddressFixtureConn{}, nil
	}
	for i := 0; i < 2; i++ {
		if _, err := e.dialPhysicalTCP(context.Background(), "tcp", "node.example:443", dial); err != nil {
			t.Fatal(err)
		}
	}
	if len(calls) != 3 || calls[2] != "203.0.113.10:443" {
		t.Fatalf("repeated the known failed first address: %v", calls)
	}
}
