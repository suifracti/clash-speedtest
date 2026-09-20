package history

import "time"

// LatencyTest is one user-triggered, single-node latency test. It is separate
// from monitor runs because it represents an on-demand workbench action, not a
// scheduler tick.
type LatencyTest struct {
	AttemptID         string              `json:"attempt_id"`
	ProfileID         string              `json:"profile_id"`
	NodeKey           string              `json:"node_key"`
	NodeIdentityKey   string              `json:"node_identity_key"`
	ConfigRevisionKey string              `json:"config_revision_key"`
	DisplayName       string              `json:"display_name"`
	NodeType          string              `json:"node_type"`
	TestProject       string              `json:"test_project"`
	RequestedAt       time.Time           `json:"requested_at"`
	StartedAt         time.Time           `json:"started_at"`
	FinishedAt        time.Time           `json:"finished_at"`
	Status            string              `json:"status"`
	LatencyMs         int64               `json:"latency_ms"`
	JitterMs          int64               `json:"jitter_ms"`
	PacketLoss        float64             `json:"packet_loss"`
	TotalSamples      int                 `json:"total_samples"`
	SuccessSamples    int                 `json:"success_samples"`
	FailureSamples    int                 `json:"failure_samples"`
	ErrorMessage      string              `json:"error_message,omitempty"`
	Samples           []LatencyTestSample `json:"samples"`
}

// LatencyTestSample is an immutable raw ping result belonging to one attempt.
type LatencyTestSample struct {
	Seq       int       `json:"seq"`
	Timestamp time.Time `json:"timestamp"`
	LatencyMs int64     `json:"latency_ms"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
}

// LatencyTestFilter scopes history to a logical subscription node.
type LatencyTestFilter struct {
	ProfileID string
	NodeKey   string
	Limit     int
}
