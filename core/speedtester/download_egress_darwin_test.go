//go:build darwin

package speedtester

import (
	"context"
	"net"
	"net/netip"
	"testing"
)

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
