//go:build darwin

package speedtester

import (
	"golang.org/x/net/route"
	"syscall"
)

// Prefer physical interfaces with a real IPv4 default gateway, not the first
// UP Ethernet/Wi-Fi interface. VPN split routes do not replace this selection.
func physicalGatewayMetrics() map[int]uint32 {
	metrics := map[int]uint32{}
	rib, err := route.FetchRIB(syscall.AF_INET, syscall.NET_RT_DUMP, 0)
	if err != nil {
		return metrics
	}
	messages, err := route.ParseRIB(syscall.NET_RT_DUMP, rib)
	if err != nil {
		return metrics
	}
	for _, m := range messages {
		r, ok := m.(*route.RouteMessage)
		if !ok || r.Flags&syscall.RTF_UP == 0 || r.Flags&syscall.RTF_GATEWAY == 0 || len(r.Addrs) <= syscall.RTAX_GATEWAY {
			continue
		}
		dst, ok := r.Addrs[syscall.RTAX_DST].(*route.Inet4Addr)
		if !ok || dst.IP != [4]byte{} {
			continue
		}
		gateway, ok := r.Addrs[syscall.RTAX_GATEWAY].(*route.Inet4Addr)
		if !ok || gateway.IP == [4]byte{} {
			continue
		}
		metric := uint32(0)
		if r.Flags&syscall.RTF_IFSCOPE != 0 {
			metric = 1
		}
		if old, ok := metrics[r.Index]; !ok || metric < old {
			metrics[r.Index] = metric
		}
	}
	return metrics
}
