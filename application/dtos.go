package application

import (
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
)

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

type WorkbenchLatencyBatchSelection struct {
	ProfileID         string `json:"profile_id"`
	NodeKey           string `json:"node_key"`
	NodeIdentityKey   string `json:"node_identity_key"`
	ConfigRevisionKey string `json:"config_revision_key"`
	DisplayName       string `json:"display_name"`
	NodeType          string `json:"node_type"`
}

type WorkbenchLatencyBatchRequest struct {
	RequestID      string                           `json:"request_id"`
	TestProject    string                           `json:"test_project"`
	TimeoutSeconds int64                            `json:"timeout_seconds"`
	Selections     []WorkbenchLatencyBatchSelection `json:"selections"`
}

type WorkbenchLatencyBatchItemDTO struct {
	ItemID            string                   `json:"item_id"`
	BatchID           string                   `json:"batch_id"`
	Ordinal           int                      `json:"ordinal"`
	ProfileID         string                   `json:"profile_id"`
	NodeKey           string                   `json:"node_key"`
	NodeIdentityKey   string                   `json:"node_identity_key"`
	ConfigRevisionKey string                   `json:"config_revision_key"`
	DisplayName       string                   `json:"display_name"`
	NodeType          string                   `json:"node_type"`
	ExecutionState    string                   `json:"execution_state"`
	PersistenceState  string                   `json:"persistence_state"`
	AttemptID         string                   `json:"attempt_id,omitempty"`
	RequestedAt       time.Time                `json:"requested_at"`
	StartedAt         time.Time                `json:"started_at,omitempty"`
	FinishedAt        time.Time                `json:"finished_at,omitempty"`
	ErrorMessage      string                   `json:"error_message,omitempty"`
	PersistenceError  string                   `json:"persistence_error,omitempty"`
	Result            *WorkbenchLatencyTestDTO `json:"result,omitempty"`
}

type WorkbenchLatencyBatchDTO struct {
	BatchID        string                         `json:"batch_id"`
	RequestID      string                         `json:"request_id"`
	TestProject    string                         `json:"test_project"`
	TimeoutSeconds int64                          `json:"timeout_seconds"`
	RequestedAt    time.Time                      `json:"requested_at"`
	State          string                         `json:"state"`
	ItemCount      int                            `json:"item_count"`
	Items          []WorkbenchLatencyBatchItemDTO `json:"items,omitempty"`
}

// WorkbenchLatencyHistoryQuery scopes history to one logical subscription node.
type WorkbenchLatencyHistoryQuery struct {
	ProfileID         string     `json:"profile_id"`
	NodeKey           string     `json:"node_key"`
	NodeIdentityKey   string     `json:"node_identity_key"`
	ConfigRevisionKey string     `json:"config_revision_key"`
	Since             *time.Time `json:"since,omitempty"`
	Until             *time.Time `json:"until,omitempty"`
	Limit             int        `json:"limit,omitempty"`
	BeforeFinishedAt  *time.Time `json:"before_finished_at,omitempty"`
	BeforeAttemptID   string     `json:"before_attempt_id,omitempty"`
}

// WorkbenchLatencyHistoryDetailQuery binds a detail read to the same logical
// node scope and immutable attempt ID that produced the history row.
type WorkbenchLatencyHistoryDetailQuery struct {
	ProfileID         string     `json:"profile_id"`
	NodeKey           string     `json:"node_key"`
	NodeIdentityKey   string     `json:"node_identity_key"`
	ConfigRevisionKey string     `json:"config_revision_key"`
	AttemptID         string     `json:"attempt_id"`
	Since             *time.Time `json:"since,omitempty"`
	Until             *time.Time `json:"until,omitempty"`
}

// WorkbenchLatencyHistoryResult is one frozen observation-window response.
// Complete is false when the bounded attempt page has more matching history;
// the returned samples remain valid for the range but are not full-window
// statistics in that case.
type WorkbenchLatencyHistoryResult struct {
	Tests    []WorkbenchLatencyTestDTO `json:"tests"`
	Since    time.Time                 `json:"since"`
	Until    time.Time                 `json:"until"`
	AsOf     time.Time                 `json:"as_of"`
	HasMore  bool                      `json:"has_more"`
	Complete bool                      `json:"complete"`
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
	Source            string                      `json:"source,omitempty"`
	Method            string                      `json:"method,omitempty"`
	MethodVersion     int                         `json:"method_version,omitempty"`
	Target            string                      `json:"target,omitempty"`
	Unit              string                      `json:"unit,omitempty"`
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
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	URLDisplay    string    `json:"url_display"`
	URLConfigured bool      `json:"url_configured"`
	UpdatedAt     time.Time `json:"updated_at,omitempty"`
	NodeCount     int       `json:"node_count"`
	HasCache      bool      `json:"has_cache"`
}

// ProfileSourceDTO is a redacted, user-selectable local source summary. It
// never carries subscription URLs or raw cache contents.
type ProfileSourceDTO struct {
	Path             string   `json:"path"`
	Label            string   `json:"label"`
	Available        bool     `json:"available"`
	ProfileCount     int      `json:"profile_count"`
	CacheCount       int      `json:"cache_count"`
	Missing          []string `json:"missing,omitempty"`
	PossibleTestData bool     `json:"possible_test_data"`
	Error            string   `json:"error,omitempty"`
}

// DataMigrationDTO is a safe summary of the legacy-to-canonical data root
// migration. It contains paths and counts only; it never carries database,
// settings, or subscription contents.
type DataMigrationDTO struct {
	State              string `json:"state"` // ready, pending, conflict, invalid, isolated
	SourceHistoryDir   string `json:"source_history_dir,omitempty"`
	TargetHistoryDir   string `json:"target_history_dir"`
	SourceSettingsFile string `json:"source_settings_file,omitempty"`
	TargetSettingsFile string `json:"target_settings_file"`
	SourceHasSQLite    bool   `json:"source_has_sqlite"`
	SourceJSONCount    int    `json:"source_json_count"`
	SourceHasSettings  bool   `json:"source_has_settings"`
	TargetHasSQLite    bool   `json:"target_has_sqlite"`
	TargetJSONCount    int    `json:"target_json_count"`
	TargetHasSettings  bool   `json:"target_has_settings"`
	Error              string `json:"error,omitempty"`
}

// ProfileSetupDTO separates "not yet decided" from a real empty profile
// store, and gives both Web and Wails the same path/import contract.
type ProfileSetupDTO struct {
	State             string             `json:"state"` // ready, needs_choice, error
	Initialized       bool               `json:"initialized"`
	DataRoot          string             `json:"data_root"`
	ProfileDir        string             `json:"profile_dir"`
	HistoryDir        string             `json:"history_dir"`
	SettingsFile      string             `json:"settings_file"`
	UnfinishedStaging []string           `json:"unfinished_staging,omitempty"`
	LockPresent       bool               `json:"lock_present"`
	Error             string             `json:"error,omitempty"`
	Sources           []ProfileSourceDTO `json:"sources,omitempty"`
	Migration         DataMigrationDTO   `json:"migration"`
}

// AppSettings represents user-configurable persistent settings.
type AppSettings struct {
	PreferredBrowser           string                  `json:"preferred_browser"` // "auto", "chrome", "edge", "default", or custom path
	MonitorRetentionPolicy     monitor.RetentionPolicy `json:"monitor_retention_policy"`
	MonitorRetentionCustomDays int                     `json:"monitor_retention_custom_days"`
	MonitorStorageWarningBytes *int64                  `json:"monitor_storage_warning_bytes,omitempty"`
	MonitorStorageHardBytes    *int64                  `json:"monitor_storage_hard_bytes,omitempty"`
	MonitorBudgetMaxConcurrent *int                    `json:"monitor_budget_max_concurrent,omitempty"`
	MonitorBudgetDailyRequests *int64                  `json:"monitor_budget_daily_requests,omitempty"`
	MonitorBudgetDailyBytes    *int64                  `json:"monitor_budget_daily_bytes,omitempty"`
	MonitorBudgetResponseBytes *int64                  `json:"monitor_budget_response_bytes,omitempty"`
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
