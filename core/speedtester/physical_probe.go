package speedtester

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"sync"
	"time"
)

type physicalProbePreparer func(context.Context, *CProxy, int) (*CProxy, *PhysicalDownloadEgress, error)

// OpenPhysicalProbeClient reuses the download DNS/socket protections. The
// returned snapshot proves binding only, never independently observed routing.
func OpenPhysicalProbeClient(ctx context.Context, original *CProxy, timeout time.Duration, sockets int) (*http.Client, func() DownloadNetworkPath, func(), error) {
	return openPhysicalProbeClient(ctx, original, timeout, sockets, PreparePhysicalProbeProxy)
}

func openPhysicalProbeClient(ctx context.Context, original *CProxy, timeout time.Duration, sockets int, prepare physicalProbePreparer) (*http.Client, func() DownloadNetworkPath, func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, func() {}, err
	}
	proxy, path, err := prepare(ctx, original, sockets)
	var snapshot func() DownloadNetworkPath
	if path != nil {
		snapshot = path.Snapshot
	}
	var client *http.Client
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			if client != nil {
				client.CloseIdleConnections()
			}
			if proxy != nil && proxy.Proxy != nil {
				_ = proxy.Close()
			}
		})
	}
	if err != nil {
		cleanup()
		return nil, snapshot, cleanup, err
	}
	if proxy == nil || proxy.Proxy == nil {
		cleanup()
		return nil, snapshot, cleanup, &PhysicalPathError{Cause: fmt.Errorf("physical proxy unavailable")}
	}
	client = (&SpeedTester{}).createClient(proxy, timeout)
	transport := client.Transport.(*http.Transport)
	dial := transport.DialContext
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := dial(ctx, network, address)
		if err != nil {
			return nil, err
		}
		// An adapter that ignores the private API dialer must not reach HTTP.
		// QUIC/multiplexed streams may reuse this proxy's already verified socket.
		if conn == nil || snapshot == nil || !snapshot().SocketBindVerified {
			if conn != nil {
				_ = conn.Close()
			}
			return nil, &PhysicalPathError{Cause: fmt.Errorf("selected proxy has no verified bound socket")}
		}
		return conn, nil
	}
	// A fresh request connection prevents net/http's stale keep-alive replay.
	transport.DisableKeepAlives = true
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return client, snapshot, cleanup, nil
}

// This entry is used only by the on-demand Mac workbench, not subscription
// fetching or the legacy monitor runner. Each sample keeps its original timeout.
func (st *SpeedTester) TestSinglePhysicalContext(ctx context.Context, name string, proxy *CProxy, emit func(*Result) bool) *Result {
	if runtime.GOOS != "darwin" {
		return st.TestSingleContext(ctx, name, proxy, emit)
	}
	return st.testSinglePhysicalContext(ctx, name, proxy, emit, PreparePhysicalProbeProxy)
}

func (st *SpeedTester) testSinglePhysicalContext(ctx context.Context, name string, proxy *CProxy, emit func(*Result) bool, prepare physicalProbePreparer) *Result {
	if ctx == nil {
		ctx = context.Background()
	}
	result := &Result{ProxyName: name}
	if proxy != nil {
		result.ProxyConfig = proxy.Config
		if proxy.Proxy != nil {
			result.ProxyType = proxy.Type().String()
		}
	}
	samples := st.config.PingCount
	if samples <= 0 {
		samples = 6
	}
	timeout := st.config.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	prepCtx, cancel := context.WithTimeout(ctx, timeout)
	client, path, cleanup, err := openPhysicalProbeClient(prepCtx, proxy, timeout, samples*len(LatencyProbeURLs(st.probeURL())), prepare)
	cancel()
	defer cleanup()
	if err != nil {
		result.LatencyErrorClass = "physical_path"
		result.LatencyError = "未执行：物理出口准备未完成"
	} else {
		measured := st.testLatencyWithClientContext(ctx, client, samples)
		result.Latency, result.Jitter, result.PacketLoss, result.LatencySamples = measured.avgLatency, measured.jitter, measured.packetLoss, measured.samples
	}
	if path != nil {
		snapshot := path()
		result.NetworkPath = &snapshot
		if err != nil && snapshot.FailureReason == "unverified_ipv6_node_family" {
			result.LatencyError = "未执行：物理出口当前不支持此节点的 IPv6 地址"
		}
	}
	if emit != nil {
		emit(result.snapshot())
	}
	return result
}
