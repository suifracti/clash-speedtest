//go:build windows

package speedtester

import (
	"context"
	"net"
	"net/netip"
	"syscall"
	"testing"
	"time"

	"github.com/metacubex/mihomo/component/dialer"
)

func TestPickPhysicalAdapterDNSFiltersLinkLocalLoopbackAndFakeIP(t *testing.T) {
	got := pickPhysicalAdapterDNS([]netip.Addr{
		netip.MustParseAddr("fe80::1"),
		netip.MustParseAddr("127.0.0.1"),
		netip.MustParseAddr("198.18.0.2"),
		netip.MustParseAddr("fdfe:dcba:9876::2"),
		netip.MustParseAddr("169.254.1.1"),
		netip.MustParseAddr("192.168.1.1"),
	})
	if got != "192.168.1.1" {
		t.Fatalf("pickPhysicalAdapterDNS = %q, want 192.168.1.1", got)
	}
	if empty := pickPhysicalAdapterDNS([]netip.Addr{netip.MustParseAddr("fe80::1"), netip.MustParseAddr("198.18.0.2")}); empty != "" {
		t.Fatalf("expected empty when only link-local/fake-ip DNS exists, got %q", empty)
	}
}

func TestWindowsPhysicalInterfaceDNSAndSocketBindVerification(t *testing.T) {
	ifaceName := AutoDetectPhysicalInterface()
	if ifaceName == "" {
		t.Skip("no physical default-gateway interface detected on this Windows host")
	}
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		t.Fatalf("InterfaceByName(%q): %v", ifaceName, err)
	}
	dns, mode, err := detectPhysicalInterfaceDNS(context.Background(), ifaceName, iface.Index)
	if err != nil {
		t.Fatalf("detectPhysicalInterfaceDNS(%q, %d): %v", ifaceName, iface.Index, err)
	}
	if mode != "windows_adapter_dns_v1" {
		t.Fatalf("unexpected DNS mode %q", mode)
	}
	if dns == "" {
		t.Skipf("physical adapter %q (%d) has no usable IPv4 DNS configured", ifaceName, iface.Index)
	}
	t.Logf("detected physical interface=%q index=%d dns=%s mode=%s", ifaceName, iface.Index, dns, mode)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Unbound socket must fail verification (fail-closed).
	unbound, err := (&net.Dialer{}).DialContext(ctx, "udp", net.JoinHostPort(dns, "53"))
	if err != nil {
		t.Fatalf("unbound UDP dial: %v", err)
	}
	defer unbound.Close()
	rawUnbound, ok := unbound.(syscall.Conn)
	if !ok {
		t.Fatal("unbound UDP conn is not syscall.Conn")
	}
	egress := newPhysicalDownloadEgress()
	egress.iface, egress.index = ifaceName, iface.Index
	if err := egress.verify(rawUnbound, true, true); err == nil {
		t.Fatal("expected unbound socket verification to fail closed")
	}

	// Interface-bound socket via mihomo dialer must pass IP_UNICAST_IF verification.
	bound, err := dialer.DialContext(ctx, "udp", net.JoinHostPort(dns, "53"),
		dialer.WithInterface(ifaceName), dialer.WithOnlySingleStack(true), dialer.WithFallbackBind(false))
	if err != nil {
		t.Fatalf("bound UDP dial: %v", err)
	}
	defer bound.Close()
	rawBound, ok := bound.(syscall.Conn)
	if !ok {
		t.Fatal("bound UDP conn is not syscall.Conn")
	}
	if err := egress.verify(rawBound, true, true); err != nil {
		t.Fatalf("expected bound IP_UNICAST_IF socket verification to succeed on %q (%d): %v", ifaceName, iface.Index, err)
	}
	snap := egress.Snapshot()
	if !snap.DNSBindVerified {
		t.Fatalf("expected DNSBindVerified=true, got %+v", snap)
	}

	// ListenPacket must bind & verify UDP wildcard sockets via the physical adapter index.
	pktConn, err := egress.ListenPacket(ctx, "udp", ":0", netip.AddrPortFrom(netip.MustParseAddr(dns), 53))
	if err != nil {
		t.Fatalf("egress.ListenPacket failed on %q (%d): %v", ifaceName, iface.Index, err)
	}
	pktConn.Close()
	if snap = egress.Snapshot(); !snap.SocketBindVerified || snap.UDPBindings != 1 {
		t.Fatalf("expected SocketBindVerified=true UDPBindings=1, got %+v", snap)
	}
}

func TestWindowsPhysicalPathRejectsSocketHookAndFakeIP(t *testing.T) {
	originalHook := dialer.DefaultSocketHook
	t.Cleanup(func() { dialer.DefaultSocketHook = originalHook })
	dialer.DefaultSocketHook = func(network, address string, c syscall.RawConn) error { return nil }

	path := PlanPhysicalDownloadPath(context.Background(), &CProxy{Config: map[string]any{"server": "203.0.113.10", "type": "ss"}})
	if path.FailureReason != "external_socket_hook" || path.SocketBindVerified {
		t.Fatalf("expected external_socket_hook failure without verified socket, got %+v", path)
	}

	dialer.DefaultSocketHook = nil
	fakePath := PlanPhysicalDownloadPath(context.Background(), &CProxy{Config: map[string]any{"server": "198.18.0.50", "type": "ss"}})
	if fakePath.FailureReason == "" || fakePath.SocketBindVerified {
		t.Fatalf("expected Fake-IP server rejection on Windows physical path, got %+v", fakePath)
	}
	if _, err := ResolvePhysicalHost(context.Background(), "198.18.0.25", time.Second); err == nil {
		t.Fatal("expected ResolvePhysicalHost to reject Fake-IPv4 literal on Windows")
	}
	if _, err := ResolvePhysicalHost(context.Background(), "fdfe:dcba:9876::1", time.Second); err == nil {
		t.Fatal("expected ResolvePhysicalHost to reject Fake-IPv6 literal on Windows")
	}
}
