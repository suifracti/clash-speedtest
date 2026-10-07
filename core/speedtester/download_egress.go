package speedtester

// Local path verification failures must not be reported as node failures.
type PhysicalPathError struct{ Cause error }

func (e *PhysicalPathError) Error() string { return "physical download path could not be verified" }
func (e *PhysicalPathError) Unwrap() error { return e.Cause }

// Network conditions are saved with the measurement so old system-DNS/default
// binding runs are never silently treated as verified physical-path results.
type DownloadNetworkPath struct {
	Method                string `json:"method"`
	Interface             string `json:"interface,omitempty"`
	DNSMode               string `json:"dns_mode,omitempty"`
	DNSBindVerified       bool   `json:"dns_bind_verified"`
	SocketBindVerified    bool   `json:"socket_bind_verified"`
	TCPBindings           int    `json:"tcp_bindings"`
	UDPBindings           int    `json:"udp_bindings"`
	DNSDurationNS         int64  `json:"dns_duration_ns,omitempty"`
	PreparationDurationNS int64  `json:"preparation_duration_ns,omitempty"`
}
