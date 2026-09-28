package speedtester

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// A connected USB device can have a valid address without an Internet route.
// Only adapters with an IPv4 gateway are eligible for automatic probe binding.
func physicalGatewayMetrics() map[int]uint32 {
	size := uint32(15 * 1024)
	for attempt := 0; attempt < 3; attempt++ {
		buf := make([]byte, size)
		first := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		err := windows.GetAdaptersAddresses(windows.AF_INET, windows.GAA_FLAG_INCLUDE_GATEWAYS, 0, first, &size)
		if err == windows.ERROR_BUFFER_OVERFLOW {
			continue
		}
		if err != nil {
			return map[int]uint32{}
		}
		metrics := make(map[int]uint32)
		for adapter := first; adapter != nil; adapter = adapter.Next {
			if adapter.OperStatus == windows.IfOperStatusUp && adapter.FirstGatewayAddress != nil {
				metrics[int(adapter.IfIndex)] = adapter.Ipv4Metric
			}
		}
		return metrics
	}
	return map[int]uint32{}
}
