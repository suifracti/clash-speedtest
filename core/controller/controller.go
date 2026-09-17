package controller

import (
	"context"
	"time"
)

// Controller abstracts external proxy core controllers (e.g. Mihomo External Controller REST API)
// or future standalone/bundled proxy cores.
//
// Crucial Design Constraint:
// Testing candidate nodes using our probe engine is strictly decoupled from the Controller.
// Testing must never switch the active proxy or alter user's live traffic.
// Controller is solely invoked when querying proxy groups or explicitly switching nodes
// either via user command or the Decision Policy engine.
type Controller interface {
	// GetVersion returns proxy core version and flavor information.
	GetVersion(ctx context.Context) (VersionInfo, error)

	// GetCapabilities returns the features supported by this controller instance.
	GetCapabilities(ctx context.Context) (Capabilities, error)

	// ListGroups returns all proxy selector/fallback/url-test groups managed by the core.
	ListGroups(ctx context.Context) ([]Group, error)

	// ListNodes returns candidate proxy nodes belonging to the specified group.
	// If group is empty, all proxy nodes are returned.
	ListNodes(ctx context.Context, group string) ([]Node, error)

	// GetCurrentSelection returns the name of the currently selected proxy in the specified group.
	GetCurrentSelection(ctx context.Context, group string) (string, error)

	// SelectNode updates the active proxy selection for the specified Selector group in the external core.
	// Note: This updates the Selector's active choice in Mihomo; by default existing user
	// connections are NOT terminated or interrupted.
	SelectNode(ctx context.Context, group string, nodeName string) error

	// TestDelay tests basic HTTP/TCP connectivity and delay to a target URL using the core's built-in probe.
	// Note: For deep AI, TTFB, and IPPure probing, our own Probe Engine is used instead.
	TestDelay(ctx context.Context, proxyName string, url string, timeout time.Duration) (time.Duration, error)
}

// Group represents a proxy group (e.g. Selector, Fallback, URLTest) in the proxy core.
type Group struct {
	Name string   `json:"name"`
	Type string   `json:"type"` // "Selector", "URLTest", "Fallback", "LoadBalance", etc.
	Now  string   `json:"now"`  // Currently selected active node name
	All  []string `json:"all"`  // List of all candidate node names in this group
}

// Node represents a proxy node known to the proxy core.
type Node struct {
	Name    string        `json:"name"`
	Type    string        `json:"type"` // "ss", "vmess", "vless", "hysteria2", "trojan", etc.
	UDP     bool          `json:"udp"`
	History []DelayRecord `json:"history,omitempty"`
}

// DelayRecord represents a historical delay probe recorded by the proxy core.
type DelayRecord struct {
	Time  time.Time `json:"time"`
	Delay int       `json:"delay"` // Milliseconds
}

// Capabilities describes the operational features supported by this controller.
type Capabilities struct {
	CanSelectNode       bool `json:"can_select_node"`
	CanTestDelay        bool `json:"can_test_delay"`
	CanStreamLogs       bool `json:"can_stream_logs"`
	CanListConnections  bool `json:"can_list_connections"`
	SupportsMemoryStats bool `json:"supports_memory_stats"`
}

// VersionInfo contains proxy core version and build details.
type VersionInfo struct {
	Version  string `json:"version"`
	Premium  bool   `json:"premium,omitempty"`
	CoreType string `json:"core_type"` // e.g. "mihomo", "clash.meta", "clash"
}
