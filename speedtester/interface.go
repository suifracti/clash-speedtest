package speedtester

import (
	"net"
	"strings"

	"github.com/metacubex/mihomo/component/dialer"
)

// AutoDetectPhysicalInterface finds the active physical network interface
// (e.g. WLAN, Ethernet, 以太网, en0, eth0) while excluding virtual adapters such as
// TUN/TAP, Clash/Mihomo, Sing-box, Docker, etc.
func AutoDetectPhysicalInterface() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	virtualKeywords := []string{
		"tun", "tap", "wintun", "mihomo", "clash", "sing-box", "sing_tun",
		"tailscale", "wireguard", "utun", "docker", "veth", "bridge", "vmnet", "vbox",
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if len(iface.HardwareAddr) == 0 {
			continue
		}
		lowerName := strings.ToLower(iface.Name)
		isVirtual := false
		for _, kw := range virtualKeywords {
			if strings.Contains(lowerName, kw) {
				isVirtual = true
				break
			}
		}
		if isVirtual {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ipv4 := ip.To4()
			if ipv4 == nil {
				continue
			}
			// Skip link-local 169.254.0.0/16
			if ipv4.IsLinkLocalUnicast() {
				continue
			}
			// Skip TUN fake-ip 198.18.0.0/15
			if ipv4[0] == 198 && (ipv4[1] == 18 || ipv4[1] == 19) {
				continue
			}
			return iface.Name
		}
	}
	return ""
}

// BindPhysicalInterface automatically detects and binds Mihomo's dialer
// to the physical network interface so that proxy probes bypass any active TUN adapter.
func BindPhysicalInterface() string {
	ifaceName := AutoDetectPhysicalInterface()
	if ifaceName != "" {
		dialer.DefaultInterface.Store(ifaceName)
	}
	return ifaceName
}
