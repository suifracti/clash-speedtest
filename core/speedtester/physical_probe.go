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

// Workbench opt-in; the strict entry above remains unchanged for IPOK and
// single-path diagnostics with separately authorized network budgets.
func OpenPhysicalAutoProbeClient(ctx context.Context, original *CProxy, timeout time.Duration, sockets int) (*http.Client, func() DownloadNetworkPath, func(), error) {
	return openPhysicalProbeClient(ctx, original, timeout, sockets, func(ctx context.Context, p *CProxy, n int) (*CProxy, *PhysicalDownloadEgress, error) {
		return PreparePhysicalAutoProxy(ctx, p, timeout, n)
	})
}

func openPhysicalProbeClient(ctx context.Context, original *CProxy, timeout time.Duration, sockets int, prepare physicalProbePreparer) (*http.Client, func() DownloadNetworkPath, func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, func() {}, err
	}
	if prepare == nil {
		return nil, nil, func() {}, fmt.Errorf("physical probe preparer is nil")
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
	if st == nil {
		return nil
	}
	if runtime.GOOS != "darwin" {
		return st.TestSingleContext(ctx, name, proxy, emit)
	}
	var configTimeout time.Duration
	if st.config != nil {
		configTimeout = st.config.Timeout
	}
	return st.testSinglePhysicalContext(ctx, name, proxy, emit, func(ctx context.Context, p *CProxy, n int) (*CProxy, *PhysicalDownloadEgress, error) {
		return PreparePhysicalAutoProxy(ctx, p, configTimeout, n)
	})
}

func (st *SpeedTester) testSinglePhysicalContext(ctx context.Context, name string, proxy *CProxy, emit func(*Result) bool, prepare physicalProbePreparer) *Result {
	if st == nil {
		return nil
	}
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
	samples := 6
	timeout := 5 * time.Second
	if st.config != nil {
		if st.config.PingCount > 0 {
			samples = st.config.PingCount
		}
		if st.config.Timeout > 0 {
			timeout = st.config.Timeout
		}
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
		if err != nil && snapshot.FailureReason == "physical_ipv6_interface_unavailable" {
			result.LatencyError = "未执行：当前物理网卡没有可用 IPv6 地址"
		}
	}
	if emit != nil {
		emit(result.snapshot())
	}
	return result
}
