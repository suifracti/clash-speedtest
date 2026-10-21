// Package autopick defines the durable configuration and report shape of the
// scheduled "auto pick" task. The selection rules follow the same latency and
// unlock evidence used by the workbench; nothing here performs network I/O.
package autopick

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Schedule kinds. Cron expressions are deliberately not supported.
const (
	Schedule6h    = "6h"
	Schedule12h   = "12h"
	ScheduleDaily = "daily"
)

// Report status values.
const (
	StatusSelected     = "selected"
	StatusKeptPrevious = "kept_previous"
	StatusFailed       = "failed"
)

// Purpose group names, matching the "现在用哪个" page.
const (
	PurposeDaily     = "日常"
	PurposeStreaming = "流媒体"
	PurposeAI        = "AI"
)

// Defaults for the two selection rules.
const (
	DefaultMinResponseRate = 90.0
	DefaultMaxLatencyMS    = 800
	DefaultSampleCount     = 6
	RequestWarningThreshold = 500
)

// GroupServices are the representative unlock services of each optional group.
// A node must pass every service in a group to be listed in that group.
var GroupServices = map[string][]string{
	PurposeAI:        {"chatgpt_unlock", "claude_unlock", "gemini_unlock"},
	PurposeStreaming: {"netflix_unlock", "youtube_premium", "disney_unlock"},
}

// Config is the user-visible definition of the auto-pick task.
type Config struct {
	Enabled          bool       `json:"enabled"`
	Schedule         string     `json:"schedule"`
	DailyHour        int        `json:"daily_hour"`
	AirportIDs       []string   `json:"airport_ids"`
	RegionFilter     string     `json:"region_filter,omitempty"`
	IncludeAI        bool       `json:"include_ai"`
	IncludeStreaming bool       `json:"include_streaming"`
	MinResponseRate  float64    `json:"min_response_rate"`
	MaxLatencyMS     int        `json:"max_latency_ms"`
	SampleCount      int        `json:"sample_count"`
	LastRunAt        *time.Time `json:"last_run_at,omitempty"`
	NextRunAt        *time.Time `json:"next_run_at,omitempty"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// Normalize fills defaults and canonicalizes fields without validating scope.
func (c *Config) Normalize() {
	c.Schedule = strings.ToLower(strings.TrimSpace(c.Schedule))
	switch c.Schedule {
	case Schedule6h, Schedule12h, ScheduleDaily:
	default:
		c.Schedule = Schedule6h
	}
	if c.DailyHour < 0 || c.DailyHour > 23 {
		c.DailyHour = 6
	}
	if c.MinResponseRate == 0 {
		c.MinResponseRate = DefaultMinResponseRate
	}
	if c.MaxLatencyMS == 0 {
		c.MaxLatencyMS = DefaultMaxLatencyMS
	}
	if c.SampleCount == 0 {
		c.SampleCount = DefaultSampleCount
	}
	seen := make(map[string]bool, len(c.AirportIDs))
	ids := make([]string, 0, len(c.AirportIDs))
	for _, id := range c.AirportIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	c.AirportIDs = ids
	c.RegionFilter = strings.TrimSpace(c.RegionFilter)
}

// Validate rejects configurations that cannot run.
func (c Config) Validate() error {
	if c.Schedule != Schedule6h && c.Schedule != Schedule12h && c.Schedule != ScheduleDaily {
		return errors.New("运行时间只能是每 6 小时、每 12 小时或每天固定时间")
	}
	if c.DailyHour < 0 || c.DailyHour > 23 {
		return errors.New("每天运行的小时必须在 0 到 23 之间")
	}
	if len(c.AirportIDs) == 0 {
		return errors.New("请至少选择一个机场")
	}
	if c.MinResponseRate < 50 || c.MinResponseRate > 100 {
		return errors.New("响应率下限必须在 50% 到 100% 之间")
	}
	if c.MaxLatencyMS < 100 || c.MaxLatencyMS > 5000 {
		return errors.New("稳定延迟上限必须在 100 ms 到 5000 ms 之间")
	}
	if c.SampleCount < 1 || c.SampleCount > 20 {
		return errors.New("每个节点的采样次数必须在 1 到 20 之间")
	}
	return nil
}

// Projects returns how many probe projects one node costs in one round.
func (c Config) Projects() int {
	projects := 1 // latency is mandatory
	if c.IncludeAI {
		projects += len(GroupServices[PurposeAI])
	}
	if c.IncludeStreaming {
		projects += len(GroupServices[PurposeStreaming])
	}
	return projects
}

// EstimateRequests is the UI estimate: nodes × projects.
func (c Config) EstimateRequests(nodes int) int {
	if nodes <= 0 {
		return 0
	}
	return nodes * c.Projects()
}

// NextRunAfter returns the next grid-aligned run time strictly after now.
// Grids are anchored at local midnight so "每 6 小时" always means 00/06/12/18.
func NextRunAfter(now time.Time, cfg Config) time.Time {
	local := now.In(time.Local)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.Local)
	hours := []int{0, 6, 12, 18}
	switch cfg.Schedule {
	case Schedule12h:
		hours = []int{0, 12}
	case ScheduleDaily:
		hours = []int{cfg.DailyHour}
	}
	for offset := 0; offset <= 1; offset++ {
		for _, hour := range hours {
			candidate := day.AddDate(0, 0, offset).Add(time.Duration(hour) * time.Hour)
			if candidate.After(local) {
				return candidate
			}
		}
	}
	return local.Add(24 * time.Hour)
}

// Node is one measured node in a report.
type Node struct {
	DisplayName       string            `json:"display_name"`
	ProfileID         string            `json:"profile_id"`
	AirportID         string            `json:"airport_id"`
	AirportName       string            `json:"airport_name,omitempty"`
	NodeKey           string            `json:"node_key"`
	NodeIdentityKey   string            `json:"node_identity_key"`
	ConfigRevisionKey string            `json:"config_revision_key"`
	LatencyMS         int64             `json:"latency_ms"`
	ResponseRate      float64           `json:"response_rate"`
	Samples           int               `json:"samples"`
	SuccessSamples    int               `json:"success_samples"`
	Purposes          []string          `json:"purposes"`
	Services          map[string]string `json:"services,omitempty"`
	ExcludedReason    string            `json:"excluded_reason,omitempty"`
}

// Report is one auto-pick round. It is persisted as a JSON file so it can be
// listed under 检测记录 → 报告 without a schema migration.
type Report struct {
	ReportID         string         `json:"report_id"`
	CreatedAt        time.Time      `json:"created_at"`
	Trigger          string         `json:"trigger"` // scheduled | manual
	Status           string         `json:"status"`
	Summary          string         `json:"summary"`
	KeptPrevious     bool           `json:"kept_previous"`
	PreviousReportID string         `json:"previous_report_id,omitempty"`
	PreviousAt       *time.Time     `json:"previous_at,omitempty"`
	BatchID          string         `json:"batch_id,omitempty"`
	Scope            Scope          `json:"scope"`
	Rules            Rules          `json:"rules"`
	Measured         int            `json:"measured"`
	LatencyPassed    int            `json:"latency_passed"`
	SelectedCount    int            `json:"selected_count"`
	GroupCounts      map[string]int `json:"group_counts"`
	Nodes            []Node         `json:"nodes"`
	Notes            []string       `json:"notes,omitempty"`
	Subscription     string         `json:"subscription_path,omitempty"`
}

// Scope records what the round covered.
type Scope struct {
	AirportIDs   []string `json:"airport_ids"`
	AirportNames []string `json:"airport_names,omitempty"`
	RegionFilter string   `json:"region_filter,omitempty"`
}

// Rules records the two selection rules actually applied.
type Rules struct {
	MinResponseRate  float64  `json:"min_response_rate"`
	MaxLatencyMS     int      `json:"max_latency_ms"`
	Groups           []string `json:"groups,omitempty"`
	SampleCount      int      `json:"sample_count"`
}

// ReportMeta is the list projection used by 检测记录 → 报告.
type ReportMeta struct {
	ReportID       string    `json:"report_id"`
	CreatedAt      time.Time `json:"created_at"`
	Trigger        string    `json:"trigger"`
	Status         string    `json:"status"`
	Summary        string    `json:"summary"`
	SelectedCount  int       `json:"selected_count"`
	DailyCount     int       `json:"daily_count"`
	StreamingCount int       `json:"streaming_count"`
	AICount        int       `json:"ai_count"`
	KeptPrevious   bool      `json:"kept_previous"`
}

// Meta projects a report for list views.
func (r Report) Meta() ReportMeta {
	return ReportMeta{
		ReportID: r.ReportID, CreatedAt: r.CreatedAt, Trigger: r.Trigger, Status: r.Status, Summary: r.Summary,
		SelectedCount: r.SelectedCount, DailyCount: r.GroupCounts[PurposeDaily],
		StreamingCount: r.GroupCounts[PurposeStreaming], AICount: r.GroupCounts[PurposeAI], KeptPrevious: r.KeptPrevious,
	}
}

// SummaryLine builds "选中 18 个 · 日常 12／流媒体 6／AI 4 · 10/22 06:00".
func SummaryLine(report Report) string {
	at := report.CreatedAt.In(time.Local).Format("01/02 15:04")
	groups := []string{}
	for _, purpose := range []string{PurposeDaily, PurposeStreaming, PurposeAI} {
		if count, ok := report.GroupCounts[purpose]; ok {
			groups = append(groups, fmt.Sprintf("%s %d", purpose, count))
		}
	}
	groupText := ""
	if len(groups) > 0 {
		groupText = " · " + strings.Join(groups, "／")
	}
	if report.KeptPrevious {
		return fmt.Sprintf("沿用上次结果 · %s", at)
	}
	if report.SelectedCount == 0 {
		return fmt.Sprintf("未选中节点 · %s", at)
	}
	return fmt.Sprintf("选中 %d 个%s · %s", report.SelectedCount, groupText, at)
}
