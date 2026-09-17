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
