package monitor

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// JobState represents the execution lifecycle state of a MonitorJob.
type JobState string

const (
	JobStateStopped JobState = "stopped"
	JobStateRunning JobState = "running"
	JobStatePaused  JobState = "paused"
	JobStateBlocked JobState = "blocked"
)

// ProbeSetType defines the intensity and scope of probes executed on each tick.
type ProbeSetType string

const (
	// ProbeSetLight executes fast latency & lightweight HTTP reachability (e.g. Cloudflare trace / 204).
	ProbeSetLight ProbeSetType = "light"
	// ProbeSetService tests common essential services (Google, Cloudflare, GitHub, etc.).
	ProbeSetService ProbeSetType = "service"
	// ProbeSetHeavy executes deep diagnostics including multi-target TTFB, IP drift, and bandwidth samples.
	ProbeSetHeavy ProbeSetType = "heavy"
)

// RunStatus defines the outcome status of a single scheduled monitoring run.
type RunStatus string

const (
	RunStatusRunning       RunStatus = "running"
	RunStatusCompleted     RunStatus = "completed"
	RunStatusPartialFailed RunStatus = "partial_failed"
	RunStatusFailed        RunStatus = "failed"
	RunStatusSkipped       RunStatus = "skipped"
	// RunStatusPersistenceFailed means probe execution may have completed, but
	// the durable run/sample transaction did not complete successfully.
	RunStatusPersistenceFailed RunStatus = "persistence_failed"
)

const (
	PersistenceStateHealthy  = "healthy"
	PersistenceStateDegraded = "degraded"
)

// MonitoredNode represents a proxy node targeted for monitoring.
type MonitoredNode struct {
	NodeKey           string         `json:"node_key"`            // Backward-compatible composite key (endpoint + cred hash)
	NodeIdentityKey   string         `json:"node_identity_key"`   // Pure physical/transport endpoint identity
	ConfigRevisionKey string         `json:"config_revision_key"` // Config & credential revision digest
	DisplayName       string         `json:"display_name"`
	Type              string         `json:"type"`
	Server            string         `json:"server"`
	Port              int            `json:"port"`
	RawConfig         map[string]any `json:"raw_config,omitempty"`
}

// MonitorJob holds the persistent configuration and state of a 24/7 monitoring task.
type MonitorJob struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	ProfileID     string          `json:"profile_id"`
	NodeKeys      []string        `json:"node_keys"`
	Nodes         []MonitoredNode `json:"nodes"`
	ProbeSet      ProbeSetType    `json:"probe_set"`
	Interval      time.Duration   `json:"interval"`
	Timeout       time.Duration   `json:"timeout"`
	State         JobState        `json:"state"`
	BlockedReason string          `json:"blocked_reason,omitempty"`
	// PersistenceState and PersistenceError are runtime read-model fields. They
	// are deliberately excluded from MonitorJobDefinition persistence.
	PersistenceState string    `json:"persistence_state,omitempty"`
	PersistenceError string    `json:"persistence_error,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// MonitorJobDefinitionVersion is the version of the credential-free persisted
// definition shape. Runtime scheduler state is deliberately not part of it.
const MonitorJobDefinitionVersion = 1

// MonitorJobNodeReference is the safe, stable reference stored for one selected
// node. The current cached profile must be re-resolved before the node can run.
type MonitorJobNodeReference struct {
	NodeKey           string `json:"node_key"`
	NodeIdentityKey   string `json:"node_identity_key"`
	ConfigRevisionKey string `json:"config_revision_key"`
	DisplayName       string `json:"display_name"`
	Type              string `json:"type"`
}

// MonitorJobDefinition is the durable product definition. It contains no raw
// proxy configuration, credentials, controller secrets, or scheduler runtime.
type MonitorJobDefinition struct {
	ID                string                    `json:"id"`
	Name              string                    `json:"name"`
	ProfileID         string                    `json:"profile_id"`
	Nodes             []MonitorJobNodeReference `json:"nodes"`
	ProbeSet          ProbeSetType              `json:"probe_set"`
	Interval          time.Duration             `json:"interval"`
	Timeout           time.Duration             `json:"timeout"`
	CreatedAt         time.Time                 `json:"created_at"`
	UpdatedAt         time.Time                 `json:"updated_at"`
	DefinitionVersion int                       `json:"definition_version"`
}

// MonitorRun records the execution metadata of one scheduled monitoring round.
type MonitorRun struct {
	RunID        string     `json:"run_id"`
	JobID        string     `json:"job_id"`
	ScheduledAt  time.Time  `json:"scheduled_at"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	Status       RunStatus  `json:"status"`
	TotalNodes   int        `json:"total_nodes"`
	SuccessNodes int        `json:"success_nodes"`
	FailedNodes  int        `json:"failed_nodes"`
	ErrorMessage string     `json:"error_message,omitempty"`
}

// MonitorSample records an individual, immutable probe sample at an exact timestamp.
// Aggregated metrics must never replace these raw samples.
type MonitorSample struct {
	SampleID            string         `json:"sample_id"`
	RunID               string         `json:"run_id"`
	NodeKey             string         `json:"node_key"`            // Backward-compatible composite key
	NodeIdentityKey     string         `json:"node_identity_key"`   // Pure transport endpoint identity
	ConfigRevisionKey   string         `json:"config_revision_key"` // Config & credential revision digest
	ProfileID           string         `json:"profile_id"`
	DisplayNameSnapshot string         `json:"display_name_snapshot"`
	ProbeType           string         `json:"probe_type"` // "rtt", "ttfb", "http_status", "exit_ip", "ai_check"
	Target              string         `json:"target"`     // e.g. "https://cp.cloudflare.com/generate_204"
	Timestamp           time.Time      `json:"timestamp"`
	Success             bool           `json:"success"`
	Latency             time.Duration  `json:"latency"`
	TTFB                time.Duration  `json:"ttfb"`
	ErrorClass          string         `json:"error_class"` // "none", "timeout", "conn_refused", "dns_error", "tls_error", "blocked", "http_status_error"
	ErrorDetail         string         `json:"error_detail,omitempty"`
	ExitIP              string         `json:"exit_ip,omitempty"`
	ExitRegion          string         `json:"exit_region,omitempty"`
	Metadata            map[string]any `json:"metadata,omitempty"`
}

// SampleFilter specifies query criteria for retrieving raw samples from persistence using OFFSET.
type SampleFilter struct {
	JobID           string
	RunID           string
	NodeKey         string
	NodeIdentityKey string
	ProfileID       string
	ProbeType       string
	Target          string
	Success         *bool
	Since           *time.Time
	Until           *time.Time
	Limit           int
	Offset          int
	OrderDesc       bool // true: newest first; false: chronological ascending
}

// CursorFilter specifies query criteria for keyset/cursor-based sample pagination.
type CursorFilter struct {
	NodeIdentityKey string     `json:"node_identity_key,omitempty"`
	LegacyNodeKey   string     `json:"legacy_node_key,omitempty"` // For PR#3 backfilled samples compatibility
	NodeKey         string     `json:"node_key,omitempty"`
	ProfileID       string     `json:"profile_id,omitempty"`
	ProbeType       string     `json:"probe_type,omitempty"`
	Target          string     `json:"target,omitempty"`
	Success         *bool      `json:"success,omitempty"`
	Since           *time.Time `json:"since,omitempty"`
	Until           *time.Time `json:"until,omitempty"`
	Limit           int        `json:"limit"`
	OrderDesc       bool       `json:"order_desc"`       // true: newest first (default); false: chronological ascending
	Cursor          string     `json:"cursor,omitempty"` // Opaque cursor token
}

// SampleCursorPage represents a page of MonitorSample results using keyset pagination.
// In this revision, pagination is single-direction stream (next_cursor only).
type SampleCursorPage struct {
	Items      []*MonitorSample `json:"items"`
	NextCursor string           `json:"next_cursor,omitempty"`
	HasMore    bool             `json:"has_more"`
	Limit      int              `json:"limit"`
}

// StatsQuery defines filtering criteria for deriving statistical metrics over raw samples.
type StatsQuery struct {
	NodeIdentityKey string     `json:"node_identity_key,omitempty"`
	LegacyNodeKey   string     `json:"legacy_node_key,omitempty"` // For PR#3 backfilled samples compatibility
	NodeKey         string     `json:"node_key,omitempty"`
	ProfileID       string     `json:"profile_id,omitempty"`
	ProbeType       string     `json:"probe_type,omitempty"`
	Target          string     `json:"target,omitempty"`
	Since           *time.Time `json:"since,omitempty"`
	Until           *time.Time `json:"until,omitempty"`
}

// Validation errors for client request inputs (mapped to HTTP 400 Bad Request in Web adapter).
var (
	ErrInvalidCursor          = errors.New("invalid cursor token")
	ErrInvalidTimeRange       = errors.New("invalid time range: since cannot be after until")
	ErrInvalidLimit           = errors.New("limit must be between 1 and 1000")
	ErrFutureCutoff           = errors.New("cutoff time cannot be in the future")
	ErrInvalidCustomDays      = errors.New("custom days must be between 1 and 36500")
	ErrInvalidRetentionPolicy = errors.New("unsupported retention policy")
	ErrQueryWindowTooLarge    = errors.New("query window too large: please specify a narrower time window")
)

// ValidationError represents an explicit client-side validation failure.
type ValidationError struct {
	Err error
}

func (v *ValidationError) Error() string {
	if v.Err != nil {
		return v.Err.Error()
	}
	return "validation error"
}

func (v *ValidationError) Unwrap() error {
	return v.Err
}

// NewValidationError constructs a ValidationError with the given message.
func NewValidationError(msg string) error {
	return &ValidationError{Err: errors.New(msg)}
}

// WrapValidationError wraps an existing error as a ValidationError.
func WrapValidationError(err error) error {
	if err == nil {
		return nil
	}
	return &ValidationError{Err: err}
}

// IsValidationError checks whether an error is a ValidationError.
func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}

// DerivedStats contains aggregated metrics computed on-the-fly from raw immutable samples.
type DerivedStats struct {
	SampleCount     int64            `json:"sample_count"`
	SuccessCount    int64            `json:"success_count"`
	FailureCount    int64            `json:"failure_count"`
	SuccessRate     float64          `json:"success_rate"` // 0.0 to 1.0
	LatencyMinMs    *int64           `json:"latency_min_ms"`
	LatencyP50Ms    *int64           `json:"latency_p50_ms"`
	LatencyP95Ms    *int64           `json:"latency_p95_ms"`
	LatencyMaxMs    *int64           `json:"latency_max_ms"`
	TTFBP50Ms       *int64           `json:"ttfb_p50_ms"`
	TTFBP95Ms       *int64           `json:"ttfb_p95_ms"`
	ErrorBreakdown  map[string]int64 `json:"error_breakdown"`
	FirstSampleAt   *time.Time       `json:"first_sample_at"`
	LastSampleAt    *time.Time       `json:"last_sample_at"`
	ObservedSince   *time.Time       `json:"observed_since"`
	ObservedUntil   *time.Time       `json:"observed_until"`
	NodeIdentityKey string           `json:"node_identity_key,omitempty"`
	NodeKey         string           `json:"node_key,omitempty"`
	ProbeType       string           `json:"probe_type,omitempty"`
}

// FacetNode describes one distinct node identity observed among persisted raw samples.
type FacetNode struct {
	NodeIdentityKey string `json:"node_identity_key"`
	NodeKey         string `json:"node_key"`
	DisplayName     string `json:"display_name"`
	ProfileID       string `json:"profile_id"`
	SampleCount     int64  `json:"sample_count"`
}

// MonitorSampleFacets enumerates the filter dimensions that actually exist in persisted
// raw samples. It is a presentation-only projection used to build UI filters, and carries
// no monitoring semantics: it never aggregates, summarizes, or replaces raw samples.
type MonitorSampleFacets struct {
	Nodes       []FacetNode `json:"nodes"`
	Profiles    []string    `json:"profiles"`
	ProbeTypes  []string    `json:"probe_types"`
	Targets     []string    `json:"targets"`
	WindowSince time.Time   `json:"window_since"`
	WindowUntil time.Time   `json:"window_until"`
	Truncated   bool        `json:"truncated"`
}

// RetentionPolicy defines data retention strategies for raw samples.
type RetentionPolicy string

const (
	RetentionKeepAll RetentionPolicy = "keep_all" // Default: Never delete or overwrite raw samples
	Retention30d     RetentionPolicy = "30d"
	Retention90d     RetentionPolicy = "90d"
	Retention180d    RetentionPolicy = "180d"
	RetentionCustom  RetentionPolicy = "custom"
)

// RetentionRequest specifies retention execution options.
type RetentionRequest struct {
	Policy     RetentionPolicy `json:"policy"`
	CustomDays int             `json:"custom_days,omitempty"`
	CutoffTime *time.Time      `json:"cutoff_time,omitempty"`
}

// RetentionResult summarizes the outcome of a retention deletion operation.
type RetentionResult struct {
	Policy         RetentionPolicy `json:"policy"`
	Cutoff         time.Time       `json:"cutoff"`
	SamplesDeleted int64           `json:"samples_deleted"`
	RunsDeleted    int64           `json:"runs_deleted"`
	DurationMs     int64           `json:"duration_ms"`
	Partial        bool            `json:"partial"`
	ErrorMessage   string          `json:"error_message,omitempty"`
}

// RetentionError indicates that retention pruning failed, optionally with partial progress.
type RetentionError struct {
	Result *RetentionResult
	Err    error
}

func (e *RetentionError) Error() string {
	if e.Result != nil && e.Result.Partial {
		return fmt.Sprintf("retention partially applied (%d samples, %d runs deleted): %v",
			e.Result.SamplesDeleted, e.Result.RunsDeleted, e.Err)
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "retention execution failed"
}

func (e *RetentionError) Unwrap() error {
	return e.Err
}

// SampleStore defines the persistence abstraction for saving, querying, calculating, and pruning runs and samples.
type SampleStore interface {
	SaveMonitorRun(ctx context.Context, run *MonitorRun) error
	UpdateMonitorRun(ctx context.Context, run *MonitorRun) error
	SaveMonitorSamples(ctx context.Context, samples []*MonitorSample) error
	QueryMonitorRuns(ctx context.Context, jobID string, limit int) ([]*MonitorRun, error)
	QueryMonitorSamples(ctx context.Context, filter SampleFilter) ([]*MonitorSample, error)
	QueryMonitorSamplesCursor(ctx context.Context, filter CursorFilter) (*SampleCursorPage, error)
	GetDerivedStats(ctx context.Context, query StatsQuery) (*DerivedStats, error)
	ApplyRetention(ctx context.Context, req RetentionRequest) (*RetentionResult, error)
	GetNodeTimelineSamples(ctx context.Context, nodeKey string, since time.Time) ([]*MonitorSample, error)
}
