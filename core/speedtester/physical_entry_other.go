//go:build !darwin && !windows

package speedtester

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/metacubex/mihomo/component/dialer"
	"github.com/metacubex/mihomo/component/resolver"
)

var (
	fakeIPv4Prefix = netip.MustParsePrefix("198.18.0.0/15")
	fakeIPv6Prefix = netip.MustParsePrefix("fdfe:dcba:9876::/64")
)

func isFakeIPAddress(ip netip.Addr) bool {
	if !ip.IsValid() {
		return false
	}
	unmapped := ip.Unmap()
	return fakeIPv4Prefix.Contains(unmapped) || fakeIPv6Prefix.Contains(ip)
}

func filterPhysicalResolvedAddresses(addresses []string) ([]string, error) {
	seen := map[string]bool{}
	ordered := make([]string, 0, len(addresses))
	sawFakeIP := false
	for _, address := range addresses {
		ip, parseErr := netip.ParseAddr(strings.TrimSpace(address))
		if parseErr != nil {
			continue
		}
		if isFakeIPAddress(ip) {
			sawFakeIP = true
			continue
		}
		value := ip.Unmap().String()
		if seen[value] {
			continue
		}
		seen[value] = true
		ordered = append(ordered, value)
	}
	if len(ordered) == 0 {
		if sawFakeIP {
			return nil, fmt.Errorf("解析结果为 Fake-IP，未取得真实入口地址")
		}
		return nil, fmt.Errorf("没有解析到 A/AAAA 记录")
	}
	return ordered, nil
}

// PhysicalHostResolve is one node server resolved through the physical
// interface. Addresses keep DNS order; the first one is the primary entry.
type PhysicalHostResolve struct {
	Host      string
	Source    string
	Interface string
	DNS       string
	Addresses []string
}

// ResolvePhysicalHost resolves a node server on non-Darwin platforms, binding
// to the detected physical interface when available and rejecting Fake-IP answers.
func ResolvePhysicalHost(ctx context.Context, host string, timeout time.Duration) (PhysicalHostResolve, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return PhysicalHostResolve{}, fmt.Errorf("节点没有 server 地址")
	}
	ifaceName := AutoDetectPhysicalInterface()
	if ip, err := netip.ParseAddr(host); err == nil {
		if isFakeIPAddress(ip) {
			return PhysicalHostResolve{}, fmt.Errorf("节点地址为 Fake-IP，未取得真实入口地址")
		}
		return PhysicalHostResolve{Host: host, Source: "literal", Interface: ifaceName, Addresses: []string{ip.Unmap().String()}}, nil
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	queryCtx, cancel := context.WithTimeout(ctx, timeout*2)
	defer cancel()
	resolverClient := net.DefaultResolver
	if ifaceName != "" {
		resolverClient = &net.Resolver{
			PreferGo: true,
			Dial: func(dialCtx context.Context, network, address string) (net.Conn, error) {
				if dnsHost, _, splitErr := net.SplitHostPort(address); splitErr == nil {
					if dnsIP, parseErr := netip.ParseAddr(strings.TrimSpace(dnsHost)); parseErr == nil && isFakeIPAddress(dnsIP) {
						return nil, fmt.Errorf("skip fake-ip dns server %s on physical interface", address)
					}
				}
				return dialer.DialContext(dialCtx, network, address,
					dialer.WithInterface(ifaceName), dialer.WithFallbackBind(true))
			},
		}
	}
	addresses, err := resolverClient.LookupHost(queryCtx, host)
	if err != nil && ifaceName != "" {
		if ips, fallbackErr := resolver.LookupIPv4(queryCtx, host); fallbackErr == nil && len(ips) > 0 {
			addresses = make([]string, 0, len(ips))
			for _, ip := range ips {
				addresses = append(addresses, ip.String())
			}
			err = nil
		}
	}
	if err != nil {
		return PhysicalHostResolve{}, err
	}
	ordered, filterErr := filterPhysicalResolvedAddresses(addresses)
	if filterErr != nil {
		return PhysicalHostResolve{}, filterErr
	}
	return PhysicalHostResolve{Host: host, Source: "dns", Interface: ifaceName, Addresses: ordered}, nil
}

func isLoopbackDialTarget(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

// NewPhysicalHTTPClient returns a plain HTTP client without system proxies,
// binding non-loopback connections to the physical interface when available.
func NewPhysicalHTTPClient(timeout time.Duration) (*http.Client, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	resolver.AddSystemDnsBlacklist("198.18.0.1", "198.18.0.2", "fdfe:dcba:9876::1", "fdfe:dcba:9876::2")
	iface := AutoDetectPhysicalInterface()
	baseDialer := &net.Dialer{Timeout: timeout}
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if iface != "" && !isLoopbackDialTarget(host) {
			return dialer.DialContext(ctx, network, address,
				dialer.WithInterface(iface), dialer.WithFallbackBind(true))
		}
		return baseDialer.DialContext(ctx, network, address)
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
