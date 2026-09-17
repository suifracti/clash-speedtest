package history

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
	_ "modernc.org/sqlite"
)

const ddlSchema = `
CREATE TABLE IF NOT EXISTS monitor_runs (
    run_id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL,
    scheduled_at DATETIME NOT NULL,
    started_at DATETIME NOT NULL,
    finished_at DATETIME,
    status TEXT NOT NULL,
    total_nodes INTEGER NOT NULL DEFAULT 0,
    success_nodes INTEGER NOT NULL DEFAULT 0,
    failed_nodes INTEGER NOT NULL DEFAULT 0,
    error_message TEXT
);

CREATE TABLE IF NOT EXISTS monitor_samples (
    sample_id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    node_key TEXT NOT NULL,
    node_identity_key TEXT NOT NULL DEFAULT '',
    config_revision_key TEXT NOT NULL DEFAULT '',
    profile_id TEXT NOT NULL,
    display_name_snapshot TEXT NOT NULL,
    probe_type TEXT NOT NULL,
    target TEXT NOT NULL,
    timestamp DATETIME NOT NULL,
    success INTEGER NOT NULL,
    latency_ms INTEGER NOT NULL DEFAULT 0,
    ttfb_ms INTEGER NOT NULL DEFAULT 0,
    error_class TEXT NOT NULL DEFAULT 'none',
    error_detail TEXT,
    exit_ip TEXT,
    exit_region TEXT,
    metadata_json TEXT
);

CREATE INDEX IF NOT EXISTS idx_samples_run_id ON monitor_samples(run_id);
CREATE INDEX IF NOT EXISTS idx_samples_node_key_time ON monitor_samples(node_key, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_samples_timestamp ON monitor_samples(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_runs_job_id_time ON monitor_runs(job_id, scheduled_at DESC);
`

// DB manages the SQLite single-writer connection pool with WAL mode enabled.
type DB struct {
	db *sql.DB
	mu sync.Mutex // Write lock guaranteeing strictly serialized single-writer transactions
}

// OpenDB opens (or creates) the SQLite database in the given directory and executes migrations.
func OpenDB(dir string) (*DB, error) {
	dbPath := filepath.Join(dir, "history.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db at %s: %w", dbPath, err)
	}

	// Pragmas for WAL mode, high concurrency reading, and safe busy timeout
	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA busy_timeout = 5000;",
		"PRAGMA synchronous = NORMAL;",
		"PRAGMA foreign_keys = ON;",
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("execute pragma %q: %w", pragma, err)
		}
	}

	if _, err := db.Exec(ddlSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize sqlite schema: %w", err)
	}

	// Run idempotent schema migrations to support upgrading from previous DB schema revisions
	if err := migrateSchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate sqlite schema: %w", err)
	}

	return &DB{db: db}, nil
}

// migrateSchema performs idempotent, backward-compatible schema updates.
// TODO(schema): Introduce a formal schema version framework in a future milestone (N-01).
func migrateSchema(db *sql.DB) error {
	rows, err := db.Query("PRAGMA table_info(monitor_samples);")
	if err != nil {
		return fmt.Errorf("inspect monitor_samples table_info: %w", err)
	}
	defer rows.Close()

	hasNodeIdentity := false
	hasConfigRevision := false

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("scan pragma table_info: %w", err)
		}
		if strings.EqualFold(name, "node_identity_key") {
			hasNodeIdentity = true
		}
		if strings.EqualFold(name, "config_revision_key") {
			hasConfigRevision = true
		}
	}

	if !hasNodeIdentity {
		if _, err := db.Exec("ALTER TABLE monitor_samples ADD COLUMN node_identity_key TEXT NOT NULL DEFAULT '';"); err != nil {
			return fmt.Errorf("migrate add node_identity_key: %w", err)
		}
		// Migration Limitation Note (B-01): When upgrading from PR#3, samples only retain the legacy node_key.
		// If a node had multiple credential revisions during PR#3, the new NodeIdentityKey cannot be mathematically
		// back-computed from raw samples alone without credentials. Setting node_identity_key = node_key preserves
		// raw data without destructive migration while QueryMonitorSamplesCursor and GetDerivedStats bridge queries
		// using both NodeIdentityKey and LegacyNodeKey.
		_, _ = db.Exec("UPDATE monitor_samples SET node_identity_key = node_key WHERE node_identity_key = '' OR node_identity_key IS NULL;")
	}

	if !hasConfigRevision {
		if _, err := db.Exec("ALTER TABLE monitor_samples ADD COLUMN config_revision_key TEXT NOT NULL DEFAULT '';"); err != nil {
			return fmt.Errorf("migrate add config_revision_key: %w", err)
		}
	}

	// Ensure all required indexes are present
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_samples_node_identity_time ON monitor_samples(node_identity_key, timestamp DESC);",
		"CREATE INDEX IF NOT EXISTS idx_samples_cursor_desc ON monitor_samples(timestamp DESC, sample_id DESC);",
		"CREATE INDEX IF NOT EXISTS idx_samples_cursor_asc ON monitor_samples(timestamp ASC, sample_id ASC);",
	}
	for _, idx := range indexes {
		if _, err := db.Exec(idx); err != nil {
			return fmt.Errorf("create index: %w", err)
		}
	}

	return nil
}

// Close closes the underlying database.
func (d *DB) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.db != nil {
		err := d.db.Close()
		d.db = nil
		return err
	}
	return nil
}

// SaveMonitorRun persists a new MonitorRun record.
func (d *DB) SaveMonitorRun(ctx context.Context, run *monitor.MonitorRun) error {
	if run == nil {
		return fmt.Errorf("run is nil")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	query := `
		INSERT INTO monitor_runs (
			run_id, job_id, scheduled_at, started_at, finished_at,
			status, total_nodes, success_nodes, failed_nodes, error_message
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	var finishedAt any
	if run.FinishedAt != nil {
		finishedAt = run.FinishedAt.UTC()
	}

	_, err := d.db.ExecContext(ctx, query,
		run.RunID,
		run.JobID,
		run.ScheduledAt.UTC(),
		run.StartedAt.UTC(),
		finishedAt,
		string(run.Status),
		run.TotalNodes,
		run.SuccessNodes,
		run.FailedNodes,
		run.ErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("insert monitor_run %s: %w", run.RunID, err)
	}
	return nil
}

// UpdateMonitorRun updates an existing MonitorRun record (status, finish time, counts).
func (d *DB) UpdateMonitorRun(ctx context.Context, run *monitor.MonitorRun) error {
	if run == nil {
		return fmt.Errorf("run is nil")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	query := `
		UPDATE monitor_runs SET
			finished_at = ?,
			status = ?,
			total_nodes = ?,
			success_nodes = ?,
			failed_nodes = ?,
			error_message = ?
		WHERE run_id = ?
	`

	var finishedAt any
	if run.FinishedAt != nil {
		finishedAt = run.FinishedAt.UTC()
	}

	res, err := d.db.ExecContext(ctx, query,
		finishedAt,
		string(run.Status),
		run.TotalNodes,
		run.SuccessNodes,
		run.FailedNodes,
		run.ErrorMessage,
		run.RunID,
	)
	if err != nil {
		return fmt.Errorf("update monitor_run %s: %w", run.RunID, err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("monitor_run %s not found for update", run.RunID)
	}
	return nil
}

// SaveMonitorSamples atomically inserts multiple MonitorSample records in a single transaction.
func (d *DB) SaveMonitorSamples(ctx context.Context, samples []*monitor.MonitorSample) error {
	if len(samples) == 0 {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx for monitor_samples: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO monitor_samples (
			sample_id, run_id, node_key, node_identity_key, config_revision_key,
			profile_id, display_name_snapshot, probe_type, target, timestamp,
			success, latency_ms, ttfb_ms, error_class, error_detail,
			exit_ip, exit_region, metadata_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert monitor_sample stmt: %w", err)
	}
	defer stmt.Close()

	for _, s := range samples {
		var metaJSON string
		if len(s.Metadata) > 0 {
			if bytes, err := json.Marshal(s.Metadata); err == nil {
				metaJSON = string(bytes)
			}
		}

		successInt := 0
		if s.Success {
			successInt = 1
		}

		_, err := stmt.ExecContext(ctx,
			s.SampleID,
			s.RunID,
			s.NodeKey,
			s.NodeIdentityKey,
			s.ConfigRevisionKey,
			s.ProfileID,
			s.DisplayNameSnapshot,
			s.ProbeType,
			s.Target,
			s.Timestamp.UTC(),
			successInt,
			s.Latency.Milliseconds(),
			s.TTFB.Milliseconds(),
			s.ErrorClass,
			s.ErrorDetail,
			s.ExitIP,
			s.ExitRegion,
			metaJSON,
		)
		if err != nil {
			return fmt.Errorf("insert sample %s: %w", s.SampleID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit monitor_samples tx: %w", err)
	}
	return nil
}

// QueryMonitorRuns retrieves recent MonitorRuns for a given job.
func (d *DB) QueryMonitorRuns(ctx context.Context, jobID string, limit int) ([]*monitor.MonitorRun, error) {
	if limit <= 0 {
		limit = 50
	}

	var query string
	var args []any

	if jobID != "" {
		query = "SELECT run_id, job_id, scheduled_at, started_at, finished_at, status, total_nodes, success_nodes, failed_nodes, error_message FROM monitor_runs WHERE job_id = ? ORDER BY scheduled_at DESC LIMIT ?"
		args = []any{jobID, limit}
	} else {
		query = "SELECT run_id, job_id, scheduled_at, started_at, finished_at, status, total_nodes, success_nodes, failed_nodes, error_message FROM monitor_runs ORDER BY scheduled_at DESC LIMIT ?"
		args = []any{limit}
	}

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query monitor_runs: %w", err)
	}
	defer rows.Close()

	var runs []*monitor.MonitorRun
	for rows.Next() {
		var r monitor.MonitorRun
		var finishedAt sql.NullTime
		var errDetail sql.NullString
		var statusStr string

		err := rows.Scan(
			&r.RunID,
			&r.JobID,
			&r.ScheduledAt,
			&r.StartedAt,
			&finishedAt,
			&statusStr,
			&r.TotalNodes,
			&r.SuccessNodes,
			&r.FailedNodes,
			&errDetail,
		)
		if err != nil {
			return nil, fmt.Errorf("scan monitor_run: %w", err)
		}

		if finishedAt.Valid {
			t := finishedAt.Time
			r.FinishedAt = &t
		}
		if errDetail.Valid {
			r.ErrorMessage = errDetail.String
		}
		r.Status = monitor.RunStatus(statusStr)
		runs = append(runs, &r)
	}
	return runs, rows.Err()
}

// QueryMonitorSamples retrieves raw samples according to the filter parameters.
func (d *DB) QueryMonitorSamples(ctx context.Context, filter monitor.SampleFilter) ([]*monitor.MonitorSample, error) {
	var whereClauses []string
	var args []any

	if filter.RunID != "" {
		whereClauses = append(whereClauses, "run_id = ?")
		args = append(args, filter.RunID)
	}
	if filter.NodeKey != "" {
		whereClauses = append(whereClauses, "node_key = ?")
		args = append(args, filter.NodeKey)
	}
	if filter.NodeIdentityKey != "" {
		whereClauses = append(whereClauses, "node_identity_key = ?")
		args = append(args, filter.NodeIdentityKey)
	}
	if filter.ProfileID != "" {
		whereClauses = append(whereClauses, "profile_id = ?")
		args = append(args, filter.ProfileID)
	}
	if filter.ProbeType != "" {
		whereClauses = append(whereClauses, "probe_type = ?")
		args = append(args, filter.ProbeType)
	}
	if filter.Target != "" {
		whereClauses = append(whereClauses, "target = ?")
		args = append(args, filter.Target)
	}
	if filter.Success != nil {
		successVal := 0
		if *filter.Success {
			successVal = 1
		}
		whereClauses = append(whereClauses, "success = ?")
		args = append(args, successVal)
	}
	if filter.Since != nil {
		whereClauses = append(whereClauses, "timestamp >= ?")
		args = append(args, filter.Since.UTC())
	}
	if filter.Until != nil {
		whereClauses = append(whereClauses, "timestamp <= ?")
		args = append(args, filter.Until.UTC())
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	orderDir := "ASC"
	if filter.OrderDesc {
		orderDir = "DESC"
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 10000 {
		limit = 10000
	}

	query := fmt.Sprintf(`
		SELECT sample_id, run_id, node_key, node_identity_key, config_revision_key,
		       profile_id, display_name_snapshot, probe_type, target, timestamp,
		       success, latency_ms, ttfb_ms, error_class, error_detail,
		       exit_ip, exit_region, metadata_json
		FROM monitor_samples
		%s
		ORDER BY timestamp %s, sample_id %s
		LIMIT ? OFFSET ?
	`, whereSQL, orderDir, orderDir)

	args = append(args, limit, filter.Offset)

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query monitor_samples: %w", err)
	}
	defer rows.Close()

	var samples []*monitor.MonitorSample
	for rows.Next() {
		var s monitor.MonitorSample
		var successInt int
		var latMs, ttfbMs int64
		var errDetail, exitIP, exitRegion, metaJSON sql.NullString

		err := rows.Scan(
			&s.SampleID,
			&s.RunID,
			&s.NodeKey,
			&s.NodeIdentityKey,
			&s.ConfigRevisionKey,
			&s.ProfileID,
			&s.DisplayNameSnapshot,
			&s.ProbeType,
			&s.Target,
			&s.Timestamp,
			&successInt,
			&latMs,
			&ttfbMs,
			&s.ErrorClass,
			&errDetail,
			&exitIP,
			&exitRegion,
			&metaJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("scan monitor_sample: %w", err)
		}

		s.Success = successInt == 1
		s.Latency = time.Duration(latMs) * time.Millisecond
		s.TTFB = time.Duration(ttfbMs) * time.Millisecond

		if errDetail.Valid {
			s.ErrorDetail = errDetail.String
		}
		if exitIP.Valid {
			s.ExitIP = exitIP.String
		}
		if exitRegion.Valid {
			s.ExitRegion = exitRegion.String
		}
		if metaJSON.Valid && metaJSON.String != "" {
			var m map[string]any
			if err := json.Unmarshal([]byte(metaJSON.String), &m); err == nil {
				s.Metadata = m
			}
		}

		samples = append(samples, &s)
	}
	return samples, rows.Err()
}

// GetNodeTimelineSamples retrieves chronologically ordered raw samples for a specific node since a given time.
func (d *DB) GetNodeTimelineSamples(ctx context.Context, nodeKey string, since time.Time) ([]*monitor.MonitorSample, error) {
	return d.QueryMonitorSamples(ctx, monitor.SampleFilter{
		NodeKey:   nodeKey,
		Since:     &since,
		OrderDesc: false, // Chronological ascending for timeline reconstruction
		Limit:     5000,
	})
}

// --- Keyset Pagination ---

const (
	maxCursorLength = 512
	cursorVersion   = 1
)

type cursorData struct {
	Version   int    `json:"v"`
	Timestamp int64  `json:"t"`
	SampleID  string `json:"id"`
	Direction string `json:"dir"` // "desc" or "asc"
}

// EncodeCursor creates an opaque, URL-safe base64 string representing the pagination cursor.
func EncodeCursor(t time.Time, sampleID string, direction string) string {
	b, _ := json.Marshal(cursorData{
		Version:   cursorVersion,
		Timestamp: t.UTC().UnixNano(),
		SampleID:  sampleID,
		Direction: strings.ToLower(direction),
	})
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor parses an opaque cursor string into its timestamp, sampleID, and validates direction/version.
func DecodeCursor(cursorStr string, expectedDir string) (time.Time, string, error) {
	if cursorStr == "" {
		return time.Time{}, "", nil
	}
	if len(cursorStr) > maxCursorLength {
		return time.Time{}, "", monitor.WrapValidationError(monitor.ErrInvalidCursor)
	}
	b, err := base64.RawURLEncoding.DecodeString(cursorStr)
	if err != nil {
		b, err = base64.StdEncoding.DecodeString(cursorStr)
		if err != nil {
			return time.Time{}, "", monitor.WrapValidationError(monitor.ErrInvalidCursor)
		}
	}
	var cd cursorData
	if err := json.Unmarshal(b, &cd); err != nil {
		return time.Time{}, "", monitor.WrapValidationError(monitor.ErrInvalidCursor)
	}
	if cd.Version != cursorVersion {
		return time.Time{}, "", monitor.WrapValidationError(monitor.ErrInvalidCursor)
	}
	if cd.SampleID == "" || cd.Timestamp == 0 {
		return time.Time{}, "", monitor.WrapValidationError(monitor.ErrInvalidCursor)
	}
	if expectedDir != "" && cd.Direction != "" && strings.ToLower(cd.Direction) != strings.ToLower(expectedDir) {
		return time.Time{}, "", monitor.WrapValidationError(fmt.Errorf("%w: cursor direction mismatch (expected %s, got %s)", monitor.ErrInvalidCursor, expectedDir, cd.Direction))
	}
	return time.Unix(0, cd.Timestamp).UTC(), cd.SampleID, nil
}

// QueryMonitorSamplesCursor executes keyset pagination based on (timestamp, sample_id).
// It ensures that dynamic sample insertions during paging will not duplicate or shift historical windows.
func (d *DB) QueryMonitorSamplesCursor(ctx context.Context, filter monitor.CursorFilter) (*monitor.SampleCursorPage, error) {
	if filter.Since != nil && filter.Until != nil && filter.Since.After(*filter.Until) {
		return nil, monitor.WrapValidationError(monitor.ErrInvalidTimeRange)
	}
	if filter.Limit < 0 || filter.Limit > 1000 {
		return nil, monitor.WrapValidationError(monitor.ErrInvalidLimit)
	}

	var whereClauses []string
	var args []any

	// B-01 Legacy Key Bridge:
	if filter.NodeIdentityKey != "" && filter.LegacyNodeKey != "" {
		whereClauses = append(whereClauses, "((node_identity_key = ?) OR (node_identity_key = node_key AND node_key = ?))")
		args = append(args, filter.NodeIdentityKey, filter.LegacyNodeKey)
	} else if filter.NodeIdentityKey != "" {
		whereClauses = append(whereClauses, "node_identity_key = ?")
		args = append(args, filter.NodeIdentityKey)
	} else if filter.LegacyNodeKey != "" {
		whereClauses = append(whereClauses, "node_key = ?")
		args = append(args, filter.LegacyNodeKey)
	}
	if filter.NodeKey != "" {
		whereClauses = append(whereClauses, "node_key = ?")
		args = append(args, filter.NodeKey)
	}
	if filter.ProfileID != "" {
		whereClauses = append(whereClauses, "profile_id = ?")
		args = append(args, filter.ProfileID)
	}
	if filter.ProbeType != "" {
		whereClauses = append(whereClauses, "probe_type = ?")
		args = append(args, filter.ProbeType)
	}
	if filter.Target != "" {
		whereClauses = append(whereClauses, "target = ?")
		args = append(args, filter.Target)
	}
	if filter.Success != nil {
		successVal := 0
		if *filter.Success {
			successVal = 1
		}
		whereClauses = append(whereClauses, "success = ?")
		args = append(args, successVal)
	}
	if filter.Since != nil {
		whereClauses = append(whereClauses, "timestamp >= ?")
		args = append(args, filter.Since.UTC())
	}
	if filter.Until != nil {
		whereClauses = append(whereClauses, "timestamp <= ?")
		args = append(args, filter.Until.UTC())
	}

	orderDir := "DESC"
	if !filter.OrderDesc {
		orderDir = "ASC"
	}

	// Keyset evaluation
	if filter.Cursor != "" {
		curTime, curID, err := DecodeCursor(filter.Cursor, orderDir)
		if err != nil {
			return nil, err
		}
		if !curTime.IsZero() && curID != "" {
			if orderDir == "DESC" {
				whereClauses = append(whereClauses, "((timestamp < ?) OR (timestamp = ? AND sample_id < ?))")
				args = append(args, curTime.UTC(), curTime.UTC(), curID)
			} else {
				whereClauses = append(whereClauses, "((timestamp > ?) OR (timestamp = ? AND sample_id > ?))")
				args = append(args, curTime.UTC(), curTime.UTC(), curID)
			}
		}
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Read limit + 1 to detect has_more without an extra COUNT query
	query := fmt.Sprintf(`
		SELECT sample_id, run_id, node_key, node_identity_key, config_revision_key,
		       profile_id, display_name_snapshot, probe_type, target, timestamp,
		       success, latency_ms, ttfb_ms, error_class, error_detail,
		       exit_ip, exit_region, metadata_json
		FROM monitor_samples
		%s
		ORDER BY timestamp %s, sample_id %s
		LIMIT ?
	`, whereSQL, orderDir, orderDir)

	args = append(args, limit+1)

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query monitor_samples cursor: %w", err)
	}
	defer rows.Close()

	samples := make([]*monitor.MonitorSample, 0)
	for rows.Next() {
		var s monitor.MonitorSample
		var successInt int
		var latMs, ttfbMs int64
		var errDetail, exitIP, exitRegion, metaJSON sql.NullString

		err := rows.Scan(
			&s.SampleID,
			&s.RunID,
			&s.NodeKey,
			&s.NodeIdentityKey,
			&s.ConfigRevisionKey,
			&s.ProfileID,
			&s.DisplayNameSnapshot,
			&s.ProbeType,
			&s.Target,
			&s.Timestamp,
			&successInt,
			&latMs,
			&ttfbMs,
			&s.ErrorClass,
			&errDetail,
			&exitIP,
			&exitRegion,
			&metaJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("scan monitor_sample cursor: %w", err)
		}

		s.Success = successInt == 1
		s.Latency = time.Duration(latMs) * time.Millisecond
		s.TTFB = time.Duration(ttfbMs) * time.Millisecond

		if errDetail.Valid {
			s.ErrorDetail = errDetail.String
		}
		if exitIP.Valid {
			s.ExitIP = exitIP.String
		}
		if exitRegion.Valid {
			s.ExitRegion = exitRegion.String
		}
		if metaJSON.Valid && metaJSON.String != "" {
			var m map[string]any
			if err := json.Unmarshal([]byte(metaJSON.String), &m); err == nil {
				s.Metadata = m
			}
		}

		samples = append(samples, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	hasMore := len(samples) > limit
	if hasMore {
		samples = samples[:limit]
	}

	page := &monitor.SampleCursorPage{
		Items:   samples,
		HasMore: hasMore,
		Limit:   limit,
	}

	if len(samples) > 0 && hasMore {
		lastItem := samples[len(samples)-1]
		page.NextCursor = EncodeCursor(lastItem.Timestamp, lastItem.SampleID, orderDir)
	}

	return page, nil
}

// GetDerivedStats dynamically calculates metrics across raw samples within a query window.
// It is read-only, never overwrites raw samples, calculates exact quantiles via SQL rank in O(1) memory,
// and gracefully handles edge cases (zero samples, all fail/pass).
func (d *DB) GetDerivedStats(ctx context.Context, q monitor.StatsQuery) (*monitor.DerivedStats, error) {
	if q.Since != nil && q.Until != nil {
		if q.Since.After(*q.Until) {
			return nil, monitor.WrapValidationError(monitor.ErrInvalidTimeRange)
		}
		if q.Until.Sub(*q.Since) > 366*24*time.Hour {
			return nil, monitor.WrapValidationError(monitor.ErrQueryWindowTooLarge)
		}
	}

	var whereClauses []string
	var args []any

	if q.NodeIdentityKey != "" && q.LegacyNodeKey != "" {
		whereClauses = append(whereClauses, "((node_identity_key = ?) OR (node_identity_key = node_key AND node_key = ?))")
		args = append(args, q.NodeIdentityKey, q.LegacyNodeKey)
	} else if q.NodeIdentityKey != "" {
		whereClauses = append(whereClauses, "node_identity_key = ?")
		args = append(args, q.NodeIdentityKey)
	} else if q.LegacyNodeKey != "" {
		whereClauses = append(whereClauses, "node_key = ?")
		args = append(args, q.LegacyNodeKey)
	}
	if q.NodeKey != "" {
		whereClauses = append(whereClauses, "node_key = ?")
		args = append(args, q.NodeKey)
	}
	if q.ProfileID != "" {
		whereClauses = append(whereClauses, "profile_id = ?")
		args = append(args, q.ProfileID)
	}
	if q.ProbeType != "" {
		whereClauses = append(whereClauses, "probe_type = ?")
		args = append(args, q.ProbeType)
	}
	if q.Target != "" {
		whereClauses = append(whereClauses, "target = ?")
		args = append(args, q.Target)
	}
	if q.Since != nil {
		whereClauses = append(whereClauses, "timestamp >= ?")
		args = append(args, q.Since.UTC())
	}
	if q.Until != nil {
		whereClauses = append(whereClauses, "timestamp <= ?")
		args = append(args, q.Until.UTC())
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	// 1. Summary counts and first/last timestamps via SQL aggregate
	summaryQuery := fmt.Sprintf(`
		SELECT COUNT(*),
		       COALESCE(SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN success = 0 THEN 1 ELSE 0 END), 0),
		       MIN(timestamp),
		       MAX(timestamp)
		FROM monitor_samples
		%s
	`, whereSQL)

	var totalCount, successCount, failureCount int64
	var minTimeStr, maxTimeStr sql.NullString

	err := d.db.QueryRowContext(ctx, summaryQuery, args...).Scan(
		&totalCount,
		&successCount,
		&failureCount,
		&minTimeStr,
		&maxTimeStr,
	)
	if err != nil {
		return nil, fmt.Errorf("calculate stats summary: %w", err)
	}

	// Bounded exact sample safety boundary (B-02)
	const maxExactStatsSamples = 1000000
	if totalCount > maxExactStatsSamples {
		return nil, monitor.WrapValidationError(fmt.Errorf("%w: matched %d samples (safety boundary %d)", monitor.ErrQueryWindowTooLarge, totalCount, maxExactStatsSamples))
	}

	stats := &monitor.DerivedStats{
		SampleCount:     totalCount,
		SuccessCount:    successCount,
		FailureCount:    failureCount,
		ErrorBreakdown:  make(map[string]int64),
		NodeIdentityKey: q.NodeIdentityKey,
		NodeKey:         q.NodeKey,
		ProbeType:       q.ProbeType,
	}

	if q.Since != nil {
		stats.ObservedSince = q.Since
	}
	if q.Until != nil {
		stats.ObservedUntil = q.Until
	}

	if minTimeStr.Valid && minTimeStr.String != "" {
		if t, err := parseSQLiteTime(minTimeStr.String); err == nil {
			stats.FirstSampleAt = &t
			if stats.ObservedSince == nil {
				stats.ObservedSince = &t
			}
		}
	}
	if maxTimeStr.Valid && maxTimeStr.String != "" {
		if t, err := parseSQLiteTime(maxTimeStr.String); err == nil {
			stats.LastSampleAt = &t
			if stats.ObservedUntil == nil {
				stats.ObservedUntil = &t
			}
		}
	}

	if totalCount == 0 {
		return stats, nil
	}

	stats.SuccessRate = math.Round((float64(successCount)/float64(totalCount))*10000) / 10000

	// 2. Error Breakdown for failed samples
	if failureCount > 0 {
		errWhere := whereSQL
		if errWhere == "" {
			errWhere = "WHERE success = 0"
		} else {
			errWhere += " AND success = 0"
		}
		errQuery := fmt.Sprintf(`
			SELECT error_class, COUNT(*)
			FROM monitor_samples
			%s
			GROUP BY error_class
		`, errWhere)

		rows, err := d.db.QueryContext(ctx, errQuery, args...)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var errClass string
				var cnt int64
				if scanErr := rows.Scan(&errClass, &cnt); scanErr == nil {
					if errClass == "" {
						errClass = "unknown"
					}
					stats.ErrorBreakdown[errClass] = cnt
				}
			}
		}
	}

	// 3. Exact Percentiles and Min/Max for Latency via SQL rank & streaming offset (O(1) memory)
	// Latency only accounts for valid successful samples with latency_ms > 0
	if successCount > 0 {
		latWhere := whereSQL
		if latWhere == "" {
			latWhere = "WHERE success = 1 AND latency_ms > 0"
		} else {
			latWhere += " AND success = 1 AND latency_ms > 0"
		}

		var validLatCount int64
		var minLat, maxLat sql.NullInt64
		latSummaryQuery := fmt.Sprintf(`SELECT COUNT(*), MIN(latency_ms), MAX(latency_ms) FROM monitor_samples %s`, latWhere)
		if err := d.db.QueryRowContext(ctx, latSummaryQuery, args...).Scan(&validLatCount, &minLat, &maxLat); err == nil && validLatCount > 0 {
			if minLat.Valid {
				v := minLat.Int64
				stats.LatencyMinMs = &v
			}
			if maxLat.Valid {
				v := maxLat.Int64
				stats.LatencyMaxMs = &v
			}

			// P50 rank
			rank50 := int(math.Ceil(0.50*float64(validLatCount))) - 1
			if rank50 < 0 {
				rank50 = 0
			}
			var p50Val int64
			p50Query := fmt.Sprintf(`SELECT latency_ms FROM monitor_samples %s ORDER BY latency_ms ASC LIMIT 1 OFFSET ?`, latWhere)
			p50Args := append(append([]any{}, args...), rank50)
			if err := d.db.QueryRowContext(ctx, p50Query, p50Args...).Scan(&p50Val); err == nil {
				stats.LatencyP50Ms = &p50Val
			}

			// P95 rank
			rank95 := int(math.Ceil(0.95*float64(validLatCount))) - 1
			if rank95 < 0 {
				rank95 = 0
			}
			var p95Val int64
			p95Query := fmt.Sprintf(`SELECT latency_ms FROM monitor_samples %s ORDER BY latency_ms ASC LIMIT 1 OFFSET ?`, latWhere)
			p95Args := append(append([]any{}, args...), rank95)
			if err := d.db.QueryRowContext(ctx, p95Query, p95Args...).Scan(&p95Val); err == nil {
				stats.LatencyP95Ms = &p95Val
			}
		}

		// 4. Exact Percentiles for TTFB (exclude non-HTTP evidence where ttfb_ms <= 0 or NULL)
		ttfbWhere := whereSQL
		if ttfbWhere == "" {
			ttfbWhere = "WHERE success = 1 AND ttfb_ms > 0"
		} else {
			ttfbWhere += " AND success = 1 AND ttfb_ms > 0"
		}

		var validTTFBCount int64
		ttfbCountQuery := fmt.Sprintf(`SELECT COUNT(*) FROM monitor_samples %s`, ttfbWhere)
		if err := d.db.QueryRowContext(ctx, ttfbCountQuery, args...).Scan(&validTTFBCount); err == nil && validTTFBCount > 0 {
			// TTFB P50
			ttfbRank50 := int(math.Ceil(0.50*float64(validTTFBCount))) - 1
			if ttfbRank50 < 0 {
				ttfbRank50 = 0
			}
			var ttfbP50Val int64
			ttfbP50Query := fmt.Sprintf(`SELECT ttfb_ms FROM monitor_samples %s ORDER BY ttfb_ms ASC LIMIT 1 OFFSET ?`, ttfbWhere)
			ttfbP50Args := append(append([]any{}, args...), ttfbRank50)
			if err := d.db.QueryRowContext(ctx, ttfbP50Query, ttfbP50Args...).Scan(&ttfbP50Val); err == nil {
				stats.TTFBP50Ms = &ttfbP50Val
			}

			// TTFB P95
			ttfbRank95 := int(math.Ceil(0.95*float64(validTTFBCount))) - 1
			if ttfbRank95 < 0 {
				ttfbRank95 = 0
			}
			var ttfbP95Val int64
			ttfbP95Query := fmt.Sprintf(`SELECT ttfb_ms FROM monitor_samples %s ORDER BY ttfb_ms ASC LIMIT 1 OFFSET ?`, ttfbWhere)
			ttfbP95Args := append(append([]any{}, args...), ttfbRank95)
			if err := d.db.QueryRowContext(ctx, ttfbP95Query, ttfbP95Args...).Scan(&ttfbP95Val); err == nil {
				stats.TTFBP95Ms = &ttfbP95Val
			}
		}
	}

	return stats, nil
}

func parseSQLiteTime(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse sqlite time: %s", s)
}

// ApplyRetention prunes raw samples and orphaned runs according to the retention policy.
// It executes deletions in bounded batches to prevent monopolizing the SQLite write lock.
// Default policy is KeepAll, which performs no deletions.
func (d *DB) ApplyRetention(ctx context.Context, req monitor.RetentionRequest) (*monitor.RetentionResult, error) {
	if req.Policy == "" || req.Policy == monitor.RetentionKeepAll {
		return &monitor.RetentionResult{
			Policy:         monitor.RetentionKeepAll,
			Cutoff:         time.Time{},
			SamplesDeleted: 0,
			RunsDeleted:    0,
			DurationMs:     0,
		}, nil
	}

	start := time.Now()
	now := time.Now().UTC()
	var cutoff time.Time

	if req.CutoffTime != nil && !req.CutoffTime.IsZero() {
		cutoff = req.CutoffTime.UTC()
		if cutoff.After(now) {
			return nil, monitor.WrapValidationError(monitor.ErrFutureCutoff)
		}
	} else {
		switch req.Policy {
		case monitor.Retention30d:
			cutoff = now.AddDate(0, 0, -30)
		case monitor.Retention90d:
			cutoff = now.AddDate(0, 0, -90)
		case monitor.Retention180d:
			cutoff = now.AddDate(0, 0, -180)
		case monitor.RetentionCustom:
			if req.CustomDays <= 0 || req.CustomDays > 36500 {
				return nil, monitor.WrapValidationError(monitor.ErrInvalidCustomDays)
			}
			cutoff = now.AddDate(0, 0, -req.CustomDays)
		default:
			return nil, monitor.WrapValidationError(monitor.ErrInvalidRetentionPolicy)
		}
	}

	if cutoff.After(now) {
		return nil, monitor.WrapValidationError(monitor.ErrFutureCutoff)
	}

	const batchSize = 500
	var totalSamplesDeleted int64

	// Prune samples in bounded batches
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		var batchDeleted int64
		err := func() error {
			d.mu.Lock()
			defer d.mu.Unlock()

			tx, err := d.db.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			defer func() { _ = tx.Rollback() }()

			res, err := tx.ExecContext(ctx, `
				DELETE FROM monitor_samples
				WHERE sample_id IN (
					SELECT sample_id FROM monitor_samples
					WHERE timestamp < ?
					LIMIT ?
				)
			`, cutoff, batchSize)
			if err != nil {
				return err
			}

			batchDeleted, err = res.RowsAffected()
			if err != nil {
				return err
			}

			return tx.Commit()
		}()
		if err != nil {
			return nil, fmt.Errorf("prune sample batch: %w", err)
		}

		totalSamplesDeleted += batchDeleted
		if batchDeleted < batchSize {
			break
		}

		// Briefly pause to yield single-writer lock to concurrent probe workers
		time.Sleep(2 * time.Millisecond)
	}

	// Prune orphaned completed/failed/partial_failed runs older than cutoff with no remaining samples.
	// Strictly preserve 'running' (in-flight) and 'skipped' (intentional no-op runs with 0 samples).
	var runsDeleted int64
	err := func() error {
		d.mu.Lock()
		defer d.mu.Unlock()

		res, err := d.db.ExecContext(ctx, `
			DELETE FROM monitor_runs
			WHERE scheduled_at < ?
			  AND status IN ('completed', 'failed', 'partial_failed')
			  AND run_id NOT IN (SELECT DISTINCT run_id FROM monitor_samples)
		`, cutoff)
		if err != nil {
			return err
		}
		runsDeleted, err = res.RowsAffected()
		return err
	}()
	if err != nil {
		return nil, fmt.Errorf("prune orphaned runs: %w", err)
	}

	return &monitor.RetentionResult{
		Policy:         req.Policy,
		Cutoff:         cutoff,
		SamplesDeleted: totalSamplesDeleted,
		RunsDeleted:    runsDeleted,
		DurationMs:     time.Since(start).Milliseconds(),
	}, nil
}
