package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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

	return &DB{db: db}, nil
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
			sample_id, run_id, node_key, profile_id, display_name_snapshot,
			probe_type, target, timestamp, success, latency_ms, ttfb_ms,
			error_class, error_detail, exit_ip, exit_region, metadata_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
	if filter.ProfileID != "" {
		whereClauses = append(whereClauses, "profile_id = ?")
		args = append(args, filter.ProfileID)
	}
	if filter.ProbeType != "" {
		whereClauses = append(whereClauses, "probe_type = ?")
		args = append(args, filter.ProbeType)
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
		SELECT sample_id, run_id, node_key, profile_id, display_name_snapshot,
		       probe_type, target, timestamp, success, latency_ms, ttfb_ms,
		       error_class, error_detail, exit_ip, exit_region, metadata_json
		FROM monitor_samples
		%s
		ORDER BY timestamp %s
		LIMIT ? OFFSET ?
	`, whereSQL, orderDir)

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
