package publicservice

import (
	"context"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/speedtester"
	"net/http"
	"runtime"
	"time"
)

func defaultClientFactory(ctx context.Context, node monitor.MonitoredNode, timeout time.Duration, autoFamily bool, socketLimit int) (*http.Client, func() speedtester.DownloadNetworkPath, func(), error) {
	var client *http.Client
	var snapshot func() speedtester.DownloadNetworkPath
	var cleanup func()
	var err error
	if runtime.GOOS != "darwin" {
		client, err = monitor.NewDefaultNodeDialer().CreateClient(node, timeout)
		cleanup = func() {
			if client != nil {
				client.CloseIdleConnections()
			}
		}
	} else if autoFamily {
		// The frozen raw config supplies the node, not the current VPN selection.
		client, snapshot, cleanup, err = speedtester.OpenPhysicalAutoProbeClient(ctx, &speedtester.CProxy{Config: node.RawConfig}, timeout, socketLimit)
	} else {
		client, snapshot, cleanup, err = speedtester.OpenPhysicalProbeClient(ctx, &speedtester.CProxy{Config: node.RawConfig}, timeout, socketLimit)
	}
	if err == nil {
		enableServiceHTTP2(client)
	}
	return client, snapshot, cleanup, err
}

// These clients and transports are freshly owned by the service check. Custom
// node DialContext disables Go's automatic HTTP/2 setup unless explicitly enabled.
// ALPN still falls back to HTTP/1.1; routing, TLS verification and budgets stay put.
func enableServiceHTTP2(client *http.Client) {
	if client == nil {
		return
	}
	if tr, ok := client.Transport.(*http.Transport); ok && tr.TLSNextProto == nil && tr.DialTLS == nil && tr.DialTLSContext == nil {
		tr.ForceAttemptHTTP2 = true
	}
}
