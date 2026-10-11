//go:build darwin

package speedtester

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestPhysicalDNSResolverPreservesUDPWireFraming(t *testing.T) {
	server, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	var packets atomic.Int32
	go func() {
		b := make([]byte, 512)
		for {
			n, peer, err := server.ReadFrom(b)
			if err != nil {
				return
			}
			packets.Add(1)
			q := new(dns.Msg)
			if q.Unpack(b[:n]) != nil || len(q.Question) != 1 {
				continue
			}
			response := new(dns.Msg)
			response.SetReply(q)
			response.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: q.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60}, A: net.IPv4(203, 0, 113, 9)}}
			wire, _ := response.Pack()
			_, _ = server.WriteTo(wire, peer)
		}
	}()
	budget := newPhysicalRequestBudget(1)
	var dials atomic.Int32
	resolver := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		dials.Add(1)
		conn, err := (&net.Dialer{}).DialContext(ctx, network, server.LocalAddr().String())
		if err != nil {
			return nil, err
		}
		return wrapPhysicalDNSConn(conn, ctx, budget), nil
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	ips, err := resolver.LookupNetIP(ctx, "ip4", "framing.fixture.example")
	if err != nil || len(ips) != 1 || ips[0] != netip.MustParseAddr("203.0.113.9") {
		t.Fatalf("physical DNS wrapper broke UDP resolution: ips=%v err=%v packets=%d writes=%d", ips, err, packets.Load(), budget.count())
	}
	if packets.Load() != 1 || budget.count() != 1 || dials.Load() != 1 {
		t.Fatalf("unexpected DNS operations: packets=%d writes=%d dials=%d", packets.Load(), budget.count(), dials.Load())
	}
}

func TestPhysicalDNSWrapperKeepsTCPStreamFraming(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	if _, ok := wrapPhysicalDNSConn(a, context.Background(), newPhysicalRequestBudget(1)).(net.PacketConn); ok {
		t.Fatal("TCP DNS wrapper exposes datagram framing")
	}
}

type countingDNSPacketConn struct{ countingNetConn }

func (c *countingDNSPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	n, e := c.Read(p)
	return n, nil, e
}
func (c *countingDNSPacketConn) WriteTo(p []byte, _ net.Addr) (int, error) { return c.Write(p) }

func TestPhysicalDNSPacketWritesShareBudgetAndCancellation(t *testing.T) {
	conn := &countingDNSPacketConn{}
	budget := newPhysicalRequestBudget(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wrapped := wrapPhysicalDNSConn(conn, ctx, budget)
	packet, ok := wrapped.(net.PacketConn)
	if !ok {
		t.Fatal("lost UDP capability")
	}
	if _, err := packet.WriteTo([]byte("query"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := wrapped.Write([]byte("retry")); !errors.Is(err, errPhysicalDownloadBudgetExceeded) {
		t.Fatalf("stream path bypassed shared budget: %v", err)
	}
	cancel()
	if _, err := packet.WriteTo([]byte("cancelled"), nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled packet admitted: %v", err)
	}
	if budget.count() != 1 || conn.writes != 1 {
		t.Fatalf("extra DNS request admitted: budget=%d writes=%d", budget.count(), conn.writes)
	}
}
