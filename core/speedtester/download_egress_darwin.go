//go:build darwin

package speedtester

import (
	"context"
	"fmt"
	"net/netip"
	"os/exec"
	"strings"

	"golang.org/x/sys/unix"
)

func detectPhysicalInterfaceDNS(ctx context.Context, ifaceName string, ifaceIndex int) (string, string, error) {
	if output, commandErr := exec.CommandContext(ctx, "/usr/sbin/scutil", "--dns").Output(); commandErr == nil {
		if dns := physicalScopedDNS(string(output), ifaceName, ifaceIndex); dns != "" {
			return dns, "physical_interface_dns_v1", nil
		}
	}
	output, commandErr := exec.CommandContext(ctx, "/usr/sbin/ipconfig", "getoption", ifaceName, "domain_name_server").Output()
	if commandErr != nil {
		return "", "physical_dhcp_dns_v1", commandErr
	}
	for _, field := range strings.Fields(string(output)) {
		if ip, parseErr := netip.ParseAddr(field); parseErr == nil && ip.Is4() && usablePhysicalAddress(ip) {
			return ip.String(), "physical_dhcp_dns_v1", nil
		}
	}
	return "", "physical_dhcp_dns_v1", nil
}

func verifyPhysicalSocketHandle(fd uintptr, expectedIndex int) error {
	// UDP listeners may use an IPv6 dual-stack socket for an IPv4 peer.
	// Verify its actual family, not the remote address family.
	addr, err := unix.Getsockname(int(fd))
	if err != nil {
		return err
	}
	var index int
	switch addr.(type) {
	case *unix.SockaddrInet6:
		index, err = unix.GetsockoptInt(int(fd), unix.IPPROTO_IPV6, unix.IPV6_BOUND_IF)
	case *unix.SockaddrInet4:
		index, err = unix.GetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_BOUND_IF)
	default:
		return fmt.Errorf("physical socket family cannot be verified")
	}
	if err != nil {
		return err
	}
	if index != expectedIndex {
		return fmt.Errorf("physical socket interface verification failed")
	}
	return nil
}
