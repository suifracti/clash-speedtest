package application

import (
	"context"
	"net/http"
	"runtime"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/speedtester"
)

// The app monitor uses the same physical DNS/socket path as workbench probes.
type physicalMonitorDialer struct{}

func (physicalMonitorDialer) CreateClient(node monitor.MonitoredNode, timeout time.Duration) (*http.Client, error) {
	if runtime.GOOS != "darwin" || monitor.HasSourceTLSConstraints(node.RawConfig) {
		return monitor.NewDefaultNodeDialer().CreateClient(node, timeout)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	client, path, cleanup, err := speedtester.OpenPhysicalProbeClient(ctx, &speedtester.CProxy{Config: node.RawConfig}, timeout, len(monitor.GetProbeTargets(monitor.ProbeSetLatencySix)))
	if err != nil {
		return nil, err
	}
	// Leave the original client unchanged: cleanup owns it and closes its transport.
	copyClient := *client
	copyClient.Transport = &physicalMonitorTransport{RoundTripper: client.Transport, path: path, cleanup: cleanup}
	return &copyClient, nil
}

type physicalMonitorTransport struct {
	http.RoundTripper
	path    func() speedtester.DownloadNetworkPath
	cleanup func()
}

func (t *physicalMonitorTransport) CloseIdleConnections() { t.cleanup() }
func (t *physicalMonitorTransport) TUNBypassInterface() string {
	p := t.path()
	if p.SocketBindVerified && (p.DNSBindVerified || p.DNSMode == "node_literal_no_dns") {
		return p.Interface
	}
	return ""
}
