//go:build !darwin

package speedtester

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
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

// ResolvePhysicalHost resolves a node server on platforms without the physical
// socket binding; the system resolver is used as-is.
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
	queryCtx, cancel := context.WithTimeout(ctx, timeout*2)
	defer cancel()
	addresses, err := net.DefaultResolver.LookupHost(queryCtx, host)
	if err != nil {
		return PhysicalHostResolve{}, err
	}
	seen := map[string]bool{}
	ordered := make([]string, 0, len(addresses))
	for _, address := range addresses {
		ip, parseErr := netip.ParseAddr(address)
		if parseErr != nil {
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
		return PhysicalHostResolve{}, fmt.Errorf("没有解析到 A/AAAA 记录")
	}
	return PhysicalHostResolve{Host: host, Source: "dns", Addresses: ordered}, nil
}

// NewPhysicalHTTPClient returns a plain HTTP client without system proxies.
func NewPhysicalHTTPClient(timeout time.Duration) (*http.Client, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &http.Client{Timeout: timeout}, nil
}
