package application

import (
	"github.com/faceair/clash-speedtest/core/speedtester"
	"testing"
)

func TestMonitorPhysicalPathLabelAndCleanup(t *testing.T) {
	closed := false
	transport := &physicalMonitorTransport{path: func() speedtester.DownloadNetworkPath {
		return speedtester.DownloadNetworkPath{Interface: "en1", SocketBindVerified: true, DNSBindVerified: true}
	}, cleanup: func() { closed = true }}
	if transport.TUNBypassInterface() != "en1" {
		t.Fatal("verified physical path not labelled")
	}
	transport.CloseIdleConnections()
	if !closed {
		t.Fatal("monitor client did not release its physical proxy")
	}
}
func TestMonitorDoesNotLabelUnverifiedDNSAsBypassed(t *testing.T) {
	transport := &physicalMonitorTransport{path: func() speedtester.DownloadNetworkPath {
		return speedtester.DownloadNetworkPath{Interface: "en1", SocketBindVerified: true}
	}}
	if transport.TUNBypassInterface() != "" {
		t.Fatal("unverified DNS labelled as bypassed")
	}
}
