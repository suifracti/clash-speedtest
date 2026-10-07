//go:build darwin

package speedtester

import (
	"context"
	"github.com/metacubex/mihomo/component/dialer"
	"net"
	"os"
	"syscall"
	"testing"
	"time"
)

// Opt-in Mac check: opens a bound UDP socket to the DHCP DNS address, but
// sends no DNS packet and performs no node download.
func TestPhysicalDNSSocketDiagnostic(t *testing.T) {
	if os.Getenv("SPEEDTEST_PHYSICAL_SOCKET_CHECK") != "1" {
		t.Skip("opt-in local socket check")
	}
	ifaceName := AutoDetectPhysicalInterface()
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	c, err := dialer.DialContext(ctx, "udp", "192.168.1.1:53", dialer.WithInterface(ifaceName), dialer.WithOnlySingleStack(true), dialer.WithFallbackBind(false))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	t.Logf("interface=%s index=%d conn=%T", ifaceName, iface.Index, c)
	raw, ok := c.(syscall.Conn)
	if !ok {
		t.Fatal("no raw socket")
	}
	e := &PhysicalDownloadEgress{iface: ifaceName, index: iface.Index}
	if err := e.verify(raw, true, true); err != nil {
		t.Fatal(err)
	}
	t.Logf("snapshot=%+v", e.Snapshot())
}
