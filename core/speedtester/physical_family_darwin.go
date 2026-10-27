//go:build darwin

package speedtester

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"strings"
	"sync"
	"syscall"
	"time"

	C "github.com/metacubex/mihomo/constant"
)

const physicalAutoFamilyMethod = "physical_socket_auto_family_v1"

// Only the Mac workbench opts in. Strict/single-path callers keep the existing
// preparer. Target HTTP/TLS runs after this wrapper has selected one node stream.
type physicalFamilyProxy struct {
	C.Proxy
	primary, alternate            *CProxy
	primaryPath, alternatePath    *PhysicalDownloadEgress
	timeout                       time.Duration
	selectionGate                 chan struct{}
	mu                            sync.Mutex
	settled, selectedIPv6, closed bool
	active                        map[uint64]context.CancelFunc
	nextDial                      uint64
	attempts                      []PhysicalFamilyAttempt
	close4, close6                sync.Once
}

func wrapPhysicalFamilies(primary *CProxy, path *PhysicalDownloadEgress, alternate *CProxy, alternatePath *PhysicalDownloadEgress, timeout time.Duration) *CProxy {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	p := &physicalFamilyProxy{Proxy: primary.Proxy, primary: primary, primaryPath: path, alternate: alternate, alternatePath: alternatePath, timeout: timeout, selectionGate: make(chan struct{}, 1), active: make(map[uint64]context.CancelFunc)}
	path.mu.Lock()
	path.autoSnapshot = p.snapshot
	path.mu.Unlock()
	return &CProxy{Proxy: p, Config: primary.Config, sourceTLSPrepared: primary.sourceTLSPrepared}
}

func familyConnectFailed(ctx context.Context, err error) bool {
	var local *PhysicalPathError
	if err == nil || ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.As(err, &local) {
		return false
	}
	var network net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &network) && network.Timeout()) ||
		errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "unexpected eof")
}

func (p *physicalFamilyProxy) attempt(ctx context.Context, proxy *CProxy, family string, metadata *C.Metadata) (C.Conn, error) {
	start := time.Now()
	conn, err := proxy.DialContext(ctx, metadata)
	path := p.primaryPath
	if proxy == p.alternate {
		path = p.alternatePath
	}
	if err == nil && conn != nil && !path.snapshotBase().SocketBindVerified {
		conn.Close()
		conn = nil
		err = &PhysicalPathError{Cause: errors.New("selected family has no verified physical socket")}
	}
	if ctx.Err() != nil {
		if conn != nil {
			conn.Close()
			conn = nil
		}
		err = ctx.Err()
	}
	outcome := "connected"
	if err != nil {
		outcome = "connection_failed"
		var timeout net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
			outcome = "timeout"
		}
		if errors.Is(err, context.Canceled) {
			outcome = "cancelled"
		}
	}
	p.mu.Lock()
	p.attempts = append(p.attempts, PhysicalFamilyAttempt{AddressFamily: family, Outcome: outcome, DurationNS: time.Since(start).Nanoseconds()})
	p.mu.Unlock()
	return conn, err
}

func (p *physicalFamilyProxy) DialContext(ctx context.Context, metadata *C.Metadata) (C.Conn, error) {
	run, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, context.Canceled
	}
	p.nextDial++
	id := p.nextDial
	p.active[id] = cancel
	settled, selected := p.settled, p.selectedIPv6
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.active, id); p.mu.Unlock() }()
	if settled {
		return p.dialSelected(run, metadata, selected)
	}
	select {
	case p.selectionGate <- struct{}{}:
	case <-run.Done():
		return nil, run.Err()
	}
	p.mu.Lock()
	settled, selected = p.settled, p.selectedIPv6
	p.mu.Unlock()
	if settled {
		<-p.selectionGate
		return p.dialSelected(run, metadata, selected)
	}
	defer func() { <-p.selectionGate }()
	if run.Err() != nil {
		return nil, run.Err()
	}
	// Publish the selection only after the complete IPv4/IPv6 decision, so no
	// concurrent request uses a primary adapter while fallback closes it.
	defer func() { p.mu.Lock(); p.settled = true; p.mu.Unlock() }()
	deadline, _ := run.Deadline()
	first := run
	stop := func() {}
	if p.alternate != nil {
		slice := time.Until(deadline) / 2
		if slice > 5*time.Second {
			slice = 5 * time.Second
		}
		first, stop = context.WithTimeout(run, slice)
	}
	conn, err := p.attempt(first, p.primary, "ipv4", metadata.Clone())
	stop()
	if conn != nil && err != nil {
		conn.Close()
		conn = nil
	}
	if conn != nil || p.alternate == nil || !familyConnectFailed(run, err) {
		return conn, err
	}
	p.mu.Lock()
	p.selectedIPv6 = true
	p.mu.Unlock()
	p.close4.Do(func() { _ = p.primary.Close() })
	if run.Err() != nil {
		return nil, run.Err()
	}
	return p.attempt(run, p.alternate, "ipv6", metadata)
}

func (p *physicalFamilyProxy) dialSelected(ctx context.Context, metadata *C.Metadata, selectedIPv6 bool) (C.Conn, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	proxy := p.primary
	if selectedIPv6 {
		proxy = p.alternate
	}
	conn, err := proxy.DialContext(ctx, metadata)
	if ctx.Err() != nil {
		if conn != nil {
			conn.Close()
		}
		return nil, ctx.Err()
	}
	if err != nil && conn != nil {
		conn.Close()
		conn = nil
	}
	return conn, err
}

func (p *physicalFamilyProxy) Close() error {
	p.mu.Lock()
	p.closed = true
	cancels := make([]context.CancelFunc, 0, len(p.active))
	for _, cancel := range p.active {
		cancels = append(cancels, cancel)
	}
	p.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	p.close4.Do(func() { _ = p.primary.Close() })
	if p.alternate != nil {
		p.close6.Do(func() { _ = p.alternate.Close() })
	}
	return nil
}

func (p *physicalFamilyProxy) snapshot() DownloadNetworkPath {
	p.mu.Lock()
	selected := p.selectedIPv6
	settled := p.settled
	attempts := append([]PhysicalFamilyAttempt(nil), p.attempts...)
	p.mu.Unlock()
	first := p.primaryPath.snapshotBase()
	result := first
	result.Method = physicalAutoFamilyMethod
	result.AddressFallback = "ipv6_unavailable"
	if p.alternatePath != nil {
		second := p.alternatePath.snapshotBase()
		if selected {
			result = second
			result.AddressFallback = "ipv6_not_started"
			if len(attempts) > 1 {
				result.AddressFallback = "ipv6_failed"
				if attempts[1].Outcome == "connected" {
					result.AddressFallback = "used_ipv6"
				}
			}
		} else if p.alternate == nil {
			result.AddressFallback = "ipv6_unavailable"
			result.AddressFallbackFailure = second.FailureReason
		} else if settled {
			result.AddressFallback = "not_needed"
		} else {
			result.AddressFallback = "available"
		}
		// Both paths charge the same counters; do not double count them.
		result.DNSRequests = first.DNSRequests
		result.DNSDialAttempts = first.DNSDialAttempts
		result.UDPDialAttempts = first.UDPDialAttempts
		result.TCPDialAttempts = first.TCPDialAttempts
		result.DNSDurationNS = first.DNSDurationNS + second.DNSDurationNS
		result.PreparationDurationNS = first.PreparationDurationNS + second.PreparationDurationNS
	}
	result.Method = physicalAutoFamilyMethod
	result.FamilyAttempts = attempts
	return result
}

func autoFamilySupported(kind string) bool {
	switch kind {
	case "hysteria2", "hysteria", "tuic", "vless", "vmess", "trojan", "ss":
		return true
	default:
		return false
	}
}

func autoFamilyConfig(config map[string]any) bool {
	kind, _ := config["type"].(string)
	host, _ := config["server"].(string)
	version, _ := config["ip-version"].(string)
	if !autoFamilySupported(kind) || host == "" || (version != "" && version != "dual" && version != "ipv4-prefer") {
		return false
	}
	_, err := netip.ParseAddr(host)
	return err != nil
}
func hasPhysicalIPv6(path *PhysicalDownloadEgress) bool {
	iface, err := net.InterfaceByName(path.iface)
	if err != nil {
		return false
	}
	addrs, err := iface.Addrs()
	return err == nil && usablePhysicalIPv6(addrs)
}

// PreparePhysicalAutoProxy shares the existing DNS/socket limits across both
// families. Preparing an alternate never sends node authentication or target HTTP.
func PreparePhysicalAutoProxy(ctx context.Context, original *CProxy, timeout time.Duration, sockets int) (*CProxy, *PhysicalDownloadEgress, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	var seed *PhysicalDownloadEgress
	prepare := func(ctx context.Context, proxy *CProxy, n int) (*CProxy, *PhysicalDownloadEgress, error) {
		e := newPhysicalDownloadEgress()
		e.dnsTimeout = timeout
		e.dnsRequests = newPhysicalRequestBudget(physicalDownloadDNSRequestLimit * 4)
		e.dnsDials = newPhysicalRequestBudget(physicalDownloadDNSRequestLimit * 4)
		if seed != nil {
			e.dnsRequests = seed.dnsRequests
			e.dnsDials = seed.dnsDials
		}
		p, path, err := preparePhysicalProxyUsing(ctx, proxy, n, e)
		if seed == nil {
			seed = path
		} else {
			path.tcpDials = seed.tcpDials
			path.udpDials = seed.udpDials
		}
		return p, path, err
	}
	return prepareAutoPhysicalProxyUsing(ctx, original, timeout, sockets, prepare, hasPhysicalIPv6)
}

func prepareAutoPhysicalProxyUsing(ctx context.Context, original *CProxy, timeout time.Duration, sockets int, prepare physicalProbePreparer, hasIPv6 func(*PhysicalDownloadEgress) bool) (*CProxy, *PhysicalDownloadEgress, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	// Two address families, at most two DNS attempts per family.
	ctx, cancel := context.WithTimeout(ctx, 4*timeout)
	defer cancel()
	primary, path, err := prepare(ctx, original, sockets)
	if err != nil || original == nil || !autoFamilyConfig(original.Config) || !hasIPv6(path) {
		return primary, path, err
	}
	path.mu.Lock()
	path.method = physicalAutoFamilyMethod
	path.mu.Unlock()
	if path.snapshotBase().AddressFamily != "ipv4" || sockets < 2 {
		return primary, path, nil
	}
	cfg := make(map[string]any, len(original.Config))
	for k, v := range original.Config {
		cfg[k] = v
	}
	cfg["ip-version"] = "ipv6"
	alt, altPath, altErr := prepare(ctx, &CProxy{Config: cfg}, sockets)
	if ctx.Err() != nil {
		primary.Close()
		if alt != nil {
			alt.Close()
		}
		return nil, path, ctx.Err()
	}
	if altErr != nil {
		if alt != nil {
			alt.Close()
		}
		return wrapPhysicalFamilies(primary, path, nil, altPath, timeout), path, nil
	}
	if altPath == nil || altPath.iface != path.iface || altPath.index != path.index || altPath.snapshotBase().AddressFamily != "ipv6" {
		path.setFailure("physical_interface_or_family_changed")
		primary.Close()
		alt.Close()
		return nil, path, &PhysicalPathError{Cause: errors.New("physical interface or address family changed during preparation")}
	}
	return wrapPhysicalFamilies(primary, path, alt, altPath, timeout), path, nil
}

func PlanPhysicalAutoDownloadPath(ctx context.Context, original *CProxy) DownloadNetworkPath {
	path := PlanPhysicalDownloadPath(ctx, original)
	if original != nil && autoFamilyConfig(original.Config) && hasPhysicalIPv6(&PhysicalDownloadEgress{iface: path.Interface}) {
		path.Method = physicalAutoFamilyMethod
	}
	return path
}
