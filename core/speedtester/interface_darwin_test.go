//go:build darwin

package speedtester

import (
	"reflect"
	"syscall"
	"testing"

	"golang.org/x/net/route"
)

func TestPhysicalGatewayIgnoresZeroDestinationSplitRoutes(t *testing.T) {
	makeRoute := func(index, flags int, mask route.Addr) *route.RouteMessage {
		return &route.RouteMessage{Index: index, Flags: flags, Addrs: []route.Addr{
			syscall.RTAX_DST:     &route.Inet4Addr{},
			syscall.RTAX_GATEWAY: &route.Inet4Addr{IP: [4]byte{192, 0, 2, 1}},
			syscall.RTAX_NETMASK: mask,
		}}
	}
	upGateway := syscall.RTF_UP | syscall.RTF_GATEWAY
	zero := &route.Inet4Addr{}
	for _, tc := range []struct {
		name   string
		routes []route.Message
		want   map[int]uint32
	}{
		{"ordinary default", []route.Message{makeRoute(15, upGateway, zero)}, map[int]uint32{15: 0}},
		{"split half is not default", []route.Message{makeRoute(12, upGateway, &route.Inet4Addr{IP: [4]byte{128, 0, 0, 0}}), makeRoute(15, upGateway, zero)}, map[int]uint32{15: 0}},
		{"zero subnet is not default", []route.Message{makeRoute(12, upGateway, &route.Inet4Addr{IP: [4]byte{255, 0, 0, 0}})}, map[int]uint32{}},
		{"zero host is not default", []route.Message{makeRoute(12, upGateway|syscall.RTF_HOST, nil)}, map[int]uint32{}},
		{"scoped default keeps lower priority", []route.Message{makeRoute(15, upGateway|syscall.RTF_IFSCOPE, zero), makeRoute(15, upGateway, zero), makeRoute(16, upGateway|syscall.RTF_IFSCOPE, zero)}, map[int]uint32{15: 0, 16: 1}},
		{"absent mask retains non host compatibility", []route.Message{makeRoute(15, upGateway, nil)}, map[int]uint32{15: 0}},
		{"down route is not candidate", []route.Message{makeRoute(15, syscall.RTF_GATEWAY, zero)}, map[int]uint32{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := physicalRouteGatewayMetrics(tc.routes); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("physical gateway candidates = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPhysicalGatewayNilRouteMessages(t *testing.T) {
	if got := physicalRouteGatewayMetrics(nil); len(got) != 0 {
		t.Fatalf("expected empty map for nil messages, got %v", got)
	}
	messages := []route.Message{nil, (*route.RouteMessage)(nil)}
	if got := physicalRouteGatewayMetrics(messages); len(got) != 0 {
		t.Fatalf("expected empty map for nil messages list, got %v", got)
	}
}

