//go:build windows

package speedtester

import (
	"context"
	"fmt"
	"net/netip"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	windowsIPUnicastIF   = 31
	windowsIPv6UnicastIF = 31
)

func pickPhysicalAdapterDNS(addrs []netip.Addr) string {
	for _, raw := range addrs {
		ip := raw.Unmap()
		if ip.Is4() && usablePhysicalAddress(ip) {
			return ip.String()
		}
	}
	return ""
}

func detectPhysicalInterfaceDNS(ctx context.Context, _ string, ifaceIndex int) (string, string, error) {
	if err := ctx.Err(); err != nil {
		return "", "windows_adapter_dns_v1", err
	}
	size := uint32(15 * 1024)
	for attempt := 0; attempt < 3; attempt++ {
		buf := make([]byte, size)
		first := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, windows.GAA_FLAG_INCLUDE_GATEWAYS, 0, first, &size)
		if err == windows.ERROR_BUFFER_OVERFLOW {
			continue
		}
		if err != nil {
			return "", "windows_adapter_dns_v1", err
		}
		for adapter := first; adapter != nil; adapter = adapter.Next {
			if adapter.OperStatus != windows.IfOperStatusUp {
				continue
			}
			if int(adapter.IfIndex) != ifaceIndex && int(adapter.Ipv6IfIndex) != ifaceIndex {
				continue
			}
			var candidates []netip.Addr
			for dnsEntry := adapter.FirstDnsServerAddress; dnsEntry != nil; dnsEntry = dnsEntry.Next {
				if ip, ok := netip.AddrFromSlice(dnsEntry.Address.IP()); ok {
					candidates = append(candidates, ip)
				}
			}
			return pickPhysicalAdapterDNS(candidates), "windows_adapter_dns_v1", nil
		}
		return "", "windows_adapter_dns_v1", nil
	}
	return "", "windows_adapter_dns_v1", fmt.Errorf("windows adapter DNS enumeration exceeded buffer limit")
}

// Winsock returns IP_UNICAST_IF and IPV6_UNICAST_IF in host byte order even
// though IP_UNICAST_IF is set in network byte order.
func verifyPhysicalSocketHandle(fd uintptr, expectedIndex int) error {
	if expectedIndex <= 0 {
		return fmt.Errorf("physical socket interface index unavailable")
	}
	handle := windows.Handle(fd)
	addr, err := windows.Getsockname(handle)
	if err != nil {
		return err
	}
	switch sa := addr.(type) {
	case *windows.SockaddrInet4:
		got, optErr := windows.GetsockoptInt(handle, windows.IPPROTO_IP, windowsIPUnicastIF)
		if optErr != nil {
			return optErr
		}
		if got != expectedIndex {
			return fmt.Errorf("physical socket interface verification failed")
		}
		return nil
	case *windows.SockaddrInet6:
		got6, optErr6 := windows.GetsockoptInt(handle, windows.IPPROTO_IPV6, windowsIPv6UnicastIF)
		if optErr6 == nil && got6 == expectedIndex {
			return nil
		}
		// Wildcard dual-stack UDP listeners can bind only IP_UNICAST_IF when IPv6
		// is disabled on the adapter and the destination is IPv4.
		if sa.Addr == ([16]byte{}) {
			got4, optErr4 := windows.GetsockoptInt(handle, windows.IPPROTO_IP, windowsIPUnicastIF)
			if optErr4 == nil && got4 == expectedIndex {
				return nil
			}
		}
		if optErr6 != nil {
			return optErr6
		}
		return fmt.Errorf("physical socket interface verification failed")
	default:
		return fmt.Errorf("physical socket family cannot be verified")
	}
}
