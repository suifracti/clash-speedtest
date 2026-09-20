package application

import "time"

// TestConfig defines parameters for running speed and stability tests.
type TestConfig struct {
	Metrics      []string `json:"metrics"` // "latency", "download", "upload", "antigravity"
	Concurrent   int      `json:"concurrent"`
	TimeoutSec   int      `json:"timeout_sec"`
	DownloadSize int      `json:"download_size"` // bytes
	UploadSize   int      `json:"upload_size"`   // bytes
	ServerURL    string   `json:"server_url"`
	Rounds       int      `json:"rounds"`
}

// BatchTestRequest requests a test run across selected or all nodes of an airport.
type BatchTestRequest struct {
	AirportID string     `json:"airport_id"`
	NodeNames []string   `json:"node_names"` // empty means all
	Config    TestConfig `json:"config"`
}

// SingleTestRequest requests testing a single node (useful for retests).
type SingleTestRequest struct {
	AirportID string     `json:"airport_id"`
	NodeName  string     `json:"node_name"`
	Config    TestConfig `json:"config"`
}

// WorkbenchLatencyTestRequest is the stable-identity request for the first
// formal workbench path. Raw subscription config never crosses this boundary.
type WorkbenchLatencyTestRequest struct {
	ProfileID      string `json:"profile_id"`
	NodeKey        string `json:"node_key"`
	TestProject    string `json:"test_project"`
	TimeoutSeconds int64  `json:"timeout_seconds"`
}

// WorkbenchLatencyHistoryQuery scopes history to one logical subscription node.
type WorkbenchLatencyHistoryQuery struct {
	ProfileID string `json:"profile_id"`
	NodeKey   string `json:"node_key"`
	Limit     int    `json:"limit,omitempty"`
}

// WorkbenchLatencyHistoryDetailQuery binds a detail read to the same logical
// node scope and immutable attempt ID that produced the history row.
type WorkbenchLatencyHistoryDetailQuery struct {
	ProfileID string `json:"profile_id"`
	NodeKey   string `json:"node_key"`
	AttemptID string `json:"attempt_id"`
}

type WorkbenchLatencySampleDTO struct {
	Seq       int       `json:"seq"`
	Timestamp time.Time `json:"timestamp"`
	LatencyMs int64     `json:"latency_ms"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
}

// WorkbenchLatencyTestDTO is the shared Web/Wails result and history shape.
// PersistenceState is independent from the test Status so a valid result can
// still be shown when the history transaction fails.
type WorkbenchLatencyTestDTO struct {
	AttemptID         string                      `json:"attempt_id"`
	ProfileID         string                      `json:"profile_id"`
	NodeKey           string                      `json:"node_key"`
	NodeIdentityKey   string                      `json:"node_identity_key"`
	ConfigRevisionKey string                      `json:"config_revision_key"`
	DisplayName       string                      `json:"display_name"`
	NodeType          string                      `json:"node_type"`
	TestProject       string                      `json:"test_project"`
	RequestedAt       time.Time                   `json:"requested_at"`
	StartedAt         time.Time                   `json:"started_at"`
	FinishedAt        time.Time                   `json:"finished_at"`
	Status            string                      `json:"status"`
	LatencyMs         int64                       `json:"latency_ms"`
	JitterMs          int64                       `json:"jitter_ms"`
	PacketLoss        float64                     `json:"packet_loss"`
	TotalSamples      int                         `json:"total_samples"`
	SuccessSamples    int                         `json:"success_samples"`
	FailureSamples    int                         `json:"failure_samples"`
	ErrorMessage      string                      `json:"error_message,omitempty"`
	Samples           []WorkbenchLatencySampleDTO `json:"samples"`
	PersistenceState  string                      `json:"persistence_state"`
	PersistenceError  string                      `json:"persistence_error,omitempty"`
}

// TestStatus represents real-time progress of an active test.
type TestStatus struct {
	IsRunning    bool      `json:"is_running"`
	CurrentNode  string    `json:"current_node,omitempty"`
	CurrentStep  string    `json:"current_step,omitempty"` // latency, download, upload, antigravity
	CurrentIndex int       `json:"current_index"`
	TotalNodes   int       `json:"total_nodes"`
	Percent      int       `json:"percent"`
	AirportName  string    `json:"airport_name,omitempty"`
	StartedAt    time.Time `json:"started_at,omitempty"`
}

// TokenStatusDTO conveys the current Antigravity authentication status.
type TokenStatusDTO struct {
	HasToken bool   `json:"has_token"`
	Source   string `json:"source"`
	Preview  string `json:"preview"`
}

// AirportDTO represents airport subscription status for GUI/API display.
type AirportDTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	NodeCount int       `json:"node_count"`
	HasCache  bool      `json:"has_cache"`
}

// AppSettings represents user-configurable persistent settings.
type AppSettings struct {
	PreferredBrowser string `json:"preferred_browser"` // "auto", "chrome", "edge", "default", or custom path
}

// ControllerConfigDTO configures connection to external proxy controller.
type ControllerConfigDTO struct {
	Endpoint                     string `json:"endpoint"` // e.g. "http://127.0.0.1:9090" or "https://remote:9090"
	Secret                       string `json:"secret,omitempty"`
	Mode                         string `json:"mode"` // "external" (default) or "standalone"
	AllowRemote                  bool   `json:"allow_remote,omitempty"`
	AllowInsecurePlaintextRemote bool   `json:"allow_insecure_plaintext_remote,omitempty"`
}

// ControllerStatusDTO conveys current connection state and active proxy selection.
type ControllerStatusDTO struct {
	Connected       bool     `json:"connected"`
	Endpoint        string   `json:"endpoint"`
	CoreVersion     string   `json:"core_version,omitempty"`
	CoreType        string   `json:"core_type,omitempty"`
	CurrentGroup    string   `json:"current_group,omitempty"`
	CurrentNode     string   `json:"current_node,omitempty"`
	LockedNode      string   `json:"locked_node,omitempty"`
	Mode            string   `json:"mode"` // "monitor_only", "recommend", "auto"
	HasSecret       bool     `json:"has_secret"`
	AvailableGroups []string `json:"available_groups,omitempty"`
}

// SelectNodeRequest requests switching active proxy for a group.
type SelectNodeRequest struct {
	Group string `json:"group"`
	Node  string `json:"node"`
}
