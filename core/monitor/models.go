package monitor

import (
	"context"
	"time"
)

// JobState represents the execution lifecycle state of a MonitorJob.
type JobState string

const (
	JobStateStopped JobState = "stopped"
	JobStateRunning JobState = "running"
	JobStatePaused  JobState = "paused"
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
)

// MonitoredNode represents a proxy node targeted for monitoring.
type MonitoredNode struct {
	NodeKey     string         `json:"node_key"`
	DisplayName string         `json:"display_name"`
	Type        string         `json:"type"`
	Server      string         `json:"server"`
	Port        int            `json:"port"`
	RawConfig   map[string]any `json:"raw_config,omitempty"`
}

// MonitorJob holds the persistent configuration and state of a 24/7 monitoring task.
type MonitorJob struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	ProfileID string          `json:"profile_id"`
	NodeKeys  []string        `json:"node_keys"`
	Nodes     []MonitoredNode `json:"nodes"`
	ProbeSet  ProbeSetType    `json:"probe_set"`
	Interval  time.Duration   `json:"interval"`
	Timeout   time.Duration   `json:"timeout"`
	State     JobState        `json:"state"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
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
	NodeKey             string         `json:"node_key"`
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

// SampleFilter specifies query criteria for retrieving raw samples from persistence.
type SampleFilter struct {
	JobID     string
	RunID     string
	NodeKey   string
	ProfileID string
	ProbeType string
	Success   *bool
	Since     *time.Time
	Until     *time.Time
	Limit     int
	Offset    int
	OrderDesc bool // true: newest first; false: chronological ascending
}

// SampleStore defines the persistence abstraction for saving and querying runs and samples.
type SampleStore interface {
	SaveMonitorRun(ctx context.Context, run *MonitorRun) error
	UpdateMonitorRun(ctx context.Context, run *MonitorRun) error
	SaveMonitorSamples(ctx context.Context, samples []*MonitorSample) error
	QueryMonitorRuns(ctx context.Context, jobID string, limit int) ([]*MonitorRun, error)
	QueryMonitorSamples(ctx context.Context, filter SampleFilter) ([]*MonitorSample, error)
	GetNodeTimelineSamples(ctx context.Context, nodeKey string, since time.Time) ([]*MonitorSample, error)
}
