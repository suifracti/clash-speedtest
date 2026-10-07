//go:build darwin

package speedtester

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/component/dialer"
	"golang.org/x/sys/unix"
)

type PhysicalDownloadEgress struct {
	iface       string
	index       int
	dns         string
	dnsMode     string
	host        string
	nodeIP      netip.Addr
	resolver    *net.Resolver
	mu          sync.Mutex
	tcp, udp    int
	dnsVerified bool
	prepared    time.Duration
	dnsDuration time.Duration
}

func (e *PhysicalDownloadEgress) Snapshot() DownloadNetworkPath {
	e.mu.Lock()
	defer e.mu.Unlock()
	return DownloadNetworkPath{Method: "physical_socket_v1", Interface: e.iface, DNSMode: e.dnsMode, DNSBindVerified: e.dnsVerified, SocketBindVerified: e.tcp+e.udp > 0, TCPBindings: e.tcp, UDPBindings: e.udp, PreparationDurationNS: e.prepared.Nanoseconds(), DNSDurationNS: e.dnsDuration.Nanoseconds()}
}

// Use only numeric DNS servers explicitly scoped to this physical interface;
// never adopt a VPN's scoped resolver or a loopback/fake-IP DNS listener.
func physicalScopedDNS(text, iface string, index int) string {
	for _, block := range strings.Split(text, "\n\n") {
		matched := false
		dns := ""
		for _, line := range strings.Split(block, "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 4 && fields[0] == "if_index" && fields[2] == fmt.Sprint(index) && fields[3] == "("+iface+")" {
				matched = true
			}
			if len(fields) == 3 && strings.HasPrefix(fields[0], "nameserver[") && dns == "" {
				if ip, err := netip.ParseAddr(fields[2]); err == nil && ip.Is4() && usablePhysicalAddress(ip) {
					dns = ip.String()
				}
			}
		}
		if matched && dns != "" {
			return dns
		}
	}
	return ""
}
func usablePhysicalAddress(ip netip.Addr) bool {
	return ip.IsValid() && ip.IsGlobalUnicast() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !netip.MustParsePrefix("198.18.0.0/15").Contains(ip.Unmap())
}
func (e *PhysicalDownloadEgress) resolve(ctx context.Context, host string) (netip.Addr, error) {
	// Freeze only this run's freshly resolved node address, outside the timed
	// body measurement. Keep the original hostname for TLS/HTTP identity.
	if host == e.host && e.nodeIP.IsValid() {
		return e.nodeIP, nil
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if !usablePhysicalAddress(ip) {
			return netip.Addr{}, fmt.Errorf("proxy server is local, virtual or fake-ip")
		}
		return ip, nil
	}
	markDownloadPhase(ctx, "dns")
	started := time.Now()
	ips, err := e.resolver.LookupNetIP(ctx, "ip4", host)
	e.mu.Lock()
	e.dnsDuration += time.Since(started)
	e.mu.Unlock()
	if err != nil {
		return netip.Addr{}, fmt.Errorf("physical DNS lookup unavailable: %w", err)
	}
	for _, ip := range ips {
		if usablePhysicalAddress(ip) {
			return ip.Unmap(), nil
		}
	}
	return netip.Addr{}, fmt.Errorf("physical DNS returned no usable non-fake address")
}

// Verify the actual socket option, rather than claiming bypass from a config
// string. A failed bind/verification never falls back to an unbound connection.
func (e *PhysicalDownloadEgress) verify(conn syscall.Conn, udp, dns bool) error {
	raw, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var index int
	var inner error
	err = raw.Control(func(fd uintptr) {
		// UDP listeners may use an IPv6 dual-stack socket for an IPv4 peer.
		// Verify its actual family, not the remote address family.
		addr, err := unix.Getsockname(int(fd))
		if err != nil {
			inner = err
			return
		}
		switch addr.(type) {
		case *unix.SockaddrInet6:
			index, inner = unix.GetsockoptInt(int(fd), unix.IPPROTO_IPV6, unix.IPV6_BOUND_IF)
		case *unix.SockaddrInet4:
			index, inner = unix.GetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_BOUND_IF)
		default:
			inner = fmt.Errorf("physical socket family cannot be verified")
		}
	})
	if err != nil {
		return err
	}
	if inner != nil {
		return inner
	}
	if index != e.index {
		return fmt.Errorf("physical socket interface verification failed")
	}
	e.mu.Lock()
	if dns {
		e.dnsVerified = true
	} else if udp {
		e.udp++
	} else {
		e.tcp++
	}
	e.mu.Unlock()
	return nil
}
func (e *PhysicalDownloadEgress) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	markDownloadPhase(ctx, "address_resolution")
	ip, err := e.resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	markDownloadPhase(ctx, "connection")
	conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port), dialer.WithInterface(e.iface), dialer.WithOnlySingleStack(ip.Is4()), dialer.WithFallbackBind(false))
	if err != nil {
		return nil, err
	}
	markDownloadPhase(ctx, "proxy_handshake")
	raw, ok := conn.(syscall.Conn)
	if !ok {
		conn.Close()
		return nil, &PhysicalPathError{Cause: fmt.Errorf("physical socket cannot be verified")}
	}
	if err = e.verify(raw, false, false); err != nil {
		conn.Close()
		return nil, &PhysicalPathError{Cause: err}
	}
	return conn, nil
}
func (e *PhysicalDownloadEgress) ListenPacket(ctx context.Context, network, address string, remote netip.AddrPort) (net.PacketConn, error) {
	conn, err := dialer.ListenPacket(ctx, network, address, remote, dialer.WithInterface(e.iface), dialer.WithFallbackBind(false))
	if err != nil {
		return nil, err
	}
	raw, ok := conn.(syscall.Conn)
	if !ok {
		conn.Close()
		return nil, &PhysicalPathError{Cause: fmt.Errorf("physical UDP socket cannot be verified")}
	}
	if err = e.verify(raw, true, false); err != nil {
		conn.Close()
		return nil, &PhysicalPathError{Cause: err}
	}
	return conn, nil
}
func clonePhysicalProxyConfig(original map[string]any, ip netip.Addr, iface string) (map[string]any, error) {
	host, _ := original["server"].(string)
	if host == "" || !usablePhysicalAddress(ip) {
		return nil, fmt.Errorf("proxy server isolation unavailable")
	}
	if value, _ := original["dialer-proxy"].(string); strings.TrimSpace(value) != "" {
		return nil, fmt.Errorf("explicit upstream proxy chain requires separate confirmation")
	}
	config := make(map[string]any, len(original)+2)
	for k, v := range original {
		config[k] = v
	}
	// QUIC adapters resolve the server outside their API dialer. Freeze a real
	// physical-DNS address while keeping the original TLS identity unchanged.
	typ, _ := original["type"].(string)
	if typ == "hysteria2" || typ == "hysteria" || typ == "tuic" {
		config["server"] = ip.String()
		if sni, _ := original["sni"].(string); sni == "" {
			config["sni"] = host
		}
	}
	config["interface-name"] = iface
	return config, nil
}
func PreparePhysicalDownloadProxy(ctx context.Context, original *CProxy) (*CProxy, *PhysicalDownloadEgress, error) {
	started := time.Now()
	if original == nil || len(original.Config) == 0 {
		return nil, nil, fmt.Errorf("download proxy config unavailable")
	}
	if dialer.DefaultSocketHook != nil {
		return nil, nil, fmt.Errorf("external socket hook prevents physical binding verification")
	}
	ifaceName := AutoDetectPhysicalInterface()
	if ifaceName == "" {
		return nil, nil, fmt.Errorf("no physical default gateway interface")
	}
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	dns := ""
	dnsMode := "physical_interface_dns_v1"
	if output, err := exec.CommandContext(ctx, "/usr/sbin/scutil", "--dns").Output(); err == nil {
		dns = physicalScopedDNS(string(output), ifaceName, iface.Index)
	}
	if dns == "" {
		dnsMode = "physical_dhcp_dns_v1"
		output, err := exec.CommandContext(ctx, "/usr/sbin/ipconfig", "getoption", ifaceName, "domain_name_server").Output()
		if err != nil {
			return nil, nil, fmt.Errorf("physical interface DNS unavailable")
		}
		for _, field := range strings.Fields(string(output)) {
			if ip, err := netip.ParseAddr(field); err == nil && ip.Is4() && usablePhysicalAddress(ip) {
				dns = ip.String()
				break
			}
		}
	}
	if dns == "" {
		return nil, nil, fmt.Errorf("physical DHCP DNS is virtual, local or unavailable")
	}
	e := &PhysicalDownloadEgress{iface: ifaceName, index: iface.Index, dns: dns, dnsMode: dnsMode}
	e.resolver = &net.Resolver{PreferGo: true, StrictErrors: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(dns, "53"), dialer.WithInterface(ifaceName), dialer.WithOnlySingleStack(true), dialer.WithFallbackBind(false))
		if err != nil {
			return nil, err
		}
		raw, ok := conn.(syscall.Conn)
		if !ok {
			conn.Close()
			return nil, fmt.Errorf("physical DNS socket cannot be verified")
		}
		if err = e.verify(raw, strings.HasPrefix(network, "udp"), true); err != nil {
			conn.Close()
			return nil, err
		}
		return conn, nil
	}}
	host, _ := original.Config["server"].(string)
	ip, err := e.resolve(ctx, host)
	if err != nil {
		reason := "unavailable"
		var dnsError *net.DNSError
		if errors.As(err, &dnsError) {
			// net.DNSError.Err excludes the queried hostname and proxy credentials.
			reason = dnsError.Err
			switch {
			case dnsError.IsNotFound:
				reason = "not_found"
			case dnsError.IsTimeout:
				reason = "timeout"
			case strings.Contains(dnsError.Err, "server misbehaving"):
				reason = "server_misbehaving"
			case strings.Contains(dnsError.Err, "connection refused"):
				reason = "connection_refused"
			case strings.Contains(dnsError.Err, "no route to host"):
				reason = "local_network_unreachable"
			case strings.Contains(dnsError.Err, "operation not permitted"):
				reason = "local_permission_denied"
			}
		}
		return nil, nil, fmt.Errorf("physical DNS resolution unavailable (reason=%s, class=%s, context=%v, bound=%t); download not executed", reason, downloadErrorClass(err), ctx.Err(), e.Snapshot().DNSBindVerified)
	}
	config, err := clonePhysicalProxyConfig(original.Config, ip, ifaceName)
	if err != nil {
		return nil, nil, err
	}
	e.host, e.nodeIP = host, ip
	proxy, err := adapter.ParseProxy(config, adapter.WithDialerForAPI(e))
	if err != nil {
		return nil, nil, fmt.Errorf("cannot create physical-path node proxy")
	}
	e.prepared = time.Since(started)
	return &CProxy{Proxy: proxy, Config: config}, e, nil
}
