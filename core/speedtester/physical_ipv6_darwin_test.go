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

	"github.com/metacubex/mihomo/adapter"
	C "github.com/metacubex/mihomo/constant"
)

// Stop before creating any socket, while exercising the real HY2 resolver.
type familyFixtureDialer struct{ calls atomic.Int32 }

var errFamilyFixture = errors.New("offline packet dial intercepted")

func (d *familyFixtureDialer) DialContext(context.Context, string, string) (net.Conn, error) {
	d.calls.Add(1)
	return nil, errFamilyFixture
}
func (d *familyFixtureDialer) ListenPacket(context.Context, string, string, netip.AddrPort) (net.PacketConn, error) {
	d.calls.Add(1)
	return nil, errFamilyFixture
}

func TestPhysicalIPv6LiteralKeepsFamilyWithoutDNS(t *testing.T) {
	e := newPhysicalDownloadEgress()
	ip, err := e.resolve(context.Background(), "2001:db8::9")
	path := e.Snapshot()
	if err != nil || ip.String() != "2001:db8::9" || path.AddressFamily != "ipv6" || path.AddressSource != "literal_ip" || path.DNSRequests != 0 {
		t.Fatalf("literal IPv6 changed or sent to DNS: ip=%v err=%v path=%+v", ip, err, path)
	}
}

func TestPhysicalHysteriaFrozenFamiliesReachPrivateDialer(t *testing.T) {
	for _, server := range []string{"203.0.113.9", "2001:db8::9"} {
		t.Run(server, func(t *testing.T) {
			d := &familyFixtureDialer{}
			original := map[string]any{"type": "hysteria2", "name": "offline-family", "server": "node.example", "port": 443, "password": "fixture-only", "sni": "tls.example"}
			config, err := clonePhysicalProxyConfig(original, netip.MustParseAddr(server), "en1")
			if err != nil {
				t.Fatal(err)
			}
			p, err := adapter.ParseProxy(config, adapter.WithDialerForAPI(d))
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			conn, err := p.DialContext(ctx, &C.Metadata{NetWork: C.TCP, Host: "target.invalid", DstPort: 443})
			if conn != nil {
				conn.Close()
			}
			if !errors.Is(err, errFamilyFixture) || d.calls.Load() != 1 {
				t.Fatalf("frozen server rejected before private dialer: err=%v calls=%d", err, d.calls.Load())
			}
			if config["sni"] != "tls.example" || original["server"] != "node.example" || original["password"] != "fixture-only" {
				t.Fatal("source identity or TLS name changed")
			}
		})
	}
}

func TestPhysicalIPv6UDPListenerVerifiesActualInterfaceWithoutPackets(t *testing.T) {
	name := AutoDetectPhysicalInterface()
	if name == "" {
		t.Skip("no physical interface on test host")
	}
	iface, err := net.InterfaceByName(name)
	if err != nil {
		t.Fatal(err)
	}
	e := newPhysicalDownloadEgress()
	e.iface, e.index = name, iface.Index
	// A UDP listener does not connect or send to this documentation address.
	conn, err := e.ListenPacket(context.Background(), "udp", ":0", netip.MustParseAddrPort("[2001:db8::9]:443"))
	if err != nil {
		t.Fatalf("IPv6 listener: %v cause=%v path=%+v", err, errors.Unwrap(err), e.Snapshot())
	}
	defer conn.Close()
	addr, err := netip.ParseAddrPort(conn.LocalAddr().String())
	path := e.Snapshot()
	if err != nil || !addr.Addr().Is6() || !path.SocketBindVerified || path.UDPBindings != 1 || path.UDPDialAttempts != 1 || path.TUNEvidence != "packet_route_not_observed" {
		t.Fatalf("IPv6 binding evidence missing or overstated: local=%v path=%+v", conn.LocalAddr(), path)
	}
}

func TestPhysicalIPv6NeedsUsableSourceAndKeepsConfiguredFamily(t *testing.T) {
	for _, tt := range []struct {
		prefix string
		want   bool
	}{
		{"fe80::9/64", false}, {"::1/128", false}, {"198.18.0.1/15", false},
		{"fdfe:dcba:9876::9/64", false}, {"2001:db8::9/64", true},
	} {
		p := netip.MustParsePrefix(tt.prefix)
		addrs := []net.Addr{&net.IPNet{IP: net.IP(p.Addr().AsSlice()), Mask: net.CIDRMask(p.Bits(), p.Addr().BitLen())}}
		if got := usablePhysicalIPv6(addrs); got != tt.want {
			t.Fatalf("source %s: got %v want %v", tt.prefix, got, tt.want)
		}
	}
	for _, version := range []string{"", "ipv4"} {
		e := newPhysicalDownloadEgress()
		_, err := e.physicalNodeConfig(map[string]any{"type": "hysteria2", "server": "node.example", "ip-version": version}, netip.MustParseAddr("2001:db8::9"), "missing-fixture-interface")
		var physical *PhysicalPathError
		want := "physical_ipv6_interface_unavailable"
		if version == "ipv4" {
			want = "node_address_family_mismatch"
		}
		if !errors.As(err, &physical) || e.Snapshot().FailureReason != want || e.Snapshot().UDPDialAttempts != 0 {
			t.Fatalf("unusable source started a measurement: version=%q err=%v path=%+v", version, err, e.Snapshot())
		}
	}
}

func TestPhysicalDNSRespectsExplicitNodeFamily(t *testing.T) {
	for _, tt := range []struct {
		version string
		want    string
		reason  string
	}{
		{"ipv6", "2001:db8::9", ""},
		{"ipv4", "", "a_nodata"},
	} {
		t.Run(tt.version, func(t *testing.T) {
			e := multiAddressDNSFixture(t, []string{"node.example. 60 IN AAAA 2001:db8::9"})
			e.nodeIPVersion = tt.version
			ip, err := e.resolve(context.Background(), "node.example")
			path := e.Snapshot()
			if tt.want != "" && (err != nil || ip.String() != tt.want || path.AddressFamily != "ipv6") {
				t.Fatalf("IPv6-only source fell back: ip=%v err=%v path=%+v", ip, err, path)
			}
			if tt.want == "" && (err == nil || path.FailureReason != tt.reason) {
				t.Fatalf("IPv4-only source used AAAA: ip=%v err=%v path=%+v", ip, err, path)
			}
			if path.DNSRequests != 1 || path.DNSDialAttempts != 1 || path.UDPDialAttempts != 0 || path.TCPDialAttempts != 0 {
				t.Fatalf("explicit family issued extra requests: %+v", path)
			}
		})
	}
}

func TestPhysicalIPv6PreparationOnCapableInterfacePreservesSource(t *testing.T) {
	iface, err := net.InterfaceByName(AutoDetectPhysicalInterface())
	if err != nil {
		t.Skip("no physical interface on test host")
	}
	addrs, err := iface.Addrs()
	if err != nil || !usablePhysicalIPv6(addrs) {
		t.Skip("test host physical interface has no usable IPv6 source")
	}
	for _, kind := range []string{"hysteria2", "vless"} {
		e := newPhysicalDownloadEgress()
		original := map[string]any{"type": kind, "server": "node.example", "sni": "tls.example", "password": "fixture-only"}
		config, err := e.physicalNodeConfig(original, netip.MustParseAddr("2001:db8::9"), iface.Name)
		if err != nil || config["interface-name"] != iface.Name || config["sni"] != "tls.example" || config["password"] != "fixture-only" || original["server"] != "node.example" {
			t.Fatalf("IPv6 preparation lost source identity: type=%s err=%v", kind, err)
		}
		if e.Snapshot().UDPDialAttempts != 0 || e.Snapshot().TCPDialAttempts != 0 {
			t.Fatal("configuration preparation opened a socket")
		}
	}
}
