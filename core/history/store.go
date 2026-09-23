package history

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
)

type TestRun struct {
	ID          string           `json:"id"`
	AirportID   string           `json:"airport_id"`
	AirportName string           `json:"airport_name"`
	CreatedAt   time.Time        `json:"created_at"`
	Metrics     []string         `json:"metrics"`
	TotalNodes  int              `json:"total_nodes"`
	PassedNodes int              `json:"passed_nodes"`
	Results     []*RunNodeResult `json:"results"`
}

type LatencySample struct {
	Seq       int       `json:"seq"`
	Timestamp time.Time `json:"timestamp"`
	LatencyMs int64     `json:"latency_ms"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
}

type Ping0Info struct {
	IP        string `json:"ip"`
	Location  string `json:"location"`
	Country   string `json:"country"`
	ASN       string `json:"asn"`
	Org       string `json:"org"`
	IsIDC     bool   `json:"is_idc"`
	RiskScore int    `json:"risk_score"` // 0-100 (ping0 风控值)
}

type IPPureInfo struct {
	IP            string `json:"ip"`
	ASN           int    `json:"asn"`
	ASOrg         string `json:"as_org"`
	Country       string `json:"country"`
	CountryCode   string `json:"country_code"`
	City          string `json:"city"`
	FraudScore    int    `json:"fraud_score"`    // 0-100 (IPPure 系数)
	IsResidential bool   `json:"is_residential"` // true: 住宅, false: 机房
	IsBroadcast   bool   `json:"is_broadcast"`   // true: 广播IP, false: 原生IP
}

type IPInfo struct {
	IP          string      `json:"ip"`
	ASN         string      `json:"asn"`
	ISP         string      `json:"isp"`
	IPType      string      `json:"ip_type"`     // "住宅家宽", "机房IDC"
	OriginType  string      `json:"origin_type"` // "原生IP", "广播IP"
	RiskScore   int         `json:"risk_score"`  // 0-100 (ping0.cc 风控分)
	FraudScore  int         `json:"fraud_score"` // 0-100 (ippure.com 欺诈/纯净度系数)
	Location    string      `json:"location"`    // e.g. "新加坡 新加坡"
	Country     string      `json:"country"`
	CountryCode string      `json:"country_code"`
	Ping0       *Ping0Info  `json:"ping0,omitempty"`
	IPPure      *IPPureInfo `json:"ippure,omitempty"`
}

type StabilityInfo struct {
	TotalProbes   int      `json:"total_probes"`
	SuccessProbes int      `json:"success_probes"`
	StabilityRate float64  `json:"stability_rate"`        // e.g. 66.7
	Flapping      bool     `json:"flapping"`              // true if inconsistent results (偶发送中/负载均衡漂移)
	ExitIPs       []string `json:"exit_ips"`              // unique IPs detected across probe rounds
	BlockedIPs    []string `json:"blocked_ips,omitempty"` // IPs identified as blocked/CN by Google
	FlapReason    string   `json:"flap_reason,omitempty"`
	GoogleTTFBMs  int64    `json:"google_ttfb_ms"` // average Google API interaction latency
	GoogleMinTTFB int64    `json:"google_min_ttfb_ms"`
	GoogleMaxTTFB int64    `json:"google_max_ttfb_ms"`
	LatencyGrade  string   `json:"latency_grade,omitempty"` // "fast", "medium", "slow", "laggy"
}

type RunNodeResult struct {
	ProxyName         string          `json:"proxy_name"`
	ProxyType         string          `json:"proxy_type"`
	Server            string          `json:"server,omitempty"`
	Port              int             `json:"port,omitempty"`
	CountryCode       string          `json:"country_code,omitempty"`
	CountryFlag       string          `json:"country_flag,omitempty"`
	LatencyMs         int64           `json:"latency_ms"`
	JitterMs          int64           `json:"jitter_ms"`
	PacketLoss        float64         `json:"packet_loss"`
	LatencySamples    []LatencySample `json:"latency_samples,omitempty"`
	TimeoutCount      int             `json:"timeout_count,omitempty"`
	DownloadSpeedMBps float64         `json:"download_speed_mbps"`
	UploadSpeedMBps   float64         `json:"upload_speed_mbps"`
	DownloadError     string          `json:"download_error,omitempty"`
	UploadError       string          `json:"upload_error,omitempty"`
	AntigravityStatus string          `json:"antigravity_status,omitempty"`
	AntigravityDetail string          `json:"antigravity_detail,omitempty"`
	GoogleTTFBMs      int64           `json:"google_ttfb_ms,omitempty"`
	ExitCountry       string          `json:"exit_country,omitempty"`
	ExitCountryCode   string          `json:"exit_country_code,omitempty"`
	IPInfo            *IPInfo         `json:"ip_info,omitempty"`
	Stability         *StabilityInfo  `json:"stability,omitempty"`
}

type NodeTimelineItem struct {
	RunID             string         `json:"run_id"`
	AirportID         string         `json:"airport_id"`
	AirportName       string         `json:"airport_name"`
	CreatedAt         time.Time      `json:"created_at"`
	LatencyMs         int64          `json:"latency_ms"`
	JitterMs          int64          `json:"jitter_ms"`
	PacketLoss        float64        `json:"packet_loss"`
	DownloadSpeedMBps float64        `json:"download_speed_mbps"`
	UploadSpeedMBps   float64        `json:"upload_speed_mbps"`
	AntigravityStatus string         `json:"antigravity_status"`
	AntigravityDetail string         `json:"antigravity_detail"`
	GoogleTTFBMs      int64          `json:"google_ttfb_ms,omitempty"`
	ExitCountry       string         `json:"exit_country"`
	ExitCountryCode   string         `json:"exit_country_code"`
	IPInfo            *IPInfo        `json:"ip_info,omitempty"`
	Stability         *StabilityInfo `json:"stability,omitempty"`
}

type RunSummary struct {
	ID                        string    `json:"id"`
	AirportID                 string    `json:"airport_id"`
	AirportName               string    `json:"airport_name"`
	CreatedAt                 time.Time `json:"created_at"`
	TotalNodes                int       `json:"total_nodes"`
	PassedNodes               int       `json:"passed_nodes"`
	AvgLatencyMs              int64     `json:"avg_latency_ms"`
	AvgSpeedMBps              float64   `json:"avg_speed_mbps"`
	AntigravityAvailableCount int       `json:"antigravity_available_count"`
}

type RunComparison struct {
	BaseRunID   string            `json:"base_run_id"`
	TargetRunID string            `json:"target_run_id"`
	BaseTime    time.Time         `json:"base_time"`
	TargetTime  time.Time         `json:"target_time"`
	NodeDiffs   []*NodeDiff       `json:"node_diffs"`
	Summary     ComparisonSummary `json:"summary"`
}

type NodeDiff struct {
	ProxyName          string  `json:"proxy_name"`
	CountryCode        string  `json:"country_code"`
	CountryFlag        string  `json:"country_flag"`
	BaseLatencyMs      int64   `json:"base_latency_ms"`
	TargetLatencyMs    int64   `json:"target_latency_ms"`
	LatencyDeltaMs     int64   `json:"latency_delta_ms"` // Target - Base (negative = faster)
	BaseSpeedMBps      float64 `json:"base_speed_mbps"`
	TargetSpeedMBps    float64 `json:"target_speed_mbps"`
	SpeedDeltaMBps     float64 `json:"speed_delta_mbps"` // Target - Base (positive = faster)
	BaseAntigravity    string  `json:"base_antigravity"`
	TargetAntigravity  string  `json:"target_antigravity"`
	AntigravityChanged bool    `json:"antigravity_changed"`
	BaseIP             string  `json:"base_ip,omitempty"`
	TargetIP           string  `json:"target_ip,omitempty"`
	BaseRiskScore      int     `json:"base_risk_score,omitempty"`
	TargetRiskScore    int     `json:"target_risk_score,omitempty"`
	BaseFraudScore     int     `json:"base_fraud_score,omitempty"`
	TargetFraudScore   int     `json:"target_fraud_score,omitempty"`
	BaseOriginType     string  `json:"base_origin_type,omitempty"`
	TargetOriginType   string  `json:"target_origin_type,omitempty"`
	BaseFlapping       bool    `json:"base_flapping,omitempty"`
	TargetFlapping     bool    `json:"target_flapping,omitempty"`
	Status             string  `json:"status"` // "improved", "degraded", "unchanged", "new", "removed"
}

type ComparisonSummary struct {
	TotalCompared  int `json:"total_compared"`
	ImprovedCount  int `json:"improved_count"`
	DegradedCount  int `json:"degraded_count"`
	UnchangedCount int `json:"unchanged_count"`
	NewCount       int `json:"new_count"`
	RemovedCount   int `json:"removed_count"`
}

type AirportSummary struct {
	AirportID         string    `json:"airport_id"`
	AirportName       string    `json:"airport_name"`
	TotalRuns         int       `json:"total_runs"`
	EarliestTime      time.Time `json:"earliest_time"`
	LatestTime        time.Time `json:"latest_time"`
	DistinctNodeCount int       `json:"distinct_node_count"`
	AvgLatencyMs      int64     `json:"avg_latency_ms"`
	MaxSpeedMBps      float64   `json:"max_speed_mbps"`
}

type AirportTimelineMoment struct {
	RunID       string    `json:"run_id"`
	CreatedAt   time.Time `json:"created_at"`
	TotalNodes  int       `json:"total_nodes"`
	PassedNodes int       `json:"passed_nodes"`
}

type AirportNodePoint struct {
	RunID             string         `json:"run_id"`
	CreatedAt         time.Time      `json:"created_at"`
	LatencyMs         int64          `json:"latency_ms"`
	JitterMs          int64          `json:"jitter_ms"`
	PacketLoss        float64        `json:"packet_loss"`
	DownloadSpeedMBps float64        `json:"download_speed_mbps"`
	UploadSpeedMBps   float64        `json:"upload_speed_mbps"`
	AntigravityStatus string         `json:"antigravity_status"`
	AntigravityDetail string         `json:"antigravity_detail"`
	GoogleTTFBMs      int64          `json:"google_ttfb_ms,omitempty"`
	ExitCountry       string         `json:"exit_country"`
	ExitCountryCode   string         `json:"exit_country_code"`
	IPInfo            *IPInfo        `json:"ip_info,omitempty"`
	Stability         *StabilityInfo `json:"stability,omitempty"`
}

type AirportNodeHistory struct {
	ProxyName        string              `json:"proxy_name"`
	CountryCode      string              `json:"country_code"`
	CountryFlag      string              `json:"country_flag"`
	ProxyType        string              `json:"proxy_type"`
	Server           string              `json:"server,omitempty"`
	Port             int                 `json:"port,omitempty"`
	Points           []*AirportNodePoint `json:"points"`
	Latest           *AirportNodePoint   `json:"latest"`
	Earliest         *AirportNodePoint   `json:"earliest"`
	MinLatencyMs     int64               `json:"min_latency_ms"`
	MaxLatencyMs     int64               `json:"max_latency_ms"`
	AvgLatencyMs     int64               `json:"avg_latency_ms"`
	LatencyChangeMs  int64               `json:"latency_change_ms"`  // Latest - Earliest
	LatencyChangePct float64             `json:"latency_change_pct"` // (Latest - Earliest) / Earliest * 100
	AvgGoogleTTFB    int64               `json:"avg_google_ttfb_ms,omitempty"`
	LatestGoogleTTFB int64               `json:"latest_google_ttfb_ms,omitempty"`
	MaxSpeedMBps     float64             `json:"max_speed_mbps"`
	AvgSpeedMBps     float64             `json:"avg_speed_mbps"`
	LatestSpeedMBps  float64             `json:"latest_speed_mbps"`
	SpeedChangeMBps  float64             `json:"speed_change_mbps"`
	AvailableCount   int                 `json:"available_count"`
	TotalTests       int                 `json:"total_tests"`
	AvailableRate    float64             `json:"available_rate"`
	UniqueIPs        []string            `json:"unique_ips"`
}

type AirportHistory struct {
	AirportID         string                   `json:"airport_id"`
	AirportName       string                   `json:"airport_name"`
	TotalRuns         int                      `json:"total_runs"`
	EarliestTime      time.Time                `json:"earliest_time"`
	LatestTime        time.Time                `json:"latest_time"`
	DistinctNodeCount int                      `json:"distinct_node_count"`
	Moments           []*AirportTimelineMoment `json:"moments"`
	Nodes             []*AirportNodeHistory    `json:"nodes"`
	Runs              []*RunSummary            `json:"runs"`
}

type Store struct {
	dir       string
	legacyDir string
	mu        sync.RWMutex
	db        *DB
}

func DefaultHistoryDir() string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		return filepath.Join(home, ".clash-speedtest", "history")
	}
	return filepath.Join(".", ".clash-speedtest", "history")
}

func NewStore(dir string) (*Store, error) {
	return NewStoreWithLegacyDir(dir, "")
}

// NewStoreWithLegacyDir opens the SQLite history database in dir while keeping
// legacy JSON runs in legacyDir. A blank legacyDir preserves the historical
// behavior and stores JSON beside the database.
func NewStoreWithLegacyDir(dir, legacyDir string) (*Store, error) {
	if dir == "" {
		dir = DefaultHistoryDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create history dir %q: %w", dir, err)
	}
	if legacyDir == "" {
		legacyDir = dir
	}
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		return nil, fmt.Errorf("create legacy history dir %q: %w", legacyDir, err)
	}
	db, err := OpenDB(dir)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}
	return &Store{dir: dir, legacyDir: legacyDir, db: db}, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		err := s.db.Close()
		s.db = nil
		return err
	}
	return nil
}

func (s *Store) DB() *DB {
	return s.db
}

// SampleStore interface implementation delegating to SQLite DB

func (s *Store) SaveMonitorRun(ctx context.Context, run *monitor.MonitorRun) error {
	return s.db.SaveMonitorRun(ctx, run)
}

func (s *Store) UpdateMonitorRun(ctx context.Context, run *monitor.MonitorRun) error {
	return s.db.UpdateMonitorRun(ctx, run)
}

func (s *Store) MarkRunningMonitorRunsInterrupted(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("history store is not initialized")
	}
	return s.db.MarkRunningMonitorRunsInterrupted(ctx)
}

func (s *Store) SaveMonitorSamples(ctx context.Context, samples []*monitor.MonitorSample) error {
	return s.db.SaveMonitorSamples(ctx, samples)
}

func (s *Store) QueryMonitorRuns(ctx context.Context, jobID string, limit int) ([]*monitor.MonitorRun, error) {
	return s.db.QueryMonitorRuns(ctx, jobID, limit)
}

func (s *Store) QueryMonitorSamples(ctx context.Context, filter monitor.SampleFilter) ([]*monitor.MonitorSample, error) {
	return s.db.QueryMonitorSamples(ctx, filter)
}

func (s *Store) SaveMonitorJobDefinition(ctx context.Context, definition *monitor.MonitorJobDefinition) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("history store is not initialized")
	}
	return s.db.SaveMonitorJobDefinition(ctx, definition)
}

func (s *Store) UpdateMonitorJobSamplingTier(ctx context.Context, jobID string, tier monitor.SamplingTier, updatedAt time.Time) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("history store is not initialized")
	}
	return s.db.UpdateMonitorJobSamplingTier(ctx, jobID, tier, updatedAt)
}

func (s *Store) UpdateMonitorJobLaunchIntent(ctx context.Context, jobID string, resumeOnLaunch bool, desired monitor.JobState, updatedAt time.Time) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("history store is not initialized")
	}
	return s.db.UpdateMonitorJobLaunchIntent(ctx, jobID, resumeOnLaunch, desired, updatedAt)
}

func (s *Store) DeleteMonitorJobDefinition(ctx context.Context, jobID string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("history store is not initialized")
	}
	return s.db.DeleteMonitorJobDefinition(ctx, jobID)
}

func (s *Store) ListMonitorJobDefinitions(ctx context.Context) ([]*monitor.MonitorJobDefinition, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	return s.db.ListMonitorJobDefinitions(ctx)
}

func (s *Store) GetNodeTimelineSamples(ctx context.Context, nodeKey string, since time.Time) ([]*monitor.MonitorSample, error) {
	return s.db.GetNodeTimelineSamples(ctx, nodeKey, since)
}

func (s *Store) QueryMonitorSamplesCursor(ctx context.Context, filter monitor.CursorFilter) (*monitor.SampleCursorPage, error) {
	return s.db.QueryMonitorSamplesCursor(ctx, filter)
}

func (s *Store) GetDerivedStats(ctx context.Context, query monitor.StatsQuery) (*monitor.DerivedStats, error) {
	return s.db.GetDerivedStats(ctx, query)
}

func (s *Store) ApplyRetention(ctx context.Context, req monitor.RetentionRequest) (*monitor.RetentionResult, error) {
	return s.db.ApplyRetention(ctx, req)
}

// GetMonitorSampleFacets returns the distinct filter dimensions present in raw samples.
// Presentation-only read model; see DB.GetMonitorSampleFacets.
func (s *Store) GetMonitorSampleFacets(ctx context.Context, since, until time.Time, maxNodes, maxValues int) (*monitor.MonitorSampleFacets, error) {
	return s.db.GetMonitorSampleFacets(ctx, since, until, maxNodes, maxValues)
}

// SaveLatencyTest persists one on-demand workbench latency test and its raw samples.
func (s *Store) SaveLatencyTest(ctx context.Context, test *LatencyTest) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("history store is not initialized")
	}
	return s.db.SaveLatencyTest(ctx, test)
}

// QueryLatencyTests returns on-demand latency history scoped to one logical node.
func (s *Store) QueryLatencyTests(ctx context.Context, filter LatencyTestFilter) (*LatencyTestQueryResult, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	return s.db.QueryLatencyTests(ctx, filter)
}

// GetLatencyTest returns one on-demand latency test by its immutable attempt ID.
func (s *Store) GetLatencyTest(ctx context.Context, attemptID string) (*LatencyTest, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	return s.db.GetLatencyTest(ctx, attemptID)
}

// GetLatencyTestInWindow returns one on-demand latency test with raw samples
// restricted to the requested half-open observation window.
func (s *Store) GetLatencyTestInWindow(ctx context.Context, attemptID string, since, until *time.Time) (*LatencyTest, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	return s.db.GetLatencyTestInWindow(ctx, attemptID, since, until)
}

func (s *Store) ListNodeHistoryRevisions(ctx context.Context, profileID, nodeIdentityKey string) ([]NodeHistoryRevision, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	return s.db.ListNodeHistoryRevisions(ctx, profileID, nodeIdentityKey)
}

func (s *Store) CreateLatencyBatch(ctx context.Context, batch *LatencyBatch) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("history store is not initialized")
	}
	return s.db.CreateLatencyBatch(ctx, batch)
}

func (s *Store) UpdateLatencyBatchItem(ctx context.Context, item *LatencyBatchItem) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("history store is not initialized")
	}
	return s.db.UpdateLatencyBatchItem(ctx, item)
}

func (s *Store) UpdateLatencyBatchState(ctx context.Context, batchID, state string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("history store is not initialized")
	}
	return s.db.UpdateLatencyBatchState(ctx, batchID, state)
}

func (s *Store) GetLatencyBatch(ctx context.Context, batchID string) (*LatencyBatch, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	return s.db.GetLatencyBatch(ctx, batchID)
}

func (s *Store) FindLatencyBatchByRequestID(ctx context.Context, requestID string) (*LatencyBatch, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	return s.db.FindLatencyBatchByRequestID(ctx, requestID)
}

func (s *Store) ListLatencyBatches(ctx context.Context, limit int) ([]LatencyBatch, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	return s.db.ListLatencyBatches(ctx, limit)
}

func (s *Store) ListLatencyBatchesNeedingRecovery(ctx context.Context) ([]LatencyBatch, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	return s.db.ListLatencyBatchesNeedingRecovery(ctx)
}

// SetTestBatchFailAt injects a batch failure on batch n for testing partial retention semantics.
func (s *Store) SetTestBatchFailAt(n int) {
	if s.db != nil {
		s.db.SetTestBatchFailAt(n)
	}
}

func (s *Store) Dir() string {
	return s.dir
}

// LegacyDir returns the directory used by the compatibility JSON history
// format. Modern SQLite records always live in Dir().
func (s *Store) LegacyDir() string {
	if s == nil {
		return ""
	}
	if s.legacyDir == "" {
		return s.dir
	}
	return s.legacyDir
}

func (s *Store) Save(run *TestRun) (string, error) {
	if run == nil {
		return "", fmt.Errorf("run is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if run.ID == "" {
		run.ID = newRunID()
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now()
	}

	filePath := filepath.Join(s.LegacyDir(), run.ID+".json")
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal run %s: %w", run.ID, err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return "", fmt.Errorf("write run file %s: %w", filePath, err)
	}
	return run.ID, nil
}

func (s *Store) Get(id string) (*TestRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filePath := filepath.Join(s.LegacyDir(), id+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read run %s: %w", id, err)
	}

	var run TestRun
	if err := json.Unmarshal(data, &run); err != nil {
		return nil, fmt.Errorf("unmarshal run %s: %w", id, err)
	}
	return &run, nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filePath := filepath.Join(s.LegacyDir(), id+".json")
	return os.Remove(filePath)
}

func (s *Store) List() ([]*RunSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.LegacyDir())
	if err != nil {
		return nil, fmt.Errorf("read history dir: %w", err)
	}

	summaries := make([]*RunSummary, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		filePath := filepath.Join(s.LegacyDir(), entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var run TestRun
		if err := json.Unmarshal(data, &run); err != nil {
			continue
		}

		summary := &RunSummary{
			ID:          run.ID,
			AirportID:   run.AirportID,
			AirportName: run.AirportName,
			CreatedAt:   run.CreatedAt,
			TotalNodes:  run.TotalNodes,
			PassedNodes: run.PassedNodes,
		}

		var totalLatency int64
		var latencyCount int64
		var totalSpeed float64
		var speedCount int
		for _, r := range run.Results {
			if r.LatencyMs > 0 {
				totalLatency += r.LatencyMs
				latencyCount++
			}
			if r.DownloadSpeedMBps > 0 {
				totalSpeed += r.DownloadSpeedMBps
				speedCount++
			}
			if r.AntigravityStatus == "available" {
				summary.AntigravityAvailableCount++
			}
		}
		if latencyCount > 0 {
			summary.AvgLatencyMs = totalLatency / latencyCount
		}
		if speedCount > 0 {
			summary.AvgSpeedMBps = math.Round((totalSpeed/float64(speedCount))*100) / 100
		}

		summaries = append(summaries, summary)
	}

	// Sort newest first
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].CreatedAt.After(summaries[j].CreatedAt)
	})

	return summaries, nil
}

// GetAllRuns loads and returns all full test runs, sorted newest first.
func (s *Store) GetAllRuns() ([]*TestRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.LegacyDir())
	if err != nil {
		return nil, fmt.Errorf("read history dir: %w", err)
	}

	runs := make([]*TestRun, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		filePath := filepath.Join(s.LegacyDir(), entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		var run TestRun
		if err := json.Unmarshal(data, &run); err != nil {
			continue
		}
		runs = append(runs, &run)
	}

	sort.Slice(runs, func(i, j int) bool {
		return runs[i].CreatedAt.After(runs[j].CreatedAt)
	})
	return runs, nil
}

func (s *Store) Compare(baseID, targetID string) (*RunComparison, error) {
	baseRun, err := s.Get(baseID)
	if err != nil {
		return nil, fmt.Errorf("load base run: %w", err)
	}
	targetRun, err := s.Get(targetID)
	if err != nil {
		return nil, fmt.Errorf("load target run: %w", err)
	}

	baseMap := make(map[string]*RunNodeResult, len(baseRun.Results))
	for _, r := range baseRun.Results {
		baseMap[r.ProxyName] = r
	}

	targetMap := make(map[string]*RunNodeResult, len(targetRun.Results))
	for _, r := range targetRun.Results {
		targetMap[r.ProxyName] = r
	}

	allNamesMap := make(map[string]struct{})
	for name := range baseMap {
		allNamesMap[name] = struct{}{}
	}
	for name := range targetMap {
		allNamesMap[name] = struct{}{}
	}

	allNames := make([]string, 0, len(allNamesMap))
	for name := range allNamesMap {
		allNames = append(allNames, name)
	}
	sort.Strings(allNames)

	diffs := make([]*NodeDiff, 0, len(allNames))
	var summary ComparisonSummary

	for _, name := range allNames {
		b, hasBase := baseMap[name]
		t, hasTarget := targetMap[name]

		diff := &NodeDiff{
			ProxyName: name,
		}

		if !hasBase && hasTarget {
			// New node
			diff.CountryCode = t.CountryCode
			diff.CountryFlag = t.CountryFlag
			diff.TargetLatencyMs = t.LatencyMs
			diff.TargetSpeedMBps = t.DownloadSpeedMBps
			diff.TargetAntigravity = t.AntigravityStatus
			if t.IPInfo != nil {
				diff.TargetIP = t.IPInfo.IP
				diff.TargetRiskScore = t.IPInfo.RiskScore
				diff.TargetFraudScore = t.IPInfo.FraudScore
				diff.TargetOriginType = t.IPInfo.OriginType
			}
			if t.Stability != nil {
				diff.TargetFlapping = t.Stability.Flapping
			}
			diff.Status = "new"
			summary.NewCount++
		} else if hasBase && !hasTarget {
			// Removed node
			diff.CountryCode = b.CountryCode
			diff.CountryFlag = b.CountryFlag
			diff.BaseLatencyMs = b.LatencyMs
			diff.BaseSpeedMBps = b.DownloadSpeedMBps
			diff.BaseAntigravity = b.AntigravityStatus
			if b.IPInfo != nil {
				diff.BaseIP = b.IPInfo.IP
				diff.BaseRiskScore = b.IPInfo.RiskScore
				diff.BaseFraudScore = b.IPInfo.FraudScore
				diff.BaseOriginType = b.IPInfo.OriginType
			}
			if b.Stability != nil {
				diff.BaseFlapping = b.Stability.Flapping
			}
			diff.Status = "removed"
			summary.RemovedCount++
		} else {
			// Both exist
			summary.TotalCompared++
			diff.CountryCode = t.CountryCode
			diff.CountryFlag = t.CountryFlag
			diff.BaseLatencyMs = b.LatencyMs
			diff.TargetLatencyMs = t.LatencyMs
			diff.LatencyDeltaMs = t.LatencyMs - b.LatencyMs
			diff.BaseSpeedMBps = b.DownloadSpeedMBps
			diff.TargetSpeedMBps = t.DownloadSpeedMBps
			diff.SpeedDeltaMBps = math.Round((t.DownloadSpeedMBps-b.DownloadSpeedMBps)*100) / 100
			diff.BaseAntigravity = b.AntigravityStatus
			diff.TargetAntigravity = t.AntigravityStatus
			diff.AntigravityChanged = b.AntigravityStatus != t.AntigravityStatus

			if b.IPInfo != nil {
				diff.BaseIP = b.IPInfo.IP
				diff.BaseRiskScore = b.IPInfo.RiskScore
				diff.BaseFraudScore = b.IPInfo.FraudScore
				diff.BaseOriginType = b.IPInfo.OriginType
			}
			if t.IPInfo != nil {
				diff.TargetIP = t.IPInfo.IP
				diff.TargetRiskScore = t.IPInfo.RiskScore
				diff.TargetFraudScore = t.IPInfo.FraudScore
				diff.TargetOriginType = t.IPInfo.OriginType
			}
			if b.Stability != nil {
				diff.BaseFlapping = b.Stability.Flapping
			}
			if t.Stability != nil {
				diff.TargetFlapping = t.Stability.Flapping
			}

			// Determine status: improved, degraded, or unchanged
			speedBetter := diff.SpeedDeltaMBps > 1.0
			speedWorse := diff.SpeedDeltaMBps < -1.0
			latencyBetter := b.LatencyMs > 0 && t.LatencyMs > 0 && diff.LatencyDeltaMs < -15
			latencyWorse := b.LatencyMs > 0 && t.LatencyMs > 0 && diff.LatencyDeltaMs > 15
			agBetter := (b.AntigravityStatus != "available" && t.AntigravityStatus == "available") || (diff.BaseFlapping && !diff.TargetFlapping && t.AntigravityStatus == "available")
			agWorse := (b.AntigravityStatus == "available" && t.AntigravityStatus != "available") || (!diff.BaseFlapping && diff.TargetFlapping)

			if agBetter || (speedBetter && !latencyWorse && !agWorse) || (latencyBetter && !speedWorse && !agWorse) {
				diff.Status = "improved"
				summary.ImprovedCount++
			} else if agWorse || speedWorse || latencyWorse {
				diff.Status = "degraded"
				summary.DegradedCount++
			} else {
				diff.Status = "unchanged"
				summary.UnchangedCount++
			}
		}
		diffs = append(diffs, diff)
	}

	return &RunComparison{
		BaseRunID:   baseID,
		TargetRunID: targetID,
		BaseTime:    baseRun.CreatedAt,
		TargetTime:  targetRun.CreatedAt,
		NodeDiffs:   diffs,
		Summary:     summary,
	}, nil
}

// GetNodeTimeline retrieves all historical test records for a specific node name across all saved runs.
func (s *Store) GetNodeTimeline(nodeName string) ([]*NodeTimelineItem, error) {
	runs, err := s.GetAllRuns()
	if err != nil {
		return nil, err
	}

	items := make([]*NodeTimelineItem, 0)
	for _, run := range runs {
		for _, r := range run.Results {
			if r.ProxyName == nodeName {
				items = append(items, &NodeTimelineItem{
					RunID:             run.ID,
					AirportID:         run.AirportID,
					AirportName:       run.AirportName,
					CreatedAt:         run.CreatedAt,
					LatencyMs:         r.LatencyMs,
					JitterMs:          r.JitterMs,
					PacketLoss:        r.PacketLoss,
					DownloadSpeedMBps: r.DownloadSpeedMBps,
					UploadSpeedMBps:   r.UploadSpeedMBps,
					AntigravityStatus: r.AntigravityStatus,
					AntigravityDetail: r.AntigravityDetail,
					GoogleTTFBMs:      r.GoogleTTFBMs,
					ExitCountry:       r.ExitCountry,
					ExitCountryCode:   r.ExitCountryCode,
					IPInfo:            r.IPInfo,
					Stability:         r.Stability,
				})
				break
			}
		}
	}

	// Sort chronological (oldest first, so charts display from left to right)
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})

	return items, nil
}

// ListAllDistinctNodes returns a sorted list of all unique node names present across all history runs.
func (s *Store) ListAllDistinctNodes() ([]string, error) {
	runs, err := s.GetAllRuns()
	if err != nil {
		return nil, err
	}

	set := make(map[string]struct{})
	for _, run := range runs {
		for _, r := range run.Results {
			if r.ProxyName != "" {
				set[r.ProxyName] = struct{}{}
			}
		}
	}

	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// ListHistoryAirports aggregates all saved runs by Airport, returning summary info for each airport.
func (s *Store) ListHistoryAirports() ([]*AirportSummary, error) {
	runs, err := s.GetAllRuns()
	if err != nil {
		return nil, err
	}

	type airportGroup struct {
		id           string
		name         string
		runs         int
		earliest     time.Time
		latest       time.Time
		nodeSet      map[string]struct{}
		totalLatency int64
		latencyCount int64
		maxSpeed     float64
	}

	groups := make(map[string]*airportGroup)
	for _, run := range runs {
		key := run.AirportID
		if key == "" {
			key = run.AirportName
		}
		if key == "" {
			key = "default"
		}

		g, ok := groups[key]
		if !ok {
			name := run.AirportName
			if name == "" {
				name = "未命名机场"
			}
			g = &airportGroup{
				id:       run.AirportID,
				name:     name,
				earliest: run.CreatedAt,
				latest:   run.CreatedAt,
				nodeSet:  make(map[string]struct{}),
			}
			groups[key] = g
		}

		g.runs++
		if run.CreatedAt.Before(g.earliest) {
			g.earliest = run.CreatedAt
		}
		if run.CreatedAt.After(g.latest) {
			g.latest = run.CreatedAt
		}

		for _, r := range run.Results {
			if r.ProxyName != "" {
				g.nodeSet[r.ProxyName] = struct{}{}
			}
			if r.LatencyMs > 0 {
				g.totalLatency += r.LatencyMs
				g.latencyCount++
			}
			if r.DownloadSpeedMBps > g.maxSpeed {
				g.maxSpeed = r.DownloadSpeedMBps
			}
		}
	}

	summaries := make([]*AirportSummary, 0, len(groups))
	for _, g := range groups {
		var avgLat int64
		if g.latencyCount > 0 {
			avgLat = g.totalLatency / g.latencyCount
		}
		summaries = append(summaries, &AirportSummary{
			AirportID:         g.id,
			AirportName:       g.name,
			TotalRuns:         g.runs,
			EarliestTime:      g.earliest,
			LatestTime:        g.latest,
			DistinctNodeCount: len(g.nodeSet),
			AvgLatencyMs:      avgLat,
			MaxSpeedMBps:      g.maxSpeed,
		})
	}

	// Sort newest test first
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].LatestTime.After(summaries[j].LatestTime)
	})

	return summaries, nil
}

// GetAirportHistory consolidates all historical test runs belonging to the specified airport.
func (s *Store) GetAirportHistory(airportIDOrName string) (*AirportHistory, error) {
	runs, err := s.GetAllRuns()
	if err != nil {
		return nil, err
	}

	var matchedRuns []*TestRun
	for _, run := range runs {
		if airportIDOrName == "" || run.AirportID == airportIDOrName || run.AirportName == airportIDOrName {
			matchedRuns = append(matchedRuns, run)
		}
	}

	if airportIDOrName == "" && len(matchedRuns) > 0 {
		targetAirportID := matchedRuns[0].AirportID
		targetAirportName := matchedRuns[0].AirportName
		var narrowed []*TestRun
		for _, r := range matchedRuns {
			if (targetAirportID != "" && r.AirportID == targetAirportID) || (r.AirportName == targetAirportName) {
				narrowed = append(narrowed, r)
			}
		}
		matchedRuns = narrowed
	}

	if len(matchedRuns) == 0 {
		return &AirportHistory{
			AirportID:   airportIDOrName,
			AirportName: airportIDOrName,
			Moments:     []*AirportTimelineMoment{},
			Nodes:       []*AirportNodeHistory{},
			Runs:        []*RunSummary{},
		}, nil
	}

	// Sort matched runs chronologically (oldest first)
	sort.Slice(matchedRuns, func(i, j int) bool {
		return matchedRuns[i].CreatedAt.Before(matchedRuns[j].CreatedAt)
	})

	airportID := matchedRuns[len(matchedRuns)-1].AirportID
	airportName := matchedRuns[len(matchedRuns)-1].AirportName
	if airportName == "" {
		airportName = "未命名机场"
	}

	moments := make([]*AirportTimelineMoment, 0, len(matchedRuns))
	rawSummaries := make([]*RunSummary, 0, len(matchedRuns))

	nodeMap := make(map[string]*AirportNodeHistory)
	nodeOrder := make([]string, 0)

	for _, run := range matchedRuns {
		moments = append(moments, &AirportTimelineMoment{
			RunID:       run.ID,
			CreatedAt:   run.CreatedAt,
			TotalNodes:  run.TotalNodes,
			PassedNodes: run.PassedNodes,
		})

		rawSummaries = append(rawSummaries, &RunSummary{
			ID:          run.ID,
			AirportID:   run.AirportID,
			AirportName: run.AirportName,
			CreatedAt:   run.CreatedAt,
			TotalNodes:  run.TotalNodes,
			PassedNodes: run.PassedNodes,
		})

		for _, r := range run.Results {
			if r.ProxyName == "" {
				continue
			}

			hist, exists := nodeMap[r.ProxyName]
			if !exists {
				hist = &AirportNodeHistory{
					ProxyName:   r.ProxyName,
					CountryCode: r.CountryCode,
					CountryFlag: r.CountryFlag,
					ProxyType:   r.ProxyType,
					Server:      r.Server,
					Port:        r.Port,
					Points:      make([]*AirportNodePoint, 0),
				}
				nodeMap[r.ProxyName] = hist
				nodeOrder = append(nodeOrder, r.ProxyName)
			} else {
				if hist.CountryCode == "" && r.CountryCode != "" {
					hist.CountryCode = r.CountryCode
					hist.CountryFlag = r.CountryFlag
				}
				if hist.ProxyType == "" && r.ProxyType != "" {
					hist.ProxyType = r.ProxyType
				}
				if hist.Server == "" && r.Server != "" {
					hist.Server = r.Server
					hist.Port = r.Port
				}
			}

			hist.Points = append(hist.Points, &AirportNodePoint{
				RunID:             run.ID,
				CreatedAt:         run.CreatedAt,
				LatencyMs:         r.LatencyMs,
				JitterMs:          r.JitterMs,
				PacketLoss:        r.PacketLoss,
				DownloadSpeedMBps: r.DownloadSpeedMBps,
				UploadSpeedMBps:   r.UploadSpeedMBps,
				AntigravityStatus: r.AntigravityStatus,
				AntigravityDetail: r.AntigravityDetail,
				GoogleTTFBMs:      r.GoogleTTFBMs,
				ExitCountry:       r.ExitCountry,
				ExitCountryCode:   r.ExitCountryCode,
				IPInfo:            r.IPInfo,
				Stability:         r.Stability,
			})
		}
	}

	nodes := make([]*AirportNodeHistory, 0, len(nodeOrder))
	for _, name := range nodeOrder {
		hist := nodeMap[name]
		hist.TotalTests = len(hist.Points)
		if len(hist.Points) > 0 {
			hist.Earliest = hist.Points[0]
			hist.Latest = hist.Points[len(hist.Points)-1]
			hist.LatestSpeedMBps = hist.Latest.DownloadSpeedMBps
			hist.LatestGoogleTTFB = hist.Latest.GoogleTTFBMs

			var sumLat int64
			var countLat int64
			var minLat int64 = 999999
			var maxLat int64 = 0

			var sumSpeed float64
			var countSpeed int
			var maxSpeed float64 = 0

			var sumTTFB int64
			var countTTFB int64

			ipMap := make(map[string]struct{})

			for _, pt := range hist.Points {
				if pt.LatencyMs > 0 {
					sumLat += pt.LatencyMs
					countLat++
					if pt.LatencyMs < minLat {
						minLat = pt.LatencyMs
					}
					if pt.LatencyMs > maxLat {
						maxLat = pt.LatencyMs
					}
				}
				if pt.DownloadSpeedMBps > 0 {
					sumSpeed += pt.DownloadSpeedMBps
					countSpeed++
					if pt.DownloadSpeedMBps > maxSpeed {
						maxSpeed = pt.DownloadSpeedMBps
					}
				}
				if pt.GoogleTTFBMs > 0 {
					sumTTFB += pt.GoogleTTFBMs
					countTTFB++
				}
				if pt.AntigravityStatus == "available" {
					hist.AvailableCount++
				}
				if pt.IPInfo != nil && pt.IPInfo.IP != "" {
					ipMap[pt.IPInfo.IP] = struct{}{}
				}
			}

			if countLat > 0 {
				hist.AvgLatencyMs = sumLat / countLat
				hist.MinLatencyMs = minLat
				hist.MaxLatencyMs = maxLat
			}
			if countSpeed > 0 {
				hist.AvgSpeedMBps = math.Round((sumSpeed/float64(countSpeed))*100) / 100
				hist.MaxSpeedMBps = maxSpeed
			}
			if countTTFB > 0 {
				hist.AvgGoogleTTFB = sumTTFB / countTTFB
			}
			if hist.TotalTests > 0 {
				hist.AvailableRate = math.Round((float64(hist.AvailableCount)/float64(hist.TotalTests))*1000) / 10
			}

			if hist.Earliest.LatencyMs > 0 && hist.Latest.LatencyMs > 0 {
				hist.LatencyChangeMs = hist.Latest.LatencyMs - hist.Earliest.LatencyMs
				hist.LatencyChangePct = math.Round((float64(hist.LatencyChangeMs)/float64(hist.Earliest.LatencyMs))*1000) / 10
			}

			hist.SpeedChangeMBps = math.Round((hist.Latest.DownloadSpeedMBps-hist.Earliest.DownloadSpeedMBps)*100) / 100

			uniqueIPs := make([]string, 0, len(ipMap))
			for ip := range ipMap {
				uniqueIPs = append(uniqueIPs, ip)
			}
			sort.Strings(uniqueIPs)
			hist.UniqueIPs = uniqueIPs
		}
		nodes = append(nodes, hist)
	}

	return &AirportHistory{
		AirportID:         airportID,
		AirportName:       airportName,
		TotalRuns:         len(matchedRuns),
		EarliestTime:      matchedRuns[0].CreatedAt,
		LatestTime:        matchedRuns[len(matchedRuns)-1].CreatedAt,
		DistinctNodeCount: len(nodes),
		Moments:           moments,
		Nodes:             nodes,
		Runs:              rawSummaries,
	}, nil
}

func newRunID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%s-%s", time.Now().Format("20060102-150405"), hex.EncodeToString(b[:]))
}
