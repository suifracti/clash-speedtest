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

CREATE TABLE IF NOT EXISTS workbench_latency_tests (
    attempt_id TEXT PRIMARY KEY,
    profile_id TEXT NOT NULL,
    node_key TEXT NOT NULL,
    node_identity_key TEXT NOT NULL,
    config_revision_key TEXT NOT NULL,
    display_name TEXT NOT NULL,
    node_type TEXT NOT NULL,
    test_project TEXT NOT NULL,
    requested_at DATETIME NOT NULL,
    started_at DATETIME NOT NULL,
    finished_at DATETIME NOT NULL,
    status TEXT NOT NULL,
    latency_ms INTEGER NOT NULL DEFAULT 0,
    jitter_ms INTEGER NOT NULL DEFAULT 0,
    packet_loss REAL NOT NULL DEFAULT 0,
    total_samples INTEGER NOT NULL DEFAULT 0,
    success_samples INTEGER NOT NULL DEFAULT 0,
    failure_samples INTEGER NOT NULL DEFAULT 0,
    error_message TEXT
);

CREATE TABLE IF NOT EXISTS workbench_latency_samples (
    attempt_id TEXT NOT NULL,
    seq INTEGER NOT NULL,
    timestamp DATETIME NOT NULL,
    latency_ms INTEGER NOT NULL DEFAULT 0,
    success INTEGER NOT NULL,
    error TEXT,
    PRIMARY KEY (attempt_id, seq),
    FOREIGN KEY (attempt_id) REFERENCES workbench_latency_tests(attempt_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_workbench_latency_scope
    ON workbench_latency_tests(profile_id, node_key, finished_at DESC);
CREATE INDEX IF NOT EXISTS idx_workbench_latency_samples_time
    ON workbench_latency_samples(attempt_id, timestamp ASC, seq ASC);
`

const monitorJobDDL = `
CREATE TABLE IF NOT EXISTS monitor_job_definitions (
    job_id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    profile_id TEXT NOT NULL,
    probe_set TEXT NOT NULL,
    interval_ns INTEGER NOT NULL,
    timeout_ns INTEGER NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    definition_version INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS monitor_job_nodes (
    job_id TEXT NOT NULL,
    ordinal INTEGER NOT NULL,
    node_key TEXT NOT NULL,
    node_identity_key TEXT NOT NULL,
    config_revision_key TEXT NOT NULL,
    display_name TEXT NOT NULL,
    node_type TEXT NOT NULL,
    PRIMARY KEY (job_id, ordinal),
    FOREIGN KEY (job_id) REFERENCES monitor_job_definitions(job_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_monitor_job_nodes_job_id ON monitor_job_nodes(job_id, ordinal);
`

const schemaMetaDDL = `
CREATE TABLE IF NOT EXISTS schema_meta (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    schema_version INTEGER NOT NULL
);
`

const monitorBudgetDDL = `
CREATE TABLE monitor_budget_usage (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    utc_day TEXT NOT NULL,
    requests_used INTEGER NOT NULL DEFAULT 0 CHECK (requests_used >= 0),
    bytes_used INTEGER NOT NULL DEFAULT 0 CHECK (bytes_used >= 0)
);
INSERT INTO monitor_budget_usage (singleton, utc_day, requests_used, bytes_used) VALUES (1, '', 0, 0);
`

const workbenchLatencyBatchDDL = `
CREATE TABLE IF NOT EXISTS workbench_latency_batches (
    batch_id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL UNIQUE,
    test_project TEXT NOT NULL,
    timeout_seconds INTEGER NOT NULL,
    requested_at DATETIME NOT NULL,
    state TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS workbench_latency_batch_items (
    item_id TEXT PRIMARY KEY,
    batch_id TEXT NOT NULL,
    ordinal INTEGER NOT NULL,
    profile_id TEXT NOT NULL,
    node_key TEXT NOT NULL,
    node_identity_key TEXT NOT NULL,
    config_revision_key TEXT NOT NULL,
    display_name TEXT NOT NULL,
    node_type TEXT NOT NULL,
    execution_state TEXT NOT NULL,
    persistence_state TEXT NOT NULL,
    attempt_id TEXT UNIQUE,
    requested_at DATETIME NOT NULL,
    started_at DATETIME,
    finished_at DATETIME,
    error_message TEXT NOT NULL DEFAULT '',
    persistence_error TEXT NOT NULL DEFAULT '',
    result_json TEXT NOT NULL DEFAULT '',
    UNIQUE(batch_id, ordinal),
    FOREIGN KEY(batch_id) REFERENCES workbench_latency_batches(batch_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_workbench_latency_batch_items_batch
    ON workbench_latency_batch_items(batch_id, ordinal);
`

const workbenchPublicServiceDDL = `
CREATE TABLE IF NOT EXISTS workbench_public_service_attempts (
    attempt_id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL UNIQUE,
    profile_id TEXT NOT NULL,
    node_key TEXT NOT NULL,
    node_identity_key TEXT NOT NULL,
    config_revision_key TEXT NOT NULL,
    display_name TEXT NOT NULL,
    node_type TEXT NOT NULL,
    source TEXT NOT NULL,
    service_id TEXT NOT NULL,
    requested_at DATETIME NOT NULL,
    started_at DATETIME,
    finished_at DATETIME,
    execution_state TEXT NOT NULL,
    persistence_state TEXT NOT NULL,
    persistence_error TEXT NOT NULL DEFAULT '',
    rule_snapshot_json TEXT NOT NULL,
    staged_result_json TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_workbench_public_service_scope
    ON workbench_public_service_attempts(profile_id, node_identity_key, config_revision_key, service_id, requested_at DESC);
CREATE TABLE IF NOT EXISTS workbench_public_service_results (
    attempt_id TEXT PRIMARY KEY,
    result_json TEXT NOT NULL,
    saved_at DATETIME NOT NULL,
    FOREIGN KEY(attempt_id) REFERENCES workbench_public_service_attempts(attempt_id) ON DELETE CASCADE
);
`

// CurrentSchemaVersion is the SQLite schema authority. Databases without a
// schema_meta row are the explicitly recognized pre-version legacy schema.
const CurrentSchemaVersion = 6

var (
	ErrUnsupportedSchemaVersion = fmt.Errorf("unsupported SQLite schema version")
	ErrUnrecognizedSchema       = fmt.Errorf("unrecognized SQLite schema")
)

// DB manages the SQLite single-writer connection pool with WAL mode enabled.
type DB struct {
	db              *sql.DB
	mu              sync.Mutex // Write lock guaranteeing strictly serialized single-writer transactions
	testBatchFailAt int        // For testing fault-injection during batched retention
}

// SetTestBatchFailAt configures a deterministic fault on retention batch #n (for test verification).
func (d *DB) SetTestBatchFailAt(n int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.testBatchFailAt = n
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

	// Initialize or upgrade the schema inside one explicit transaction. This
	// also rejects unknown future schemas before any DDL is applied.
	if err := migrateSchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate sqlite schema: %w", err)
	}
	// WAL changes the database header, so only enable it after schema
	// recognition/migration has succeeded. Unknown schemas therefore remain
	// untouched by OpenDB.
	if _, err := db.Exec("PRAGMA journal_mode = WAL;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable sqlite WAL mode: %w", err)
	}

	return &DB{db: db}, nil
}

func migrateSchema(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin schema migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	hasMeta, version, err := readSchemaVersion(tx)
	if err != nil {
		return err
	}
	if hasMeta {
		if version > CurrentSchemaVersion {
			return fmt.Errorf("%w: found %d, current %d", ErrUnsupportedSchemaVersion, version, CurrentSchemaVersion)
		}
		if version < 1 {
			return fmt.Errorf("%w: schema version %d cannot be upgraded safely", ErrUnrecognizedSchema, version)
		}
		if err := validateCurrentSchema(tx, version); err != nil {
			return fmt.Errorf("validate schema version %d: %w", version, err)
		}
	} else {
		empty, knownLegacy, err := inspectUnversionedSchema(tx)
		if err != nil {
			return err
		}
		if !empty && !knownLegacy {
			return fmt.Errorf("%w: missing schema_meta on an unknown database", ErrUnrecognizedSchema)
		}
		if err := installSchemaVersion1(tx); err != nil {
			return err
		}
		version = 1
	}
	if version == 1 {
		if _, err := tx.Exec(monitorBudgetDDL); err != nil {
			return fmt.Errorf("migrate Monitor budget ledger: %w", err)
		}
		if _, err := tx.Exec("UPDATE schema_meta SET schema_version = 2 WHERE singleton = 1"); err != nil {
			return fmt.Errorf("record schema version 2: %w", err)
		}
		version = 2
	}
	if version == 2 {
		if err := migrateSamplingSourceV3(tx); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE schema_meta SET schema_version = 3 WHERE singleton = 1"); err != nil {
			return fmt.Errorf("record schema version 3: %w", err)
		}
		version = 3
	}
	if version == 3 {
		if err := migrateMonitorLaunchRecoveryV4(tx); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE schema_meta SET schema_version = 4 WHERE singleton = 1"); err != nil {
			return fmt.Errorf("record schema version 4: %w", err)
		}
		version = 4
	}
	if version == 4 {
		if err := migrateWorkbenchLatencyBatchesV5(tx); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE schema_meta SET schema_version = 5 WHERE singleton = 1"); err != nil {
			return fmt.Errorf("record schema version 5: %w", err)
		}
		version = 5
	}
	if version == 5 {
		if _, err := tx.Exec(workbenchPublicServiceDDL); err != nil {
			return fmt.Errorf("migrate Workbench public-service history: %w", err)
		}
		if _, err := tx.Exec("UPDATE schema_meta SET schema_version = 6 WHERE singleton = 1"); err != nil {
			return fmt.Errorf("record schema version 6: %w", err)
		}
		version = 6
	}
	if err := validateCurrentSchema(tx, version); err != nil {
		return fmt.Errorf("validate schema version %d after migration: %w", version, err)
	}
	if err := validateMonitorBudgetSchema(tx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema migration: %w", err)
	}
	return nil
}

func readSchemaVersion(tx *sql.Tx) (bool, int, error) {
	var exists int
	if err := tx.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_meta'").Scan(&exists); err != nil {
		return false, 0, fmt.Errorf("inspect schema_meta: %w", err)
	}
	if exists == 0 {
		return false, 0, nil
	}

	rows, err := tx.Query("SELECT singleton, schema_version FROM schema_meta")
	if err != nil {
		return true, 0, fmt.Errorf("read schema_meta: %w", err)
	}
	defer rows.Close()
	count := 0
	version := 0
	for rows.Next() {
		var singleton, current int
		if err := rows.Scan(&singleton, &current); err != nil {
			return true, 0, fmt.Errorf("scan schema_meta: %w", err)
		}
		if singleton != 1 || count != 0 {
			return true, 0, fmt.Errorf("%w: schema_meta must contain exactly one singleton row", ErrUnrecognizedSchema)
		}
		version = current
		count++
	}
	if err := rows.Err(); err != nil {
		return true, 0, fmt.Errorf("read schema_meta rows: %w", err)
	}
	if count != 1 {
		return true, 0, fmt.Errorf("%w: schema_meta has no singleton row", ErrUnrecognizedSchema)
	}
	return true, version, nil
}

func inspectUnversionedSchema(tx *sql.Tx) (empty, known bool, err error) {
	allowed := map[string]bool{
		"monitor_runs":              true,
		"monitor_samples":           true,
		"workbench_latency_tests":   true,
		"workbench_latency_samples": true,
	}
	knownTables := 0
	rows, err := tx.Query("SELECT type, name FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' AND type <> 'index'")
	if err != nil {
		return false, false, fmt.Errorf("inspect unversioned schema: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var objectType, name string
		if err := rows.Scan(&objectType, &name); err != nil {
			return false, false, fmt.Errorf("scan unversioned schema: %w", err)
		}
		if objectType != "table" || !allowed[name] {
			return false, false, nil
		}
		knownTables++
	}
	if err := rows.Err(); err != nil {
		return false, false, fmt.Errorf("read unversioned schema: %w", err)
	}
	if knownTables == 0 {
		return true, false, nil
	}

	required := map[string][]string{
		"monitor_runs":              {"run_id", "job_id", "scheduled_at", "started_at", "status"},
		"monitor_samples":           {"sample_id", "run_id", "node_key", "profile_id", "timestamp", "success"},
		"workbench_latency_tests":   {"attempt_id", "profile_id", "node_key", "requested_at", "finished_at"},
		"workbench_latency_samples": {"attempt_id", "seq", "timestamp"},
	}
	for table, columns := range required {
		present, err := tableExists(tx, table)
		if err != nil {
			return false, false, err
		}
		if !present {
			continue
		}
		if err := requireColumns(tx, table, columns); err != nil {
			return false, false, fmt.Errorf("legacy %s: %w", table, err)
		}
	}
	return false, true, nil
}

func installSchemaVersion1(tx *sql.Tx) error {
	if _, err := tx.Exec(ddlSchema); err != nil {
		return fmt.Errorf("initialize base schema: %w", err)
	}
	if err := migrateLegacyMonitorColumns(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(monitorJobDDL); err != nil {
		return fmt.Errorf("initialize monitor job schema: %w", err)
	}
	if _, err := tx.Exec(schemaMetaDDL); err != nil {
		return fmt.Errorf("initialize schema_meta: %w", err)
	}
	if _, err := tx.Exec("INSERT INTO schema_meta (singleton, schema_version) VALUES (1, 1)"); err != nil {
		return fmt.Errorf("record schema version: %w", err)
	}
	return nil
}

func migrateLegacyMonitorColumns(tx *sql.Tx) error {
	present, err := tableExists(tx, "monitor_samples")
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	columns, err := tableColumns(tx, "monitor_samples")
	if err != nil {
		return err
	}
	if !columns["node_identity_key"] {
		if _, err := tx.Exec("ALTER TABLE monitor_samples ADD COLUMN node_identity_key TEXT NOT NULL DEFAULT ''"); err != nil {
			return fmt.Errorf("migrate add node_identity_key: %w", err)
		}
		// PR#3 samples only have node_key; preserve them and let the existing
		// identity bridge keep those raw facts queryable.
		if _, err := tx.Exec("UPDATE monitor_samples SET node_identity_key = node_key WHERE node_identity_key = '' OR node_identity_key IS NULL"); err != nil {
			return fmt.Errorf("backfill node_identity_key: %w", err)
		}
	}
	if !columns["config_revision_key"] {
		if _, err := tx.Exec("ALTER TABLE monitor_samples ADD COLUMN config_revision_key TEXT NOT NULL DEFAULT ''"); err != nil {
			return fmt.Errorf("migrate add config_revision_key: %w", err)
		}
	}
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_samples_node_identity_time ON monitor_samples(node_identity_key, timestamp DESC)",
		"CREATE INDEX IF NOT EXISTS idx_samples_cursor_desc ON monitor_samples(timestamp DESC, sample_id DESC)",
		"CREATE INDEX IF NOT EXISTS idx_samples_cursor_asc ON monitor_samples(timestamp ASC, sample_id ASC)",
	}
	for _, indexDDL := range indexes {
		if _, err := tx.Exec(indexDDL); err != nil {
			return fmt.Errorf("create monitor sample index: %w", err)
		}
	}
	return nil
}

func migrateSamplingSourceV3(tx *sql.Tx) error {
	for _, change := range []struct {
		table  string
		column string
		ddl    string
	}{
		{"monitor_job_definitions", "sampling_tier", "ALTER TABLE monitor_job_definitions ADD COLUMN sampling_tier TEXT NOT NULL DEFAULT 'regular'"},
		{"monitor_runs", "sampling_tier", "ALTER TABLE monitor_runs ADD COLUMN sampling_tier TEXT NOT NULL DEFAULT 'legacy_unknown'"},
		{"monitor_runs", "trigger_type", "ALTER TABLE monitor_runs ADD COLUMN trigger_type TEXT NOT NULL DEFAULT 'legacy_unknown'"},
		{"monitor_runs", "sampling_strategy_version", "ALTER TABLE monitor_runs ADD COLUMN sampling_strategy_version INTEGER NOT NULL DEFAULT 0"},
	} {
		columns, err := tableColumns(tx, change.table)
		if err != nil {
			return err
		}
		if !columns[change.column] {
			if _, err := tx.Exec(change.ddl); err != nil {
				return fmt.Errorf("migrate %s.%s: %w", change.table, change.column, err)
			}
		}
	}
	if _, err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_runs_sampling_source_time ON monitor_runs(sampling_tier, trigger_type, scheduled_at DESC)"); err != nil {
		return fmt.Errorf("create monitor sampling source index: %w", err)
	}
	return nil
}

func migrateMonitorLaunchRecoveryV4(tx *sql.Tx) error {
	columns, err := tableColumns(tx, "monitor_job_definitions")
	if err != nil {
		return err
	}
	if !columns["resume_on_launch"] {
		if _, err := tx.Exec("ALTER TABLE monitor_job_definitions ADD COLUMN resume_on_launch INTEGER NOT NULL DEFAULT 0"); err != nil {
			return fmt.Errorf("migrate monitor resume preference: %w", err)
		}
	}
	if !columns["desired_state"] {
		if _, err := tx.Exec("ALTER TABLE monitor_job_definitions ADD COLUMN desired_state TEXT NOT NULL DEFAULT 'stopped'"); err != nil {
			return fmt.Errorf("migrate monitor desired state: %w", err)
		}
	}
	// Schema v3 definitions predate explicit launch intent. Preserve their
	// effective stopped behavior and do not infer consent from old run rows.
	if _, err := tx.Exec("UPDATE monitor_job_definitions SET definition_version = ? WHERE definition_version < ?", monitor.MonitorJobDefinitionVersion, monitor.MonitorJobDefinitionVersion); err != nil {
		return fmt.Errorf("upgrade monitor job definition version: %w", err)
	}
	return nil
}

func validateCurrentSchema(tx *sql.Tx, version int) error {
	tables := map[string][]string{
		"monitor_runs":              {"run_id", "job_id", "scheduled_at", "started_at", "status"},
		"monitor_samples":           {"sample_id", "run_id", "node_key", "node_identity_key", "config_revision_key", "profile_id", "timestamp", "success"},
		"workbench_latency_tests":   {"attempt_id", "profile_id", "node_key", "requested_at", "finished_at"},
		"workbench_latency_samples": {"attempt_id", "seq", "timestamp"},
		"monitor_job_definitions":   {"job_id", "profile_id", "probe_set", "interval_ns", "timeout_ns", "definition_version"},
		"monitor_job_nodes":         {"job_id", "ordinal", "node_key", "node_identity_key", "config_revision_key"},
	}
	if version >= 3 {
		tables["monitor_runs"] = append(tables["monitor_runs"], "sampling_tier", "trigger_type", "sampling_strategy_version")
		tables["monitor_job_definitions"] = append(tables["monitor_job_definitions"], "sampling_tier")
	}
	if version >= 4 {
		tables["monitor_job_definitions"] = append(tables["monitor_job_definitions"], "resume_on_launch", "desired_state")
	}
	if version >= 5 {
		tables["workbench_latency_tests"] = append(tables["workbench_latency_tests"], "source", "method", "method_version", "target", "unit")
		tables["workbench_latency_batches"] = []string{"batch_id", "request_id", "test_project", "timeout_seconds", "requested_at", "state"}
		tables["workbench_latency_batch_items"] = []string{"item_id", "batch_id", "ordinal", "profile_id", "node_key", "node_identity_key", "config_revision_key", "display_name", "node_type", "execution_state", "persistence_state", "attempt_id", "requested_at", "started_at", "finished_at", "error_message", "persistence_error", "result_json"}
	}
	if version >= 6 {
		tables["workbench_public_service_attempts"] = []string{
			"attempt_id", "request_id", "profile_id", "node_key", "node_identity_key", "config_revision_key",
			"display_name", "node_type", "source", "service_id", "requested_at", "execution_state",
			"persistence_state", "rule_snapshot_json", "staged_result_json",
		}
		tables["workbench_public_service_results"] = []string{"attempt_id", "result_json", "saved_at"}
	}
	for table, columns := range tables {
		present, err := tableExists(tx, table)
		if err != nil {
			return err
		}
		if !present {
			return fmt.Errorf("required table %s is missing", table)
		}
		if err := requireColumns(tx, table, columns); err != nil {
			return fmt.Errorf("table %s: %w", table, err)
		}
	}
	return nil
}

func migrateWorkbenchLatencyBatchesV5(tx *sql.Tx) error {
	columns, err := tableColumns(tx, "workbench_latency_tests")
	if err != nil {
		return err
	}
	for _, change := range []struct{ name, ddl string }{
		{"source", "ALTER TABLE workbench_latency_tests ADD COLUMN source TEXT NOT NULL DEFAULT ''"},
		{"method", "ALTER TABLE workbench_latency_tests ADD COLUMN method TEXT NOT NULL DEFAULT ''"},
		{"method_version", "ALTER TABLE workbench_latency_tests ADD COLUMN method_version INTEGER NOT NULL DEFAULT 0"},
		{"target", "ALTER TABLE workbench_latency_tests ADD COLUMN target TEXT NOT NULL DEFAULT ''"},
		{"unit", "ALTER TABLE workbench_latency_tests ADD COLUMN unit TEXT NOT NULL DEFAULT ''"},
	} {
		if !columns[change.name] {
			if _, err := tx.Exec(change.ddl); err != nil {
				return fmt.Errorf("migrate workbench latency %s: %w", change.name, err)
			}
		}
	}
	if _, err := tx.Exec(workbenchLatencyBatchDDL); err != nil {
		return fmt.Errorf("create workbench latency batches: %w", err)
	}
	return nil
}

func validateMonitorBudgetSchema(tx *sql.Tx) error {
	present, err := tableExists(tx, "monitor_budget_usage")
	if err != nil {
		return err
	}
	if !present {
		return fmt.Errorf("required table monitor_budget_usage is missing")
	}
	if err := requireColumns(tx, "monitor_budget_usage", []string{"singleton", "utc_day", "requests_used", "bytes_used"}); err != nil {
		return err
	}
	var day string
	var requests, bytes int64
	if err := tx.QueryRow("SELECT utc_day, requests_used, bytes_used FROM monitor_budget_usage WHERE singleton = 1").Scan(&day, &requests, &bytes); err != nil {
		return fmt.Errorf("read Monitor budget ledger: %w", err)
	}
	if requests < 0 || bytes < 0 {
		return fmt.Errorf("Monitor budget ledger contains negative usage")
	}
	return nil
}

func tableExists(tx *sql.Tx, table string) (bool, error) {
	var count int
	if err := tx.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&count); err != nil {
		return false, fmt.Errorf("inspect table %s: %w", table, err)
	}
	return count == 1, nil
}

func tableColumns(tx *sql.Tx, table string) (map[string]bool, error) {
	rows, err := tx.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return nil, fmt.Errorf("inspect columns for %s: %w", table, err)
	}
	defer rows.Close()
	columns := make(map[string]bool)
	for rows.Next() {
		var cid, notNull, pk int
		var name, colType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			return nil, fmt.Errorf("scan columns for %s: %w", table, err)
		}
		columns[strings.ToLower(name)] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read columns for %s: %w", table, err)
	}
	return columns, nil
}

func requireColumns(tx *sql.Tx, table string, required []string) error {
	columns, err := tableColumns(tx, table)
	if err != nil {
		return err
	}
	for _, column := range required {
		if !columns[strings.ToLower(column)] {
			return fmt.Errorf("required column %s is missing", column)
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

// SaveMonitorJobDefinition durably stores the credential-free product
// definition. Runtime scheduler state and raw node configuration never enter
// this transaction.
func (d *DB) SaveMonitorJobDefinition(ctx context.Context, definition *monitor.MonitorJobDefinition) error {
	if definition == nil {
		return fmt.Errorf("monitor job definition is nil")
	}
	if definition.ID == "" {
		return fmt.Errorf("monitor job definition id is empty")
	}
	version := definition.DefinitionVersion
	if version == 0 {
		version = monitor.MonitorJobDefinitionVersion
	}
	if version != monitor.MonitorJobDefinitionVersion {
		return fmt.Errorf("unsupported monitor job definition version %d", version)
	}
	tier := definition.SamplingTier
	if tier == "" {
		tier = monitor.SamplingTierRegular
	}
	if tier != monitor.SamplingTierRegular && tier != monitor.SamplingTierFocus && tier != monitor.SamplingTierSparse {
		return fmt.Errorf("unsupported periodic monitor sampling tier %q", tier)
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.db == nil {
		return fmt.Errorf("history database is closed")
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin monitor job definition transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO monitor_job_definitions (
			job_id, name, profile_id, probe_set, interval_ns, timeout_ns,
			created_at, updated_at, definition_version, sampling_tier,
			resume_on_launch, desired_state
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, definition.ID, definition.Name, definition.ProfileID, string(definition.ProbeSet),
		definition.Interval.Nanoseconds(), definition.Timeout.Nanoseconds(), definition.CreatedAt.UTC(),
		definition.UpdatedAt.UTC(), version, string(tier), definition.ResumeOnLaunch,
		string(normalizedMonitorDesiredState(definition.DesiredState)))
	if err != nil {
		return fmt.Errorf("insert monitor job definition %s: %w", definition.ID, err)
	}

	for ordinal, node := range definition.Nodes {
		if node.NodeKey == "" || node.NodeIdentityKey == "" || node.ConfigRevisionKey == "" {
			return fmt.Errorf("monitor job definition %s contains an incomplete node reference", definition.ID)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO monitor_job_nodes (
				job_id, ordinal, node_key, node_identity_key, config_revision_key,
				display_name, node_type
			) VALUES (?, ?, ?, ?, ?, ?, ?)
		`, definition.ID, ordinal, node.NodeKey, node.NodeIdentityKey, node.ConfigRevisionKey,
			node.DisplayName, node.Type); err != nil {
			return fmt.Errorf("insert monitor job node %s/%d: %w", definition.ID, ordinal, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit monitor job definition %s: %w", definition.ID, err)
	}
	return nil
}

func normalizedMonitorDesiredState(state monitor.JobState) monitor.JobState {
	switch state {
	case monitor.JobStateRunning, monitor.JobStatePaused, monitor.JobStateStopped:
		return state
	default:
		return monitor.JobStateStopped
	}
}

// UpdateMonitorJobLaunchIntent persists user-controlled launch permission and
// desired lifecycle state together. Runtime scheduler state is never written.
func (d *DB) UpdateMonitorJobLaunchIntent(ctx context.Context, jobID string, resumeOnLaunch bool, desired monitor.JobState, updatedAt time.Time) error {
	if strings.TrimSpace(jobID) == "" {
		return fmt.Errorf("monitor job definition id is empty")
	}
	if desired != monitor.JobStateRunning && desired != monitor.JobStatePaused && desired != monitor.JobStateStopped {
		return fmt.Errorf("unsupported monitor desired state %q", desired)
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.db == nil {
		return fmt.Errorf("history database is closed")
	}
	result, err := d.db.ExecContext(ctx, `
		UPDATE monitor_job_definitions
		SET resume_on_launch = ?, desired_state = ?, updated_at = ?
		WHERE job_id = ?
	`, resumeOnLaunch, string(desired), updatedAt.UTC(), jobID)
	if err != nil {
		return fmt.Errorf("update monitor job %s launch intent: %w", jobID, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check monitor job %s launch intent: %w", jobID, err)
	}
	if changed == 0 {
		return fmt.Errorf("monitor job definition %s not found", jobID)
	}
	return nil
}

func (d *DB) DeleteMonitorJobDefinition(ctx context.Context, jobID string) error {
	if strings.TrimSpace(jobID) == "" {
		return fmt.Errorf("monitor job definition id is empty")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.db == nil {
		return fmt.Errorf("history database is closed")
	}
	result, err := d.db.ExecContext(ctx, `DELETE FROM monitor_job_definitions WHERE job_id = ?`, jobID)
	if err != nil {
		return fmt.Errorf("delete monitor job definition %s: %w", jobID, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check monitor job %s deletion: %w", jobID, err)
	}
	if changed == 0 {
		return fmt.Errorf("monitor job definition %s not found", jobID)
	}
	return nil
}

// UpdateMonitorJobSamplingTier changes only the persisted cadence class. It
// leaves the selected nodes, interval, probe set, and runtime state untouched.
func (d *DB) UpdateMonitorJobSamplingTier(ctx context.Context, jobID string, tier monitor.SamplingTier, updatedAt time.Time) error {
	if strings.TrimSpace(jobID) == "" {
		return fmt.Errorf("monitor job definition id is empty")
	}
	if tier != monitor.SamplingTierRegular && tier != monitor.SamplingTierFocus && tier != monitor.SamplingTierSparse {
		return fmt.Errorf("unsupported periodic monitor sampling tier %q", tier)
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.db == nil {
		return fmt.Errorf("history database is closed")
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin monitor sampling tier update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
		UPDATE monitor_job_definitions
		SET sampling_tier = ?, updated_at = ?
		WHERE job_id = ?
	`, string(tier), updatedAt.UTC(), jobID)
	if err != nil {
		return fmt.Errorf("update monitor job %s sampling tier: %w", jobID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check monitor job %s sampling tier update: %w", jobID, err)
	}
	if affected == 0 {
		return fmt.Errorf("monitor job definition %s not found", jobID)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit monitor job %s sampling tier update: %w", jobID, err)
	}
	return nil
}

// ListMonitorJobDefinitions reads only durable product definitions. The caller
// must resolve node references against the current canonical profile/cache.
func (d *DB) ListMonitorJobDefinitions(ctx context.Context) ([]*monitor.MonitorJobDefinition, error) {
	if d == nil || d.db == nil {
		return nil, fmt.Errorf("history database is closed")
	}
	rows, err := d.db.QueryContext(ctx, `
		SELECT job_id, name, profile_id, probe_set, interval_ns, timeout_ns,
		       created_at, updated_at, definition_version, sampling_tier,
		       resume_on_launch, desired_state
		FROM monitor_job_definitions
		ORDER BY created_at ASC, job_id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query monitor job definitions: %w", err)
	}
	defer rows.Close()

	definitions := make([]*monitor.MonitorJobDefinition, 0)
	for rows.Next() {
		var (
			definition   monitor.MonitorJobDefinition
			probeSet     string
			samplingTier string
			desiredState string
			intervalNS   int64
			timeoutNS    int64
		)
		if err := rows.Scan(&definition.ID, &definition.Name, &definition.ProfileID, &probeSet,
			&intervalNS, &timeoutNS, &definition.CreatedAt, &definition.UpdatedAt,
			&definition.DefinitionVersion, &samplingTier, &definition.ResumeOnLaunch, &desiredState); err != nil {
			return nil, fmt.Errorf("scan monitor job definition: %w", err)
		}
		definition.ProbeSet = monitor.ProbeSetType(probeSet)
		definition.SamplingTier = monitor.SamplingTier(samplingTier)
		definition.DesiredState = normalizedMonitorDesiredState(monitor.JobState(desiredState))
		definition.Interval = time.Duration(intervalNS)
		definition.Timeout = time.Duration(timeoutNS)
		definition.Nodes, err = d.listMonitorJobNodes(ctx, definition.ID)
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, &definition)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read monitor job definitions: %w", err)
	}
	return definitions, nil
}

func (d *DB) listMonitorJobNodes(ctx context.Context, jobID string) ([]monitor.MonitorJobNodeReference, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT node_key, node_identity_key, config_revision_key, display_name, node_type
		FROM monitor_job_nodes
		WHERE job_id = ?
		ORDER BY ordinal ASC
	`, jobID)
	if err != nil {
		return nil, fmt.Errorf("query monitor job nodes %s: %w", jobID, err)
	}
	defer rows.Close()

	nodes := make([]monitor.MonitorJobNodeReference, 0)
	for rows.Next() {
		var node monitor.MonitorJobNodeReference
		if err := rows.Scan(&node.NodeKey, &node.NodeIdentityKey, &node.ConfigRevisionKey,
			&node.DisplayName, &node.Type); err != nil {
			return nil, fmt.Errorf("scan monitor job node %s: %w", jobID, err)
		}
		nodes = append(nodes, node)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read monitor job nodes %s: %w", jobID, err)
	}
	return nodes, nil
}

// SaveMonitorRun persists a new MonitorRun record.
func (d *DB) SaveMonitorRun(ctx context.Context, run *monitor.MonitorRun) error {
	if run == nil {
		return fmt.Errorf("run is nil")
	}
	tier := run.SamplingTier
	if tier == "" {
		tier = monitor.SamplingTierRegular
	}
	trigger := run.TriggerType
	if trigger == "" {
		trigger = monitor.SamplingTriggerScheduled
	}
	strategyVersion := run.SamplingStrategyVersion
	if strategyVersion == 0 {
		strategyVersion = monitor.SamplingStrategyVersion
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	query := `
		INSERT INTO monitor_runs (
			run_id, job_id, scheduled_at, started_at, finished_at,
			status, total_nodes, success_nodes, failed_nodes, error_message,
			sampling_tier, trigger_type, sampling_strategy_version
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		string(tier),
		string(trigger),
		strategyVersion,
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

// MarkRunningMonitorRunsInterrupted closes stale runtime rows without changing
// their timestamps or any raw samples. The precise interruption time is not
// inferred from a later application launch.
func (d *DB) MarkRunningMonitorRunsInterrupted(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.db == nil {
		return fmt.Errorf("history database is closed")
	}
	_, err := d.db.ExecContext(ctx, `
		UPDATE monitor_runs
		SET status = ?,
		    error_message = CASE
		        WHEN error_message IS NULL OR error_message = '' THEN ?
		        ELSE error_message || '; ' || ?
	    END
		WHERE status = ?
	`, string(monitor.RunStatusInterrupted), "应用未观测期间该轮被中断；无法确定实际中断时间",
		"应用未观测期间该轮被中断；无法确定实际中断时间", string(monitor.RunStatusRunning))
	if err != nil {
		return fmt.Errorf("mark unfinished monitor runs interrupted: %w", err)
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

// SaveLatencyTest atomically persists one on-demand workbench test and all of
// its raw latency samples. AttemptID is the idempotency key: retrying the same
// completed execution is a no-op, while reusing it for another node/scope is
// rejected.
func (d *DB) SaveLatencyTest(ctx context.Context, test *LatencyTest) error {
	if test == nil {
		return fmt.Errorf("latency test is nil")
	}
	if strings.TrimSpace(test.AttemptID) == "" {
		return fmt.Errorf("latency test attempt_id is empty")
	}
	if strings.TrimSpace(test.ProfileID) == "" || strings.TrimSpace(test.NodeKey) == "" {
		return fmt.Errorf("latency test identity is incomplete")
	}
	if strings.TrimSpace(test.TestProject) == "" {
		return fmt.Errorf("latency test project is empty")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin latency test transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingProfileID, existingNodeKey, existingProject string
	err = tx.QueryRowContext(ctx, `
		SELECT profile_id, node_key, test_project
		FROM workbench_latency_tests
		WHERE attempt_id = ?
	`, test.AttemptID).Scan(&existingProfileID, &existingNodeKey, &existingProject)
	switch {
	case err == nil:
		if existingProfileID != test.ProfileID || existingNodeKey != test.NodeKey || existingProject != test.TestProject {
			return fmt.Errorf("latency test attempt_id %q already belongs to another execution", test.AttemptID)
		}
		return nil
	case err != sql.ErrNoRows:
		return fmt.Errorf("check latency test %s: %w", test.AttemptID, err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO workbench_latency_tests (
			attempt_id, profile_id, node_key, node_identity_key, config_revision_key,
			display_name, node_type, test_project, requested_at, started_at, finished_at,
			status, latency_ms, jitter_ms, packet_loss, total_samples,
			success_samples, failure_samples, error_message, source, method,
			method_version, target, unit
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		test.AttemptID,
		test.ProfileID,
		test.NodeKey,
		test.NodeIdentityKey,
		test.ConfigRevisionKey,
		test.DisplayName,
		test.NodeType,
		test.TestProject,
		test.RequestedAt.UTC(),
		test.StartedAt.UTC(),
		test.FinishedAt.UTC(),
		test.Status,
		test.LatencyMs,
		test.JitterMs,
		test.PacketLoss,
		test.TotalSamples,
		test.SuccessSamples,
		test.FailureSamples,
		test.ErrorMessage,
		test.Source,
		test.Method,
		test.MethodVersion,
		test.Target,
		test.Unit,
	)
	if err != nil {
		return fmt.Errorf("insert latency test %s: %w", test.AttemptID, err)
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO workbench_latency_samples (
			attempt_id, seq, timestamp, latency_ms, success, error
		) VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare latency sample insert: %w", err)
	}
	defer stmt.Close()

	for _, sample := range test.Samples {
		successInt := 0
		if sample.Success {
			successInt = 1
		}
		if _, err := stmt.ExecContext(ctx,
			test.AttemptID,
			sample.Seq,
			sample.Timestamp.UTC(),
			sample.LatencyMs,
			successInt,
			sample.Error,
		); err != nil {
			return fmt.Errorf("insert latency sample %s/%d: %w", test.AttemptID, sample.Seq, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit latency test %s: %w", test.AttemptID, err)
	}
	return nil
}

// QueryLatencyTests returns persisted on-demand tests scoped to one logical
// subscription node. Samples are loaded with each record so the UI never has
// to synthesize a graph from aggregate values.
func (d *DB) QueryLatencyTests(ctx context.Context, filter LatencyTestFilter) (*LatencyTestQueryResult, error) {
	if strings.TrimSpace(filter.ProfileID) == "" || strings.TrimSpace(filter.NodeKey) == "" {
		return nil, fmt.Errorf("latency history requires profile_id and node_key")
	}
	since, until, err := normalizeLatencyWindow(filter.Since, filter.Until)
	if err != nil {
		return nil, err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	where := `t.profile_id = ? AND t.node_key = ?`
	args := []any{filter.ProfileID, filter.NodeKey}
	if filter.NodeIdentityKey != "" {
		where += ` AND t.node_identity_key = ?`
		args = append(args, filter.NodeIdentityKey)
	}
	if filter.ConfigRevisionKey != "" {
		where += ` AND t.config_revision_key = ?`
		args = append(args, filter.ConfigRevisionKey)
	}
	if since != nil {
		// Scope attempts by raw sample timestamps before applying LIMIT. An
		// attempt may straddle the boundary and remains eligible when any raw
		// sample belongs to the requested half-open interval.
		where += ` AND EXISTS (
			SELECT 1 FROM workbench_latency_samples AS ws
			WHERE ws.attempt_id = t.attempt_id
			  AND ws.timestamp >= ? AND ws.timestamp < ?
		)`
		args = append(args, *since, *until)
	}
	if (filter.BeforeFinishedAt == nil) != (filter.BeforeAttemptID == "") {
		return nil, fmt.Errorf("latency history cursor requires before_finished_at and before_attempt_id")
	}
	if filter.BeforeFinishedAt != nil {
		where += ` AND (t.finished_at < ? OR (t.finished_at = ? AND t.attempt_id < ?))`
		args = append(args, filter.BeforeFinishedAt.UTC(), filter.BeforeFinishedAt.UTC(), filter.BeforeAttemptID)
	}
	args = append(args, limit+1)
	rows, err := d.db.QueryContext(ctx, `
		SELECT attempt_id, profile_id, node_key, node_identity_key, config_revision_key,
			display_name, node_type, test_project, requested_at, started_at, finished_at,
			status, latency_ms, jitter_ms, packet_loss, total_samples,
			 success_samples, failure_samples, error_message, source, method,
			method_version, target, unit
		FROM workbench_latency_tests AS t
		WHERE `+where+`
		ORDER BY finished_at DESC, attempt_id DESC
		LIMIT ?
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("query latency tests: %w", err)
	}
	defer rows.Close()

	var tests []*LatencyTest
	for rows.Next() {
		test, err := scanLatencyTest(rows)
		if err != nil {
			return nil, fmt.Errorf("scan latency test: %w", err)
		}
		tests = append(tests, test)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate latency tests: %w", err)
	}
	hasMore := len(tests) > limit
	if hasMore {
		tests = tests[:limit]
	}

	for _, test := range tests {
		if err := d.loadLatencyTestSamples(ctx, test, since, until); err != nil {
			return nil, err
		}
	}
	return &LatencyTestQueryResult{Tests: tests, HasMore: hasMore}, nil
}

// GetLatencyTest returns one persisted on-demand test by its immutable attempt ID.
func (d *DB) GetLatencyTest(ctx context.Context, attemptID string) (*LatencyTest, error) {
	return d.getLatencyTest(ctx, attemptID, nil, nil)
}

// GetLatencyTestInWindow returns one attempt with only raw samples in the
// requested half-open observation window.
func (d *DB) GetLatencyTestInWindow(ctx context.Context, attemptID string, since, until *time.Time) (*LatencyTest, error) {
	normalizedSince, normalizedUntil, err := normalizeLatencyWindow(since, until)
	if err != nil {
		return nil, err
	}
	return d.getLatencyTest(ctx, attemptID, normalizedSince, normalizedUntil)
}

// ListNodeHistoryRevisions enumerates observed revisions for one stable node
// identity across the independent Monitor and Workbench history domains.
func (d *DB) ListNodeHistoryRevisions(ctx context.Context, profileID, nodeIdentityKey string) ([]NodeHistoryRevision, error) {
	if strings.TrimSpace(profileID) == "" || strings.TrimSpace(nodeIdentityKey) == "" {
		return nil, fmt.Errorf("node history revisions require profile_id and node_identity_key")
	}
	rows, err := d.db.QueryContext(ctx, `
		WITH history AS (
			SELECT config_revision_key, node_key, display_name_snapshot AS display_name,
				timestamp AS observed_at, sample_id AS record_id
			FROM monitor_samples
			WHERE profile_id = ? AND node_identity_key = ? AND config_revision_key <> ''
			UNION ALL
			SELECT config_revision_key, node_key, display_name,
				finished_at AS observed_at, attempt_id AS record_id
			FROM workbench_latency_tests
			WHERE profile_id = ? AND node_identity_key = ? AND config_revision_key <> ''
		), ranked AS (
			SELECT config_revision_key, node_key, display_name, observed_at,
				ROW_NUMBER() OVER (PARTITION BY config_revision_key ORDER BY observed_at DESC, record_id DESC) AS revision_rank
			FROM history
		)
		SELECT config_revision_key, node_key, display_name, observed_at
		FROM ranked WHERE revision_rank = 1
		ORDER BY observed_at DESC, config_revision_key
	`, profileID, nodeIdentityKey, profileID, nodeIdentityKey)
	if err != nil {
		return nil, fmt.Errorf("list node history revisions: %w", err)
	}
	defer rows.Close()
	revisions := make([]NodeHistoryRevision, 0)
	for rows.Next() {
		var item NodeHistoryRevision
		if err := rows.Scan(&item.ConfigRevisionKey, &item.NodeKey, &item.DisplayName, &item.LastObservedAt); err != nil {
			return nil, fmt.Errorf("scan node history revision: %w", err)
		}
		item.LastObservedAt = item.LastObservedAt.UTC()
		revisions = append(revisions, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate node history revisions: %w", err)
	}
	return revisions, nil
}

func (d *DB) getLatencyTest(ctx context.Context, attemptID string, since, until *time.Time) (*LatencyTest, error) {
	attemptID = strings.TrimSpace(attemptID)
	if attemptID == "" {
		return nil, fmt.Errorf("latency test attempt_id is empty")
	}
	row := d.db.QueryRowContext(ctx, `
		SELECT attempt_id, profile_id, node_key, node_identity_key, config_revision_key,
			display_name, node_type, test_project, requested_at, started_at, finished_at,
			status, latency_ms, jitter_ms, packet_loss, total_samples,
			success_samples, failure_samples, error_message, source, method,
			method_version, target, unit
		FROM workbench_latency_tests
		WHERE attempt_id = ?
	`, attemptID)
	test, err := scanLatencyTest(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("latency test %q not found: %w", attemptID, err)
		}
		return nil, fmt.Errorf("get latency test %s: %w", attemptID, err)
	}
	if err := d.loadLatencyTestSamples(ctx, test, since, until); err != nil {
		return nil, err
	}
	return test, nil
}

type latencyTestScanner interface {
	Scan(dest ...any) error
}

func scanLatencyTest(scanner latencyTestScanner) (*LatencyTest, error) {
	test := &LatencyTest{}
	var requestedAt, startedAt, finishedAt time.Time
	var errorMessage sql.NullString
	if err := scanner.Scan(
		&test.AttemptID,
		&test.ProfileID,
		&test.NodeKey,
		&test.NodeIdentityKey,
		&test.ConfigRevisionKey,
		&test.DisplayName,
		&test.NodeType,
		&test.TestProject,
		&requestedAt,
		&startedAt,
		&finishedAt,
		&test.Status,
		&test.LatencyMs,
		&test.JitterMs,
		&test.PacketLoss,
		&test.TotalSamples,
		&test.SuccessSamples,
		&test.FailureSamples,
		&errorMessage,
		&test.Source,
		&test.Method,
		&test.MethodVersion,
		&test.Target,
		&test.Unit,
	); err != nil {
		return nil, err
	}
	test.RequestedAt = requestedAt.UTC()
	test.StartedAt = startedAt.UTC()
	test.FinishedAt = finishedAt.UTC()
	test.ErrorMessage = errorMessage.String
	return test, nil
}

func (d *DB) loadLatencyTestSamples(ctx context.Context, test *LatencyTest, since, until *time.Time) error {
	query := `
		SELECT seq, timestamp, latency_ms, success, error
		FROM workbench_latency_samples
		WHERE attempt_id = ?`
	args := []any{test.AttemptID}
	if since != nil {
		query += ` AND timestamp >= ? AND timestamp < ?`
		args = append(args, *since, *until)
	}
	query += ` ORDER BY timestamp ASC, seq ASC`
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("query latency samples for %s: %w", test.AttemptID, err)
	}
	defer rows.Close()

	test.Samples = make([]LatencyTestSample, 0, test.TotalSamples)
	for rows.Next() {
		var sample LatencyTestSample
		var timestamp time.Time
		var success int
		var errorText sql.NullString
		if err := rows.Scan(&sample.Seq, &timestamp, &sample.LatencyMs, &success, &errorText); err != nil {
			return fmt.Errorf("scan latency sample for %s: %w", test.AttemptID, err)
		}
		sample.Timestamp = timestamp.UTC()
		sample.Success = success != 0
		sample.Error = errorText.String
		test.Samples = append(test.Samples, sample)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate latency samples for %s: %w", test.AttemptID, err)
	}
	return nil
}

func normalizeLatencyWindow(since, until *time.Time) (*time.Time, *time.Time, error) {
	if (since == nil) != (until == nil) {
		return nil, nil, fmt.Errorf("latency history requires both since and until")
	}
	if since == nil {
		return nil, nil, nil
	}
	normalizedSince := since.UTC()
	normalizedUntil := until.UTC()
	if !normalizedSince.Before(normalizedUntil) {
		return nil, nil, fmt.Errorf("latency history window must satisfy since < until")
	}
	return &normalizedSince, &normalizedUntil, nil
}

// QueryMonitorRuns retrieves recent MonitorRuns for a given job.
func (d *DB) QueryMonitorRuns(ctx context.Context, jobID string, limit int) ([]*monitor.MonitorRun, error) {
	if limit <= 0 {
		limit = 50
	}

	var query string
	var args []any

	if jobID != "" {
		query = "SELECT run_id, job_id, scheduled_at, started_at, finished_at, status, total_nodes, success_nodes, failed_nodes, error_message, sampling_tier, trigger_type, sampling_strategy_version FROM monitor_runs WHERE job_id = ? ORDER BY scheduled_at DESC LIMIT ?"
		args = []any{jobID, limit}
	} else {
		query = "SELECT run_id, job_id, scheduled_at, started_at, finished_at, status, total_nodes, success_nodes, failed_nodes, error_message, sampling_tier, trigger_type, sampling_strategy_version FROM monitor_runs ORDER BY scheduled_at DESC LIMIT ?"
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
		var tier, trigger string

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
			&tier,
			&trigger,
			&r.SamplingStrategyVersion,
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
		r.SamplingTier = monitor.SamplingTier(tier)
		r.TriggerType = monitor.SamplingTriggerType(trigger)
		runs = append(runs, &r)
	}
	return runs, rows.Err()
}

// QueryMonitorSamples retrieves raw samples according to the filter parameters.
func (d *DB) QueryMonitorSamples(ctx context.Context, filter monitor.SampleFilter) ([]*monitor.MonitorSample, error) {
	var whereClauses []string
	var args []any

	if filter.JobID != "" {
		whereClauses = append(whereClauses, "r.job_id = ?")
		args = append(args, filter.JobID)
	}
	if filter.RunID != "" {
		whereClauses = append(whereClauses, "s.run_id = ?")
		args = append(args, filter.RunID)
	}
	if filter.NodeKey != "" {
		whereClauses = append(whereClauses, "s.node_key = ?")
		args = append(args, filter.NodeKey)
	}
	if filter.NodeIdentityKey != "" {
		whereClauses = append(whereClauses, "s.node_identity_key = ?")
		args = append(args, filter.NodeIdentityKey)
	}
	if filter.ProfileID != "" {
		whereClauses = append(whereClauses, "s.profile_id = ?")
		args = append(args, filter.ProfileID)
	}
	if filter.ProbeType != "" {
		whereClauses = append(whereClauses, "s.probe_type = ?")
		args = append(args, filter.ProbeType)
	}
	if filter.Target != "" {
		whereClauses = append(whereClauses, "s.target = ?")
		args = append(args, filter.Target)
	}
	appendSamplingSourceFilter(&whereClauses, &args, filter.SamplingTier, filter.RegularObservationOnly)
	if filter.Success != nil {
		successVal := 0
		if *filter.Success {
			successVal = 1
		}
		whereClauses = append(whereClauses, "s.success = ?")
		args = append(args, successVal)
	}
	if filter.Since != nil {
		whereClauses = append(whereClauses, "s.timestamp >= ?")
		args = append(args, filter.Since.UTC())
	}
	if filter.Until != nil {
		whereClauses = append(whereClauses, "s.timestamp <= ?")
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
		SELECT s.sample_id, s.run_id, s.node_key, s.node_identity_key, s.config_revision_key,
		       s.profile_id, s.display_name_snapshot, s.probe_type, s.target, s.timestamp,
		       s.success, s.latency_ms, s.ttfb_ms, s.error_class, s.error_detail,
	       s.exit_ip, s.exit_region, s.metadata_json,
	       COALESCE(r.sampling_tier, 'legacy_unknown'), COALESCE(r.trigger_type, 'legacy_unknown'),
	       COALESCE(r.sampling_strategy_version, 0)
	FROM monitor_samples AS s LEFT JOIN monitor_runs AS r ON r.run_id = s.run_id
		%s
		ORDER BY s.timestamp %s, s.sample_id %s
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
		sample, err := scanMonitorSample(rows)
		if err != nil {
			return nil, fmt.Errorf("scan monitor_sample: %w", err)
		}
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}

func appendSamplingSourceFilter(whereClauses *[]string, args *[]any, tier monitor.SamplingTier, regularOnly bool) {
	if tier != "" {
		*whereClauses = append(*whereClauses, "COALESCE(r.sampling_tier, 'legacy_unknown') = ?")
		*args = append(*args, string(tier))
	}
	if regularOnly {
		*whereClauses = append(*whereClauses,
			"r.sampling_tier IN ('regular', 'focus', 'sparse') AND r.trigger_type = 'scheduled'")
	}
}

type monitorSampleScanner interface {
	Scan(dest ...any) error
}

func scanMonitorSample(scanner monitorSampleScanner) (*monitor.MonitorSample, error) {
	var sample monitor.MonitorSample
	var successInt int
	var latMs, ttfbMs int64
	var errDetail, exitIP, exitRegion, metaJSON sql.NullString
	var tier, trigger sql.NullString
	var strategyVersion int
	if err := scanner.Scan(
		&sample.SampleID, &sample.RunID, &sample.NodeKey, &sample.NodeIdentityKey,
		&sample.ConfigRevisionKey, &sample.ProfileID, &sample.DisplayNameSnapshot,
		&sample.ProbeType, &sample.Target, &sample.Timestamp, &successInt, &latMs,
		&ttfbMs, &sample.ErrorClass, &errDetail, &exitIP, &exitRegion, &metaJSON,
		&tier, &trigger, &strategyVersion,
	); err != nil {
		return nil, err
	}
	sample.Success = successInt == 1
	sample.Latency = time.Duration(latMs) * time.Millisecond
	sample.TTFB = time.Duration(ttfbMs) * time.Millisecond
	sample.SamplingTier = monitor.SamplingTierLegacyUnknown
	if tier.Valid && tier.String != "" {
		sample.SamplingTier = monitor.SamplingTier(tier.String)
	}
	sample.TriggerType = monitor.SamplingTriggerLegacyUnknown
	if trigger.Valid && trigger.String != "" {
		sample.TriggerType = monitor.SamplingTriggerType(trigger.String)
	}
	sample.SamplingStrategyVersion = strategyVersion
	if errDetail.Valid {
		sample.ErrorDetail = errDetail.String
	}
	if exitIP.Valid {
		sample.ExitIP = exitIP.String
	}
	if exitRegion.Valid {
		sample.ExitRegion = exitRegion.String
	}
	if metaJSON.Valid && metaJSON.String != "" {
		var metadata map[string]any
		if err := json.Unmarshal([]byte(metaJSON.String), &metadata); err == nil {
			sample.Metadata = metadata
		}
	}
	return &sample, nil
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
		whereClauses = append(whereClauses, "((s.node_identity_key = ?) OR (s.node_identity_key = s.node_key AND s.node_key = ?))")
		args = append(args, filter.NodeIdentityKey, filter.LegacyNodeKey)
	} else if filter.NodeIdentityKey != "" {
		whereClauses = append(whereClauses, "s.node_identity_key = ?")
		args = append(args, filter.NodeIdentityKey)
	} else if filter.LegacyNodeKey != "" {
		whereClauses = append(whereClauses, "s.node_key = ?")
		args = append(args, filter.LegacyNodeKey)
	}
	if filter.NodeKey != "" {
		whereClauses = append(whereClauses, "s.node_key = ?")
		args = append(args, filter.NodeKey)
	}
	if filter.ConfigRevisionKey != "" {
		whereClauses = append(whereClauses, "s.config_revision_key = ?")
		args = append(args, filter.ConfigRevisionKey)
	}
	if filter.ProfileID != "" {
		whereClauses = append(whereClauses, "s.profile_id = ?")
		args = append(args, filter.ProfileID)
	}
	if filter.ProbeType != "" {
		whereClauses = append(whereClauses, "s.probe_type = ?")
		args = append(args, filter.ProbeType)
	}
	if filter.Target != "" {
		whereClauses = append(whereClauses, "s.target = ?")
		args = append(args, filter.Target)
	}
	appendSamplingSourceFilter(&whereClauses, &args, filter.SamplingTier, filter.RegularObservationOnly)
	if filter.Success != nil {
		successVal := 0
		if *filter.Success {
			successVal = 1
		}
		whereClauses = append(whereClauses, "s.success = ?")
		args = append(args, successVal)
	}
	if filter.Since != nil {
		whereClauses = append(whereClauses, "s.timestamp >= ?")
		args = append(args, filter.Since.UTC())
	}
	if filter.Until != nil {
		whereClauses = append(whereClauses, "s.timestamp <= ?")
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
				whereClauses = append(whereClauses, "((s.timestamp < ?) OR (s.timestamp = ? AND s.sample_id < ?))")
				args = append(args, curTime.UTC(), curTime.UTC(), curID)
			} else {
				whereClauses = append(whereClauses, "((s.timestamp > ?) OR (s.timestamp = ? AND s.sample_id > ?))")
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
	SELECT s.sample_id, s.run_id, s.node_key, s.node_identity_key, s.config_revision_key,
	       s.profile_id, s.display_name_snapshot, s.probe_type, s.target, s.timestamp,
	       s.success, s.latency_ms, s.ttfb_ms, s.error_class, s.error_detail,
	       s.exit_ip, s.exit_region, s.metadata_json,
	       COALESCE(r.sampling_tier, 'legacy_unknown'), COALESCE(r.trigger_type, 'legacy_unknown'),
	       COALESCE(r.sampling_strategy_version, 0)
	FROM monitor_samples AS s LEFT JOIN monitor_runs AS r ON r.run_id = s.run_id
		%s
		ORDER BY s.timestamp %s, s.sample_id %s
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
		sample, err := scanMonitorSample(rows)
		if err != nil {
			return nil, fmt.Errorf("scan monitor_sample cursor: %w", err)
		}
		samples = append(samples, sample)
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
		whereClauses = append(whereClauses, "((s.node_identity_key = ?) OR (s.node_identity_key = s.node_key AND s.node_key = ?))")
		args = append(args, q.NodeIdentityKey, q.LegacyNodeKey)
	} else if q.NodeIdentityKey != "" {
		whereClauses = append(whereClauses, "s.node_identity_key = ?")
		args = append(args, q.NodeIdentityKey)
	} else if q.LegacyNodeKey != "" {
		whereClauses = append(whereClauses, "s.node_key = ?")
		args = append(args, q.LegacyNodeKey)
	}
	if q.NodeKey != "" {
		whereClauses = append(whereClauses, "s.node_key = ?")
		args = append(args, q.NodeKey)
	}
	if q.ConfigRevisionKey != "" {
		whereClauses = append(whereClauses, "s.config_revision_key = ?")
		args = append(args, q.ConfigRevisionKey)
	}
	if q.ProfileID != "" {
		whereClauses = append(whereClauses, "s.profile_id = ?")
		args = append(args, q.ProfileID)
	}
	if q.ProbeType != "" {
		whereClauses = append(whereClauses, "s.probe_type = ?")
		args = append(args, q.ProbeType)
	}
	if q.Target != "" {
		whereClauses = append(whereClauses, "s.target = ?")
		args = append(args, q.Target)
	}
	appendSamplingSourceFilter(&whereClauses, &args, q.SamplingTier, q.RegularObservationOnly)
	if q.Since != nil {
		whereClauses = append(whereClauses, "s.timestamp >= ?")
		args = append(args, q.Since.UTC())
	}
	if q.Until != nil {
		whereClauses = append(whereClauses, "s.timestamp <= ?")
		args = append(args, q.Until.UTC())
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	// 1. Summary counts and first/last timestamps via SQL aggregate
	summaryQuery := fmt.Sprintf(`
		SELECT COUNT(s.sample_id),
	       COALESCE(SUM(CASE WHEN s.success = 1 THEN 1 ELSE 0 END), 0),
	       COALESCE(SUM(CASE WHEN s.success = 0 THEN 1 ELSE 0 END), 0),
	       MIN(s.timestamp),
	       MAX(s.timestamp)
	FROM monitor_samples AS s LEFT JOIN monitor_runs AS r ON r.run_id = s.run_id
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
		SampleCount:            totalCount,
		RegularObservationOnly: q.RegularObservationOnly,
		SuccessCount:           successCount,
		FailureCount:           failureCount,
		ErrorBreakdown:         make(map[string]int64),
		NodeIdentityKey:        q.NodeIdentityKey,
		NodeKey:                q.NodeKey,
		ProbeType:              q.ProbeType,
	}
	tierRows, tierErr := d.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT DISTINCT COALESCE(r.sampling_tier, 'legacy_unknown')
		FROM monitor_samples AS s LEFT JOIN monitor_runs AS r ON r.run_id = s.run_id
		%s
		ORDER BY 1
	`, whereSQL), args...)
	if tierErr != nil {
		return nil, fmt.Errorf("query included sampling tiers: %w", tierErr)
	}
	for tierRows.Next() {
		var tier string
		if err := tierRows.Scan(&tier); err != nil {
			_ = tierRows.Close()
			return nil, fmt.Errorf("scan included sampling tier: %w", err)
		}
		stats.IncludedSamplingTiers = append(stats.IncludedSamplingTiers, monitor.SamplingTier(tier))
	}
	if err := tierRows.Err(); err != nil {
		_ = tierRows.Close()
		return nil, fmt.Errorf("read included sampling tiers: %w", err)
	}
	_ = tierRows.Close()

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
			errWhere = "WHERE s.success = 0"
		} else {
			errWhere += " AND s.success = 0"
		}
		errQuery := fmt.Sprintf(`
			SELECT s.error_class, COUNT(*)
		FROM monitor_samples AS s LEFT JOIN monitor_runs AS r ON r.run_id = s.run_id
			%s
			GROUP BY s.error_class
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
			latWhere = "WHERE s.success = 1 AND s.latency_ms > 0"
		} else {
			latWhere += " AND s.success = 1 AND s.latency_ms > 0"
		}

		var validLatCount int64
		var minLat, maxLat sql.NullInt64
		latSummaryQuery := fmt.Sprintf(`SELECT COUNT(s.sample_id), MIN(s.latency_ms), MAX(s.latency_ms) FROM monitor_samples AS s LEFT JOIN monitor_runs AS r ON r.run_id = s.run_id %s`, latWhere)
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
			p50Query := fmt.Sprintf(`SELECT s.latency_ms FROM monitor_samples AS s LEFT JOIN monitor_runs AS r ON r.run_id = s.run_id %s ORDER BY s.latency_ms ASC LIMIT 1 OFFSET ?`, latWhere)
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
			p95Query := fmt.Sprintf(`SELECT s.latency_ms FROM monitor_samples AS s LEFT JOIN monitor_runs AS r ON r.run_id = s.run_id %s ORDER BY s.latency_ms ASC LIMIT 1 OFFSET ?`, latWhere)
			p95Args := append(append([]any{}, args...), rank95)
			if err := d.db.QueryRowContext(ctx, p95Query, p95Args...).Scan(&p95Val); err == nil {
				stats.LatencyP95Ms = &p95Val
			}
		}

		// 4. Exact Percentiles for TTFB (exclude non-HTTP evidence where ttfb_ms <= 0 or NULL)
		ttfbWhere := whereSQL
		if ttfbWhere == "" {
			ttfbWhere = "WHERE s.success = 1 AND s.ttfb_ms > 0"
		} else {
			ttfbWhere += " AND s.success = 1 AND s.ttfb_ms > 0"
		}

		var validTTFBCount int64
		ttfbCountQuery := fmt.Sprintf(`SELECT COUNT(s.sample_id) FROM monitor_samples AS s LEFT JOIN monitor_runs AS r ON r.run_id = s.run_id %s`, ttfbWhere)
		if err := d.db.QueryRowContext(ctx, ttfbCountQuery, args...).Scan(&validTTFBCount); err == nil && validTTFBCount > 0 {
			// TTFB P50
			ttfbRank50 := int(math.Ceil(0.50*float64(validTTFBCount))) - 1
			if ttfbRank50 < 0 {
				ttfbRank50 = 0
			}
			var ttfbP50Val int64
			ttfbP50Query := fmt.Sprintf(`SELECT s.ttfb_ms FROM monitor_samples AS s LEFT JOIN monitor_runs AS r ON r.run_id = s.run_id %s ORDER BY s.ttfb_ms ASC LIMIT 1 OFFSET ?`, ttfbWhere)
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
			ttfbP95Query := fmt.Sprintf(`SELECT s.ttfb_ms FROM monitor_samples AS s LEFT JOIN monitor_runs AS r ON r.run_id = s.run_id %s ORDER BY s.ttfb_ms ASC LIMIT 1 OFFSET ?`, ttfbWhere)
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

// GetMonitorSampleFacets enumerates the distinct filter dimensions that actually exist in
// persisted raw samples within the given window.
//
// This is a presentation-only read model. It exists so a UI can build filter controls without
// scanning or aggregating sample data client-side. It never mutates raw samples, never
// summarizes them, and never replaces them as the source of truth. The caller is responsible
// for bounding the window; the limits here only bound the number of distinct values returned.
func (d *DB) GetMonitorSampleFacets(ctx context.Context, since, until time.Time, maxNodes, maxValues int) (*monitor.MonitorSampleFacets, error) {
	if maxNodes <= 0 {
		maxNodes = 500
	}
	if maxValues <= 0 {
		maxValues = 500
	}

	sinceUTC, untilUTC := since.UTC(), until.UTC()

	facets := &monitor.MonitorSampleFacets{
		Nodes:       make([]monitor.FacetNode, 0),
		Profiles:    make([]string, 0),
		ProbeTypes:  make([]string, 0),
		Targets:     make([]string, 0),
		WindowSince: sinceUTC,
		WindowUntil: untilUTC,
	}

	// Read maxNodes+1 rows so truncation is detectable without a second COUNT query.
	rows, err := d.db.QueryContext(ctx, `
		SELECT node_identity_key, node_key, MAX(display_name_snapshot), profile_id, COUNT(*)
		FROM monitor_samples
		WHERE timestamp >= ? AND timestamp <= ?
		GROUP BY node_identity_key, node_key, profile_id
		ORDER BY node_identity_key ASC, node_key ASC
		LIMIT ?
	`, sinceUTC, untilUTC, maxNodes+1)
	if err != nil {
		return nil, fmt.Errorf("query sample facet nodes: %w", err)
	}
	for rows.Next() {
		var n monitor.FacetNode
		if err := rows.Scan(&n.NodeIdentityKey, &n.NodeKey, &n.DisplayName, &n.ProfileID, &n.SampleCount); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan sample facet node: %w", err)
		}
		facets.Nodes = append(facets.Nodes, n)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate sample facet nodes: %w", err)
	}
	rows.Close()

	if len(facets.Nodes) > maxNodes {
		facets.Nodes = facets.Nodes[:maxNodes]
		facets.Truncated = true
	}

	profiles, profilesTruncated, err := d.distinctSampleValues(ctx, "profile_id", sinceUTC, untilUTC, maxValues)
	if err != nil {
		return nil, err
	}
	facets.Profiles = profiles

	probeTypes, probeTypesTruncated, err := d.distinctSampleValues(ctx, "probe_type", sinceUTC, untilUTC, maxValues)
	if err != nil {
		return nil, err
	}
	facets.ProbeTypes = probeTypes

	targets, targetsTruncated, err := d.distinctSampleValues(ctx, "target", sinceUTC, untilUTC, maxValues)
	if err != nil {
		return nil, err
	}
	facets.Targets = targets

	facets.Truncated = facets.Truncated || profilesTruncated || probeTypesTruncated || targetsTruncated

	return facets, nil
}

// distinctSampleValues returns the distinct non-empty values of a monitor_samples column.
// column must be an internal compile-time constant; it is never derived from user input.
func (d *DB) distinctSampleValues(ctx context.Context, column string, since, until time.Time, limit int) ([]string, bool, error) {
	switch column {
	case "profile_id", "probe_type", "target":
	default:
		return nil, false, fmt.Errorf("unsupported facet column: %s", column)
	}

	query := fmt.Sprintf(`
		SELECT DISTINCT %s FROM monitor_samples
		WHERE timestamp >= ? AND timestamp <= ? AND %s <> ''
		ORDER BY %s ASC
		LIMIT ?
	`, column, column, column)

	rows, err := d.db.QueryContext(ctx, query, since, until, limit+1)
	if err != nil {
		return nil, false, fmt.Errorf("query sample facet %s: %w", column, err)
	}
	defer rows.Close()

	values := make([]string, 0)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, false, fmt.Errorf("scan sample facet %s: %w", column, err)
		}
		values = append(values, v)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate sample facet %s: %w", column, err)
	}

	truncated := len(values) > limit
	if truncated {
		values = values[:limit]
	}
	return values, truncated, nil
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
	cutoff, err := retentionCutoff(req, time.Now().UTC())
	if err != nil {
		return nil, err
	}

	const batchSize = 500
	var totalSamplesDeleted int64
	var batchNum int

	// Prune samples in bounded batches
	for {
		select {
		case <-ctx.Done():
			partial := totalSamplesDeleted > 0
			partialResult := &monitor.RetentionResult{
				Policy:         req.Policy,
				Cutoff:         cutoff,
				SamplesDeleted: totalSamplesDeleted,
				RunsDeleted:    0,
				DurationMs:     time.Since(start).Milliseconds(),
				Partial:        partial,
				ErrorMessage:   ctx.Err().Error(),
			}
			return partialResult, &monitor.RetentionError{
				Result: partialResult,
				Err:    ctx.Err(),
			}
		default:
		}

		batchNum++
		if d.testBatchFailAt > 0 && batchNum == d.testBatchFailAt {
			faultErr := fmt.Errorf("injected fault on batch %d", batchNum)
			partial := totalSamplesDeleted > 0
			partialResult := &monitor.RetentionResult{
				Policy:         req.Policy,
				Cutoff:         cutoff,
				SamplesDeleted: totalSamplesDeleted,
				RunsDeleted:    0,
				DurationMs:     time.Since(start).Milliseconds(),
				Partial:        partial,
				ErrorMessage:   faultErr.Error(),
			}
			return partialResult, &monitor.RetentionError{
				Result: partialResult,
				Err:    faultErr,
			}
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
			partial := totalSamplesDeleted > 0
			partialResult := &monitor.RetentionResult{
				Policy:         req.Policy,
				Cutoff:         cutoff,
				SamplesDeleted: totalSamplesDeleted,
				RunsDeleted:    0,
				DurationMs:     time.Since(start).Milliseconds(),
				Partial:        partial,
				ErrorMessage:   err.Error(),
			}
			return partialResult, &monitor.RetentionError{
				Result: partialResult,
				Err:    fmt.Errorf("prune sample batch %d: %w", batchNum, err),
			}
		}

		totalSamplesDeleted += batchDeleted
		if batchDeleted < batchSize {
			break
		}

		// Briefly pause to yield single-writer lock to concurrent probe workers
		time.Sleep(2 * time.Millisecond)
	}

	// Prune orphaned terminal runs older than cutoff with no remaining samples.
	// Strictly preserve 'running' (in-flight) and 'skipped' (intentional no-op runs with 0 samples).
	var runsDeleted int64
	err = func() error {
		d.mu.Lock()
		defer d.mu.Unlock()

		res, err := d.db.ExecContext(ctx, `
			DELETE FROM monitor_runs
			WHERE scheduled_at < ?
			  AND status IN ('completed', 'failed', 'partial_failed', 'resource_limited')
			  AND run_id NOT IN (SELECT DISTINCT run_id FROM monitor_samples)
		`, cutoff)
		if err != nil {
			return err
		}
		runsDeleted, err = res.RowsAffected()
		return err
	}()
	if err != nil {
		partial := totalSamplesDeleted > 0 || runsDeleted > 0
		partialResult := &monitor.RetentionResult{
			Policy:         req.Policy,
			Cutoff:         cutoff,
			SamplesDeleted: totalSamplesDeleted,
			RunsDeleted:    runsDeleted,
			DurationMs:     time.Since(start).Milliseconds(),
			Partial:        partial,
			ErrorMessage:   err.Error(),
		}
		return partialResult, &monitor.RetentionError{
			Result: partialResult,
			Err:    fmt.Errorf("prune orphaned runs: %w", err),
		}
	}

	return &monitor.RetentionResult{
		Policy:         req.Policy,
		Cutoff:         cutoff,
		SamplesDeleted: totalSamplesDeleted,
		RunsDeleted:    runsDeleted,
		DurationMs:     time.Since(start).Milliseconds(),
		Partial:        false,
	}, nil
}
