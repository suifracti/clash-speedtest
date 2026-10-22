//go:build darwin

package speedtester

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os/exec"
	"strings"
	"time"

	"github.com/metacubex/mihomo/component/dialer"
	"github.com/miekg/dns"
)

// PhysicalHostResolve is one node server resolved through the physical
// interface. Addresses keep DNS order; the first one is the primary entry.
type PhysicalHostResolve struct {
	Host      string
	Source    string
	Interface string
	DNS       string
	Addresses []string
}

// ResolvePhysicalHost resolves A and AAAA records without the node and without
// the running TUN: the DNS socket is bound to the physical interface.
func ResolvePhysicalHost(ctx context.Context, host string, timeout time.Duration) (PhysicalHostResolve, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return PhysicalHostResolve{}, fmt.Errorf("节点没有 server 地址")
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return PhysicalHostResolve{Host: host, Source: "literal", Addresses: []string{ip.Unmap().String()}}, nil
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout*2)
	defer cancel()
	ifaceName := AutoDetectPhysicalInterface()
	if ifaceName == "" {
		return PhysicalHostResolve{}, fmt.Errorf("没有可用的物理网卡")
	}
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return PhysicalHostResolve{}, fmt.Errorf("读取物理网卡 %s 失败: %w", ifaceName, err)
	}
	dnsServer := physicalInterfaceDNS(ctx, ifaceName, iface.Index)
	if dnsServer == "" {
		return PhysicalHostResolve{}, fmt.Errorf("物理网卡 %s 没有可用的 DNS", ifaceName)
	}
	resolve := PhysicalHostResolve{Host: host, Source: "dns", Interface: ifaceName, DNS: dnsServer}
	seen := map[string]bool{}
	var lastErr error
	for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA} {
		answer, err := exchangePhysicalDNS(ctx, ifaceName, dnsServer, host, qtype, timeout)
		if err != nil {
			lastErr = err
			continue
		}
		for _, address := range answer {
			if seen[address] {
				continue
			}
			seen[address] = true
			resolve.Addresses = append(resolve.Addresses, address)
		}
	}
	if len(resolve.Addresses) == 0 {
		if lastErr == nil {
			lastErr = fmt.Errorf("没有解析到 A/AAAA 记录")
		}
		return PhysicalHostResolve{}, lastErr
	}
	return resolve, nil
}

func physicalInterfaceDNS(ctx context.Context, name string, index int) string {
	if output, err := exec.CommandContext(ctx, "/usr/sbin/scutil", "--dns").Output(); err == nil {
		if dns := physicalScopedDNS(string(output), name, index); dns != "" {
			return dns
		}
	}
	if output, err := exec.CommandContext(ctx, "/usr/sbin/ipconfig", "getoption", name, "domain_name_server").Output(); err == nil {
		for _, field := range strings.Fields(string(output)) {
			if ip, parseErr := netip.ParseAddr(field); parseErr == nil && ip.Is4() && usablePhysicalAddress(ip) {
				return ip.String()
			}
		}
	}
	return ""
}

func exchangePhysicalDNS(ctx context.Context, iface, server, host string, qtype uint16, timeout time.Duration) ([]string, error) {
	queryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := dialer.DialContext(queryCtx, "udp", net.JoinHostPort(server, "53"),
		dialer.WithInterface(iface), dialer.WithOnlySingleStack(true), dialer.WithFallbackBind(false))
	if err != nil {
		return nil, err
	}
	message := new(dns.Msg)
	message.SetQuestion(dns.Fqdn(host), qtype)
	message.SetEdns0(1232, false)
	client := &dns.Client{Net: "udp", Timeout: timeout, UDPSize: 1232}
	response, _, err := client.ExchangeWithConnContext(queryCtx, message, &dns.Conn{Conn: conn, UDPSize: 1232})
	conn.Close()
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, fmt.Errorf("DNS 没有响应")
	}
	if response.Rcode != dns.RcodeSuccess {
		return nil, fmt.Errorf("DNS 返回 %s", dns.RcodeToString[response.Rcode])
	}
	addresses := make([]string, 0, len(response.Answer))
	for _, record := range response.Answer {
		switch typed := record.(type) {
		case *dns.A:
			if qtype == dns.TypeA {
				if ip, ok := netip.AddrFromSlice(typed.A); ok {
					addresses = append(addresses, ip.Unmap().String())
				}
			}
		case *dns.AAAA:
			if qtype == dns.TypeAAAA {
				if ip, ok := netip.AddrFromSlice(typed.AAAA); ok {
					addresses = append(addresses, ip.String())
				}
			}
		}
	}
	return addresses, nil
}

// NewPhysicalHTTPClient returns a plain HTTP client bound to the physical
// interface. It ignores system proxies and system DNS: names are resolved
// through the physical interface too, so a TUN's fake-IP range never gets used.
func NewPhysicalHTTPClient(timeout time.Duration) (*http.Client, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	iface := AutoDetectPhysicalInterface()
	if iface == "" {
		return nil, fmt.Errorf("没有可用的物理网卡")
	}
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		dialOne := func(target string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, net.JoinHostPort(target, port),
				dialer.WithInterface(iface), dialer.WithFallbackBind(false))
		}
		if ip, parseErr := netip.ParseAddr(host); parseErr == nil {
			return dialOne(ip.Unmap().String())
		}
		resolved, resolveErr := ResolvePhysicalHost(ctx, host, timeout)
		if resolveErr != nil {
			return nil, resolveErr
		}
		var lastErr error
		for _, ip := range resolved.Addresses {
			conn, dialErr := dialOne(ip)
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("物理网卡没有可用地址")
		}
		return nil, lastErr
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           dial,
		MaxIdleConns:          4,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
	}
	return &http.Client{Transport: transport, Timeout: timeout}, nil
}
