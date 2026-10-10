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
	iface            string
	index            int
	dns              string
	dnsMode          string
	host             string
	nodeIP           netip.Addr
	resolver         *net.Resolver
	dnsRequests      *physicalRequestBudget
	dnsDials         *physicalRequestBudget
	tcpDials         *physicalRequestBudget
	udpDials         *physicalRequestBudget
	mu               sync.Mutex
	tcp, udp         int
	dnsVerified      bool
	addressFamily    string
	resolutionSource string
	failureReason    string
	prepared         time.Duration
	dnsDuration      time.Duration
}

func (e *PhysicalDownloadEgress) Snapshot() DownloadNetworkPath {
	e.mu.Lock()
	defer e.mu.Unlock()
	return DownloadNetworkPath{
		Method: "physical_socket_v1", Interface: e.iface, AddressFamily: e.addressFamily,
		ResolutionSource: e.resolutionSource, DNSMode: e.dnsMode,
		TUNEvidence: "packet_route_not_observed", FailureReason: e.failureReason,
		DNSRequests: e.dnsRequests.count(), DNSDialAttempts: e.dnsDials.count(),
		TCPDialAttempts: e.tcpDials.count(), UDPDialAttempts: e.udpDials.count(),
		DNSBindVerified: e.dnsVerified, SocketBindVerified: e.tcp+e.udp > 0,
		TCPBindings: e.tcp, UDPBindings: e.udp, PreparationDurationNS: e.prepared.Nanoseconds(), DNSDurationNS: e.dnsDuration.Nanoseconds(),
	}
}

type physicalDNSConn struct {
	net.Conn
	ctx    context.Context
	budget *physicalRequestBudget
}

func (c *physicalDNSConn) Write(p []byte) (int, error) {
	if err := c.budget.reserve(c.ctx); err != nil {
		return 0, err
	}
	return c.Conn.Write(p)
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
		if !ip.Is4() || !usablePhysicalAddress(ip) {
			e.setFailure("node_address_unusable_or_unverified_family")
			return netip.Addr{}, fmt.Errorf("proxy server is local, virtual, fake-ip or outside the verified IPv4 path")
		}
		e.setResolution("ipv4", "literal_ip")
		return ip, nil
	}
	if err := ctx.Err(); err != nil {
		e.setFailure(physicalFailureReason(ctx, err))
		return netip.Addr{}, err
	}
	markDownloadPhase(ctx, "dns")
	started := time.Now()
	lookupCtx, cancel := boundedPhysicalContext(ctx, physicalDownloadLookupTimeout)
	ips, err := e.resolver.LookupNetIP(lookupCtx, "ip4", host)
	cancel()
	e.mu.Lock()
	e.dnsDuration += time.Since(started)
	e.mu.Unlock()
	if err != nil {
		e.setFailure(physicalFailureReason(ctx, err))
		return netip.Addr{}, fmt.Errorf("physical DNS lookup unavailable: %w", err)
	}
	for _, ip := range ips {
		if usablePhysicalAddress(ip) {
			ip = ip.Unmap()
			e.setResolution("ipv4", "physical_ipv4_dns")
			return ip, nil
		}
	}
	e.setFailure("physical_dns_no_usable_ipv4")
	return netip.Addr{}, fmt.Errorf("physical DNS returned no usable non-fake address")
}

func (e *PhysicalDownloadEgress) setResolution(family, source string) {
	e.mu.Lock()
	e.addressFamily, e.resolutionSource = family, source
	e.mu.Unlock()
}

func (e *PhysicalDownloadEgress) setFailure(reason string) {
	e.mu.Lock()
	e.failureReason = reason
	e.mu.Unlock()
}

func (e *PhysicalDownloadEgress) reserveDialBudget(ctx context.Context, udp bool) error {
	e.mu.Lock()
	budget := e.tcpDials
	if udp {
		budget = e.udpDials
	}
	if budget == nil {
		budget = newPhysicalRequestBudget(physicalDownloadSocketRequestLimit)
		if udp {
			e.udpDials = budget
		} else {
			e.tcpDials = budget
		}
	}
	e.mu.Unlock()
	return budget.reserve(ctx)
}

func physicalFailureReason(ctx context.Context, err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return "user_cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "deadline_exceeded"
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		switch {
		case dnsErr.IsNotFound:
			return "ipv4_dns_not_found"
		case dnsErr.IsTimeout:
			return "ipv4_dns_timeout"
		}
	}
	if errors.Is(err, errPhysicalDownloadBudgetExceeded) {
		return "physical_request_budget_exhausted"
	}
	if strings.Contains(err.Error(), "connection refused") {
		return "physical_dns_connection_refused"
	}
	if strings.Contains(err.Error(), "no route to host") {
		return "physical_dns_network_unreachable"
	}
	return "physical_dns_unavailable"
}

func physicalSocketFailureReason(ctx context.Context, err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return "user_cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "deadline_exceeded"
	}
	if errors.Is(err, errPhysicalDownloadBudgetExceeded) {
		return "physical_request_budget_exhausted"
	}
	return "physical_socket_connect_failed"
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
	if err := e.reserveDialBudget(ctx, false); err != nil {
		e.setFailure(physicalFailureReason(ctx, err))
		return nil, err
	}
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
		e.setFailure(physicalSocketFailureReason(ctx, err))
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
		e.setFailure("physical_tcp_socket_bind_unverified")
		return nil, &PhysicalPathError{Cause: err}
	}
	return conn, nil
}
func (e *PhysicalDownloadEgress) ListenPacket(ctx context.Context, network, address string, remote netip.AddrPort) (net.PacketConn, error) {
	if remote.IsValid() && !remote.Addr().Is4() {
		e.setFailure("unverified_ipv6_socket_family")
		return nil, &PhysicalPathError{Cause: fmt.Errorf("IPv6 packet sockets are outside the verified Darwin AF_INET path")}
	}
	if err := e.reserveDialBudget(ctx, true); err != nil {
		e.setFailure(physicalFailureReason(ctx, err))
		return nil, err
	}
	conn, err := dialer.ListenPacket(ctx, network, address, remote, dialer.WithInterface(e.iface), dialer.WithFallbackBind(false))
	if err != nil {
		e.setFailure(physicalSocketFailureReason(ctx, err))
		return nil, err
	}
	raw, ok := conn.(syscall.Conn)
	if !ok {
		conn.Close()
		return nil, &PhysicalPathError{Cause: fmt.Errorf("physical UDP socket cannot be verified")}
	}
	if err = e.verify(raw, true, false); err != nil {
		conn.Close()
		e.setFailure("physical_udp_socket_bind_unverified")
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
func PreparePhysicalDownloadProxy(ctx context.Context, original *CProxy) (prepared *CProxy, e *PhysicalDownloadEgress, returnErr error) {
	started := time.Now()
	e = &PhysicalDownloadEgress{
		dnsRequests:   newPhysicalRequestBudget(physicalDownloadDNSRequestLimit),
		dnsDials:      newPhysicalRequestBudget(physicalDownloadDNSRequestLimit),
		tcpDials:      newPhysicalRequestBudget(physicalDownloadSocketRequestLimit),
		udpDials:      newPhysicalRequestBudget(physicalDownloadSocketRequestLimit),
		addressFamily: "unknown", resolutionSource: "unobserved",
	}
	defer func() {
		e.mu.Lock()
		e.prepared = time.Since(started)
		if returnErr != nil && e.failureReason == "" {
			e.failureReason = physicalFailureReason(ctx, returnErr)
		}
		e.mu.Unlock()
	}()
	ctx, cancel := boundedPhysicalContext(ctx, physicalDownloadPreparationTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		e.setFailure(physicalFailureReason(ctx, err))
		return nil, e, err
	}
	if original == nil || len(original.Config) == 0 {
		e.setFailure("download_proxy_config_unavailable")
		return nil, e, fmt.Errorf("download proxy config unavailable")
	}
	if dialer.DefaultSocketHook != nil {
		e.setFailure("external_socket_hook")
		return nil, e, fmt.Errorf("external socket hook prevents physical binding verification")
	}
	ifaceName := AutoDetectPhysicalInterface()
	if ifaceName == "" {
		e.setFailure("physical_interface_unavailable")
		return nil, e, fmt.Errorf("no physical default gateway interface")
	}
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		e.setFailure("physical_interface_unavailable")
		return nil, e, fmt.Errorf("physical interface unavailable")
	}
	e.iface, e.index = ifaceName, iface.Index
	host, _ := original.Config["server"].(string)
	e.host = host
	if host == "" {
		e.setFailure("proxy_server_unavailable")
		return nil, e, fmt.Errorf("proxy server address unavailable")
	}
	literal, literalErr := netip.ParseAddr(host)
	if literalErr == nil && (!literal.Is4() || !usablePhysicalAddress(literal)) {
		e.setFailure("node_address_unusable_or_unverified_family")
		return nil, e, fmt.Errorf("proxy server is local, virtual, fake-ip or outside the verified IPv4 path")
	}
	dns := ""
	if literalErr == nil {
		e.dnsMode = "node_literal_no_dns"
	} else {
		e.dnsMode = "physical_interface_dns_v1"
		if output, commandErr := exec.CommandContext(ctx, "/usr/sbin/scutil", "--dns").Output(); commandErr == nil {
			dns = physicalScopedDNS(string(output), ifaceName, iface.Index)
		}
		if dns == "" {
			e.dnsMode = "physical_dhcp_dns_v1"
			output, commandErr := exec.CommandContext(ctx, "/usr/sbin/ipconfig", "getoption", ifaceName, "domain_name_server").Output()
			if commandErr != nil {
				e.setFailure(physicalFailureReason(ctx, commandErr))
				return nil, e, fmt.Errorf("physical interface DNS unavailable")
			}
			for _, field := range strings.Fields(string(output)) {
				if ip, parseErr := netip.ParseAddr(field); parseErr == nil && ip.Is4() && usablePhysicalAddress(ip) {
					dns = ip.String()
					break
				}
			}
		}
		if dns == "" {
			e.setFailure("physical_dns_unavailable")
			return nil, e, fmt.Errorf("physical interface DNS is virtual, local or unavailable")
		}
	}
	e.dns = dns
	if dns != "" {
		e.resolver = &net.Resolver{PreferGo: true, StrictErrors: true, Dial: func(dialCtx context.Context, network, _ string) (net.Conn, error) {
			if err := e.dnsDials.reserve(dialCtx); err != nil {
				e.setFailure(physicalFailureReason(dialCtx, err))
				return nil, err
			}
			conn, dialErr := dialer.DialContext(dialCtx, network, net.JoinHostPort(dns, "53"), dialer.WithInterface(ifaceName), dialer.WithOnlySingleStack(true), dialer.WithFallbackBind(false))
			if dialErr != nil {
				e.setFailure(physicalFailureReason(dialCtx, dialErr))
				return nil, dialErr
			}
			raw, ok := conn.(syscall.Conn)
			if !ok {
				_ = conn.Close()
				e.setFailure("dns_socket_unverifiable")
				return nil, fmt.Errorf("physical DNS socket cannot be verified")
			}
			if verifyErr := e.verify(raw, strings.HasPrefix(network, "udp"), true); verifyErr != nil {
				_ = conn.Close()
				e.setFailure("dns_socket_bind_failed")
				return nil, verifyErr
			}
			return &physicalDNSConn{Conn: conn, ctx: dialCtx, budget: e.dnsRequests}, nil
		}}
	}
	ip, err := e.resolve(ctx, host)
	if err != nil {
		if e.Snapshot().FailureReason == "" {
			e.setFailure(physicalFailureReason(ctx, err))
		}
		return nil, e, fmt.Errorf("physical IPv4 DNS or address selection unavailable; download not executed")
	}
	config, err := clonePhysicalProxyConfig(original.Config, ip, ifaceName)
	if err != nil {
		e.setFailure("proxy_configuration_rejected")
		return nil, e, err
	}
	e.mu.Lock()
	e.host, e.nodeIP = host, ip
	e.mu.Unlock()
	proxy, err := adapter.ParseProxy(config, adapter.WithDialerForAPI(e))
	if err != nil {
		e.setFailure("proxy_adapter_unavailable")
		return nil, e, fmt.Errorf("cannot create physical-path node proxy")
	}
	return &CProxy{Proxy: proxy, Config: config}, e, nil
}
