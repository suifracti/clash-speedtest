package publicservice

import (
	"context"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/speedtester"
	"net/http"
	"runtime"
	"time"
)

func defaultClientFactory(ctx context.Context, node monitor.MonitoredNode, timeout time.Duration) (*http.Client, func() speedtester.DownloadNetworkPath, func(), error) {
	if runtime.GOOS != "darwin" {
		client, err := monitor.NewDefaultNodeDialer().CreateClient(node, timeout)
		return client, nil, func() {
			if client != nil {
				client.CloseIdleConnections()
			}
		}, err
	}
	// The frozen raw config supplies the node, not the current VPN selection.
	return speedtester.OpenPhysicalProbeClient(ctx, &speedtester.CProxy{Config: node.RawConfig}, timeout, 4)
}
