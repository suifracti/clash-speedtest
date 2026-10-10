//go:build darwin

package speedtester

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"testing"
	"time"
)

type countingNetConn struct{ writes int }

func (c *countingNetConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *countingNetConn) Write(p []byte) (int, error)      { c.writes++; return len(p), nil }
func (*countingNetConn) Close() error                       { return nil }
func (*countingNetConn) LocalAddr() net.Addr                { return &net.IPAddr{} }
func (*countingNetConn) RemoteAddr() net.Addr               { return &net.IPAddr{} }
func (*countingNetConn) SetDeadline(_ time.Time) error      { return nil }
func (*countingNetConn) SetReadDeadline(_ time.Time) error  { return nil }
func (*countingNetConn) SetWriteDeadline(_ time.Time) error { return nil }

func TestPhysicalDownloadRejectsUnboundActualSocket(t *testing.T) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	e := &PhysicalDownloadEgress{index: 99999}
	if err := e.verify(conn, true, false); err == nil {
		t.Fatal("unbound actual socket accepted")
	}
	if e.Snapshot().SocketBindVerified {
		t.Fatal("failed verification marked successful")
	}
}

func TestPhysicalDownloadUDPVerifiesActualDualStackSocket(t *testing.T) {
	name := AutoDetectPhysicalInterface()
	if name == "" {
		t.Skip("no physical interface on test host")
	}
	iface, err := net.InterfaceByName(name)
	if err != nil {
		t.Fatal(err)
	}
	e := &PhysicalDownloadEgress{iface: name, index: iface.Index}
	// Bind a local UDP listener; no packet is sent to this documentation peer.
	conn, err := e.ListenPacket(context.Background(), "udp", ":0", netip.MustParseAddrPort("203.0.113.9:443"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if got := e.Snapshot(); !got.SocketBindVerified || got.UDPBindings != 1 {
		t.Fatalf("actual UDP bind not verified: %+v", got)
	}
}
func TestPhysicalDownloadFrozenResolutionDoesNotUseSystemDNS(t *testing.T) {
	e := &PhysicalDownloadEgress{host: "original.example", nodeIP: netip.MustParseAddr("203.0.113.9")}
	ip, err := e.resolve(context.Background(), "original.example")
	if err != nil || ip != e.nodeIP {
		t.Fatal("prepared address was not reused")
	}
}

func TestPhysicalDNSWriteBudgetStopsRetryAndHonorsCancellation(t *testing.T) {
	conn := &countingNetConn{}
	budget := newPhysicalRequestBudget(1)
	ctx, cancel := context.WithCancel(context.Background())
	dns := &physicalDNSConn{Conn: conn, ctx: ctx, budget: budget}
	if _, err := dns.Write([]byte("fixture query")); err != nil {
		t.Fatal(err)
	}
	if _, err := dns.Write([]byte("fixture retry")); !errors.Is(err, errPhysicalDownloadBudgetExceeded) {
		t.Fatalf("DNS retry should exhaust its explicit request budget: %v", err)
	}
	if conn.writes != 1 || budget.count() != 1 {
		t.Fatalf("budgeted DNS requests = writes %d, reserved %d", conn.writes, budget.count())
	}
	cancel()
	if _, err := dns.Write([]byte("cancelled follow-up")); !errors.Is(err, context.Canceled) || conn.writes != 1 {
		t.Fatalf("cancelled context allowed another DNS request: err=%v writes=%d", err, conn.writes)
	}
}

func TestPhysicalPathSnapshotRecordsIPv4MethodAndUnobservedTUN(t *testing.T) {
	e := &PhysicalDownloadEgress{
		iface: "en1", dnsMode: "physical_interface_dns_v1", addressFamily: "ipv4",
		resolutionSource: "physical_ipv4_dns",
		dnsRequests:      newPhysicalRequestBudget(physicalDownloadDNSRequestLimit),
		dnsDials:         newPhysicalRequestBudget(physicalDownloadDNSRequestLimit),
		tcpDials:         newPhysicalRequestBudget(physicalDownloadSocketRequestLimit),
		udpDials:         newPhysicalRequestBudget(physicalDownloadSocketRequestLimit),
	}
	if err := e.dnsRequests.reserve(context.Background()); err != nil {
		t.Fatal(err)
	}
	path := e.Snapshot()
	if path.Method != "physical_socket_v1" || path.Interface != "en1" || path.AddressFamily != "ipv4" || path.ResolutionSource != "physical_ipv4_dns" || path.DNSRequests != 1 || path.TUNEvidence != "packet_route_not_observed" || path.SocketBindVerified {
		t.Fatalf("physical path snapshot lost or overstated evidence: %+v", path)
	}
}

func TestPhysicalIPv4DNSFailureReasonIsSpecific(t *testing.T) {
	e := &PhysicalDownloadEgress{dnsRequests: newPhysicalRequestBudget(physicalDownloadDNSRequestLimit), dnsDials: newPhysicalRequestBudget(physicalDownloadDNSRequestLimit), tcpDials: newPhysicalRequestBudget(physicalDownloadSocketRequestLimit), udpDials: newPhysicalRequestBudget(physicalDownloadSocketRequestLimit)}
	e.setFailure(physicalFailureReason(context.Background(), &net.DNSError{IsNotFound: true}))
	if got := e.Snapshot().FailureReason; got != "ipv4_dns_not_found" {
		t.Fatalf("IPv4 DNS failure not classified: %q", got)
	}
}

func TestPhysicalDownloadDoesNotBindUnverifiedIPv6PacketSocket(t *testing.T) {
	e := &PhysicalDownloadEgress{udpDials: newPhysicalRequestBudget(physicalDownloadSocketRequestLimit)}
	_, err := e.ListenPacket(context.Background(), "udp", ":0", netip.MustParseAddrPort("[2001:db8::9]:443"))
	if err == nil || e.udpDials.count() != 0 || e.Snapshot().FailureReason != "unverified_ipv6_socket_family" {
		t.Fatalf("B3 must not attempt an unverified IPv6 socket: err=%v path=%+v", err, e.Snapshot())
	}
}

func TestPhysicalScopedDNSExcludesVPNAndFakeListeners(t *testing.T) {
	text := "resolver #1\n nameserver[0] : 1.1.1.1\n if_index : 26 (utun1024)\n\nresolver #2\n nameserver[0] : 198.18.0.1\n nameserver[1] : 114.114.114.114\n if_index : 15 (en1)\n"
	if got := physicalScopedDNS(text, "en1", 15); got != "114.114.114.114" {
		t.Fatalf("wrong DNS: %q", got)
	}
	if got := physicalScopedDNS(text, "en0", 14); got != "" {
		t.Fatalf("adopted unrelated resolver: %q", got)
	}
}

func TestPhysicalDownloadConfigPreservesTLSIdentityAndOriginalScope(t *testing.T) {
	original := map[string]any{"type": "hysteria2", "server": "original.example", "password": "private-fixture", "interface-name": "utun1024"}
	got, err := clonePhysicalProxyConfig(original, netip.MustParseAddr("203.0.113.9"), "en1")
	if err != nil {
		t.Fatal(err)
	}
	if got["server"] != "203.0.113.9" || got["sni"] != "original.example" || got["interface-name"] != "en1" || original["server"] != "original.example" || original["interface-name"] != "utun1024" {
		t.Fatal("isolation changed subscription or TLS hostname")
	}
	original["sni"] = "explicit.example"
	got, err = clonePhysicalProxyConfig(original, netip.MustParseAddr("203.0.113.9"), "en1")
	if err != nil || got["sni"] != "explicit.example" {
		t.Fatal("explicit TLS identity was overwritten")
	}
	original["dialer-proxy"] = "outer-proxy"
	if _, err = clonePhysicalProxyConfig(original, netip.MustParseAddr("203.0.113.9"), "en1"); err == nil {
		t.Fatal("explicit second proxy was silently accepted")
	}
}

func TestVLESSConfigKeepsTLSServerNameAcrossPhysicalPreparation(t *testing.T) {
	original := map[string]any{
		"type": "vless", "server": "node.example", "port": 443, "uuid": "fixture-uuid",
		"tls": true, "servername": "tls.example", "network": "ws",
	}
	got, err := clonePhysicalProxyConfig(original, netip.MustParseAddr("203.0.113.9"), "en1")
	if err != nil {
		t.Fatal(err)
	}
	if got["server"] != "node.example" || got["servername"] != "tls.example" || got["tls"] != true || got["uuid"] != "fixture-uuid" || got["interface-name"] != "en1" {
		t.Fatalf("offline VLESS/TLS fixture changed proxy identity: %+v", got)
	}
}
func TestPhysicalDownloadRejectsVirtualAndLocalUpstreams(t *testing.T) {
	for _, value := range []string{"127.0.0.1", "::1", "198.18.1.10", "198.19.255.1", "169.254.1.2", "0.0.0.0"} {
		if usablePhysicalAddress(netip.MustParseAddr(value)) {
			t.Fatalf("unsafe upstream accepted: %s", value)
		}
	}
	if !usablePhysicalAddress(netip.MustParseAddr("192.168.1.1")) {
		t.Fatal("physical LAN DNS gateway rejected")
	}
}
