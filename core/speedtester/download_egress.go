package speedtester

import (
	"context"
	"errors"
	"sync"
	"time"
)

const (
	physicalDownloadPreparationTimeout = 3 * time.Second
	physicalDownloadLookupTimeout      = 2 * time.Second
	physicalDownloadDNSRequestLimit    = 4
	physicalDownloadSocketRequestLimit = 4
)

var errPhysicalDownloadBudgetExceeded = errors.New("physical download request budget exhausted")

type physicalRequestBudget struct {
	mu    sync.Mutex
	limit int
	used  int
}

func newPhysicalRequestBudget(limit int) *physicalRequestBudget {
	return &physicalRequestBudget{limit: limit}
}

func (b *physicalRequestBudget) reserve(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.used >= b.limit {
		return errPhysicalDownloadBudgetExceeded
	}
	// Check again under the budget lock so a cancellation observed while
	// waiting for another request cannot consume another network operation.
	if err := ctx.Err(); err != nil {
		return err
	}
	b.used++
	return nil
}

func (b *physicalRequestBudget) count() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used
}

func boundedPhysicalContext(parent context.Context, maximum time.Duration) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	if maximum <= 0 || maximum > physicalDownloadPreparationTimeout {
		maximum = physicalDownloadPreparationTimeout
	}
	return context.WithTimeout(parent, maximum)
}

// Local path verification failures must not be reported as node failures.
type PhysicalPathError struct{ Cause error }

func (e *PhysicalPathError) Error() string { return "physical download path could not be verified" }
func (e *PhysicalPathError) Unwrap() error { return e.Cause }

// Network conditions are saved with the measurement so old system-DNS/default
// binding runs are never silently treated as verified physical-path results.
type DownloadNetworkPath struct {
	Method                string `json:"method"`
	Interface             string `json:"interface,omitempty"`
	AddressFamily         string `json:"address_family,omitempty"`
	ResolutionSource      string `json:"resolution_source,omitempty"`
	DNSMode               string `json:"dns_mode,omitempty"`
	TUNEvidence           string `json:"tun_evidence,omitempty"`
	FailureReason         string `json:"failure_reason,omitempty"`
	DNSRequests           int    `json:"dns_requests,omitempty"`
	DNSDialAttempts       int    `json:"dns_dial_attempts,omitempty"`
	TCPDialAttempts       int    `json:"tcp_dial_attempts,omitempty"`
	UDPDialAttempts       int    `json:"udp_dial_attempts,omitempty"`
	DNSBindVerified       bool   `json:"dns_bind_verified"`
	SocketBindVerified    bool   `json:"socket_bind_verified"`
	TCPBindings           int    `json:"tcp_bindings"`
	UDPBindings           int    `json:"udp_bindings"`
	DNSDurationNS         int64  `json:"dns_duration_ns,omitempty"`
	PreparationDurationNS int64  `json:"preparation_duration_ns,omitempty"`
}
