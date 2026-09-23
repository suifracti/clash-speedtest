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
	Source            string              `json:"source,omitempty"`
	Method            string              `json:"method,omitempty"`
	MethodVersion     int                 `json:"method_version,omitempty"`
	Target            string              `json:"target,omitempty"`
	Unit              string              `json:"unit,omitempty"`
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

// LatencyBatch is a user-confirmed Workbench selection snapshot. RequestID is
// the idempotency key used when a client retries a create request.
type LatencyBatch struct {
	BatchID        string             `json:"batch_id"`
	RequestID      string             `json:"request_id"`
	TestProject    string             `json:"test_project"`
	TimeoutSeconds int64              `json:"timeout_seconds"`
	RequestedAt    time.Time          `json:"requested_at"`
	State          string             `json:"state"`
	ItemCount      int                `json:"item_count"`
	Items          []LatencyBatchItem `json:"items"`
}

// LatencyBatchItem keeps execution and history persistence states independent.
// ResultJSON is a retryable copy of a measured result when saving its attempt
// failed; it is cleared only after the attempt transaction succeeds.
type LatencyBatchItem struct {
	ItemID            string       `json:"item_id"`
	BatchID           string       `json:"batch_id"`
	Ordinal           int          `json:"ordinal"`
	ProfileID         string       `json:"profile_id"`
	NodeKey           string       `json:"node_key"`
	NodeIdentityKey   string       `json:"node_identity_key"`
	ConfigRevisionKey string       `json:"config_revision_key"`
	DisplayName       string       `json:"display_name"`
	NodeType          string       `json:"node_type"`
	ExecutionState    string       `json:"execution_state"`
	PersistenceState  string       `json:"persistence_state"`
	AttemptID         string       `json:"attempt_id,omitempty"`
	RequestedAt       time.Time    `json:"requested_at"`
	StartedAt         time.Time    `json:"started_at,omitempty"`
	FinishedAt        time.Time    `json:"finished_at,omitempty"`
	ErrorMessage      string       `json:"error_message,omitempty"`
	PersistenceError  string       `json:"persistence_error,omitempty"`
	Result            *LatencyTest `json:"result,omitempty"`
	ResultStaged      bool         `json:"-"`
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
	ProfileID         string
	NodeKey           string
	NodeIdentityKey   string
	ConfigRevisionKey string
	Since             *time.Time
	Until             *time.Time
	Limit             int
	BeforeFinishedAt  *time.Time
	BeforeAttemptID   string
}

// LatencyTestQueryResult is a bounded, raw-sample-scoped history page.
// HasMore is true when more matching attempts exist beyond Limit; callers must
// not present the returned samples as a complete observation window then.
type LatencyTestQueryResult struct {
	Tests   []*LatencyTest
	HasMore bool
}

// NodeHistoryRevision is a safe snapshot of one observed configuration revision
// for a stable profile/node identity. It contains no proxy configuration.
type NodeHistoryRevision struct {
	ConfigRevisionKey string    `json:"config_revision_key"`
	NodeKey           string    `json:"node_key"`
	DisplayName       string    `json:"display_name"`
	LastObservedAt    time.Time `json:"last_observed_at"`
}
