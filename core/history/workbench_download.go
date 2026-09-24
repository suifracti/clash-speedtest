package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrWorkbenchDownloadAttemptNotFound = errors.New("Workbench download attempt not found")

const WorkbenchDownloadSource = "workbench_manual_download"

type WorkbenchDownloadRuleSnapshot struct {
	RuleVersion       int    `json:"rule_version"`
	TargetURL         string `json:"target_url"`
	Method            string `json:"method"`
	MaximumBytes      int64  `json:"maximum_bytes"`
	MaximumDurationNS int64  `json:"maximum_duration_ns"`
	SampleEveryBytes  int64  `json:"sample_every_bytes"`
	SampleEveryNS     int64  `json:"sample_every_ns"`
}

type WorkbenchDownloadSample struct {
	ElapsedNS       int64    `json:"elapsed_ns"`
	IntervalNS      int64    `json:"interval_ns"`
	DeltaBytes      int64    `json:"delta_bytes"`
	CumulativeBytes int64    `json:"cumulative_bytes"`
	SpeedMbps       *float64 `json:"speed_mbps,omitempty"`
}

type WorkbenchDownloadMeasurement struct {
	Outcome      string                    `json:"outcome"`
	HTTPStatus   *int                      `json:"http_status,omitempty"`
	BytesRead    int64                     `json:"bytes_read"`
	StartedAt    time.Time                 `json:"started_at"`
	FinishedAt   time.Time                 `json:"finished_at"`
	DurationNS   int64                     `json:"duration_ns"`
	FailurePhase string                    `json:"failure_phase,omitempty"`
	ErrorMessage string                    `json:"error_message,omitempty"`
	Samples      []WorkbenchDownloadSample `json:"samples"`
}

type WorkbenchDownloadAttempt struct {
	AttemptID         string                        `json:"attempt_id"`
	RequestID         string                        `json:"request_id"`
	ProfileID         string                        `json:"profile_id"`
	NodeKey           string                        `json:"node_key"`
	NodeIdentityKey   string                        `json:"node_identity_key"`
	ConfigRevisionKey string                        `json:"config_revision_key"`
	DisplayName       string                        `json:"display_name"`
	NodeType          string                        `json:"node_type"`
	Source            string                        `json:"source"`
	RequestedAt       time.Time                     `json:"requested_at"`
	StartedAt         *time.Time                    `json:"started_at,omitempty"`
	FinishedAt        *time.Time                    `json:"finished_at,omitempty"`
	ExecutionState    string                        `json:"execution_state"`
	PersistenceState  string                        `json:"persistence_state"`
	PersistenceError  string                        `json:"persistence_error,omitempty"`
	Rule              WorkbenchDownloadRuleSnapshot `json:"rule"`
	Result            *WorkbenchDownloadMeasurement `json:"result,omitempty"`
}

type WorkbenchDownloadFilter struct {
	ProfileID         string
	NodeKey           string
	NodeIdentityKey   string
	ConfigRevisionKey string
	Since             *time.Time
	Until             *time.Time
	Limit             int
	BeforeAt          *time.Time
	BeforeAttemptID   string
}

type WorkbenchDownloadQueryResult struct {
	Attempts []*WorkbenchDownloadAttempt
	HasMore  bool
}

func (d *DB) CreateWorkbenchDownloadAttempt(ctx context.Context, attempt *WorkbenchDownloadAttempt) error {
	if attempt == nil || strings.TrimSpace(attempt.AttemptID) == "" || strings.TrimSpace(attempt.RequestID) == "" {
		return fmt.Errorf("download attempt and request IDs are required")
	}
	if attempt.ProfileID == "" || attempt.NodeKey == "" || attempt.NodeIdentityKey == "" || attempt.ConfigRevisionKey == "" {
		return fmt.Errorf("download identity scope is required")
	}
	ruleJSON, err := json.Marshal(attempt.Rule)
	if err != nil {
		return fmt.Errorf("encode download rule snapshot: %w", err)
	}
	if attempt.RequestedAt.IsZero() {
		attempt.RequestedAt = time.Now().UTC()
	}
	if attempt.Source == "" {
		attempt.Source = WorkbenchDownloadSource
	}
	if attempt.ExecutionState == "" {
		attempt.ExecutionState = "queued"
	}
	if attempt.PersistenceState == "" {
		attempt.PersistenceState = "not_started"
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err = d.db.ExecContext(ctx, `INSERT INTO workbench_download_attempts (
		attempt_id,request_id,profile_id,node_key,node_identity_key,config_revision_key,
		display_name,node_type,source,requested_at,execution_state,persistence_state,rule_snapshot_json
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, attempt.AttemptID, attempt.RequestID, attempt.ProfileID, attempt.NodeKey,
		attempt.NodeIdentityKey, attempt.ConfigRevisionKey, attempt.DisplayName, attempt.NodeType, attempt.Source,
		attempt.RequestedAt.UTC(), attempt.ExecutionState, attempt.PersistenceState, string(ruleJSON))
	if err != nil {
		return fmt.Errorf("create Workbench download attempt %s: %w", attempt.AttemptID, err)
	}
	return nil
}

func (d *DB) BeginWorkbenchDownloadAttempt(ctx context.Context, attemptID string, startedAt time.Time) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	res, err := d.db.ExecContext(ctx, `UPDATE workbench_download_attempts SET execution_state='running',started_at=? WHERE attempt_id=? AND execution_state='queued'`, startedAt.UTC(), attemptID)
	if err != nil {
		return fmt.Errorf("start Workbench download attempt %s: %w", attemptID, err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("Workbench download attempt %s is not queued", attemptID)
	}
	return nil
}

func (d *DB) RequestWorkbenchDownloadCancellation(ctx context.Context, attemptID string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	res, err := d.db.ExecContext(ctx, `UPDATE workbench_download_attempts SET execution_state='cancelling' WHERE attempt_id=? AND execution_state='running'`, attemptID)
	if err != nil {
		return false, fmt.Errorf("cancel Workbench download attempt %s: %w", attemptID, err)
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (d *DB) StageWorkbenchDownloadResult(ctx context.Context, attemptID, executionState string, measurement WorkbenchDownloadMeasurement) error {
	encoded, err := json.Marshal(measurement)
	if err != nil {
		return fmt.Errorf("encode Workbench download result: %w", err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	res, err := d.db.ExecContext(ctx, `UPDATE workbench_download_attempts SET finished_at=?,execution_state=?,persistence_state='saving',persistence_error='',staged_result_json=? WHERE attempt_id=? AND execution_state IN ('running','cancelling')`, measurement.FinishedAt.UTC(), executionState, string(encoded), attemptID)
	if err != nil {
		return fmt.Errorf("stage Workbench download result %s: %w", attemptID, err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("Workbench download attempt %s is not active", attemptID)
	}
	return nil
}

func (d *DB) MarkWorkbenchDownloadStageFailed(ctx context.Context, attemptID, executionState string, finishedAt time.Time, safeMessage string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	res, err := d.db.ExecContext(ctx, `UPDATE workbench_download_attempts SET finished_at=?,execution_state=?,persistence_state='failed',persistence_error=? WHERE attempt_id=? AND execution_state IN ('running','cancelling') AND staged_result_json=''`, finishedAt.UTC(), executionState, safeMessage, attemptID)
	if err != nil {
		return fmt.Errorf("mark Workbench download stage failure %s: %w", attemptID, err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("Workbench download attempt %s could not be marked after staging failure", attemptID)
	}
	return nil
}

func (d *DB) MarkWorkbenchDownloadSaveFailed(ctx context.Context, attemptID, safeMessage string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.ExecContext(ctx, `UPDATE workbench_download_attempts SET persistence_state='failed',persistence_error=? WHERE attempt_id=? AND staged_result_json<>'' AND persistence_state<>'saved'`, safeMessage, attemptID)
	if err != nil {
		return fmt.Errorf("mark Workbench download save failed %s: %w", attemptID, err)
	}
	return nil
}

func (d *DB) CommitWorkbenchDownloadResult(ctx context.Context, attemptID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin Workbench download commit: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var staged string
	if err := tx.QueryRowContext(ctx, `SELECT staged_result_json FROM workbench_download_attempts WHERE attempt_id=?`, attemptID).Scan(&staged); err != nil {
		return fmt.Errorf("read staged Workbench download result %s: %w", attemptID, err)
	}
	if staged == "" {
		return fmt.Errorf("Workbench download attempt %s has no staged result", attemptID)
	}
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT result_json FROM workbench_download_results WHERE attempt_id=?`, attemptID).Scan(&existing)
	if err == sql.ErrNoRows {
		if _, err := tx.ExecContext(ctx, `INSERT INTO workbench_download_results(attempt_id,result_json,saved_at) VALUES(?,?,?)`, attemptID, staged, time.Now().UTC()); err != nil {
			return fmt.Errorf("commit Workbench download result %s: %w", attemptID, err)
		}
	} else if err != nil {
		return fmt.Errorf("check Workbench download result %s: %w", attemptID, err)
	} else if existing != staged {
		return fmt.Errorf("Workbench download attempt %s already has a different committed result", attemptID)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workbench_download_attempts SET persistence_state='saved',persistence_error='' WHERE attempt_id=?`, attemptID); err != nil {
		return fmt.Errorf("mark Workbench download saved %s: %w", attemptID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Workbench download result %s: %w", attemptID, err)
	}
	return nil
}

func (d *DB) ReconcileWorkbenchDownloadAttempts(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin Workbench download recovery: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE workbench_download_attempts SET execution_state='interrupted',persistence_state='not_applicable',persistence_error='' WHERE execution_state IN ('queued','running','cancelling') AND staged_result_json=''`); err != nil {
		return fmt.Errorf("mark interrupted Workbench downloads: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workbench_download_attempts SET persistence_state='saved',persistence_error='' WHERE EXISTS (SELECT 1 FROM workbench_download_results r WHERE r.attempt_id=workbench_download_attempts.attempt_id)`); err != nil {
		return fmt.Errorf("reconcile saved Workbench downloads: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workbench_download_attempts SET persistence_state='failed',persistence_error='应用退出时保存未完成；可沿用原 attempt 重试保存' WHERE staged_result_json<>'' AND NOT EXISTS (SELECT 1 FROM workbench_download_results r WHERE r.attempt_id=workbench_download_attempts.attempt_id)`); err != nil {
		return fmt.Errorf("reconcile staged Workbench downloads: %w", err)
	}
	return tx.Commit()
}

func (d *DB) GetWorkbenchDownloadAttemptByRequestID(ctx context.Context, requestID string) (*WorkbenchDownloadAttempt, error) {
	return d.getWorkbenchDownloadAttempt(ctx, `a.request_id=?`, requestID)
}

func (d *DB) GetWorkbenchDownloadAttempt(ctx context.Context, attemptID string, filter WorkbenchDownloadFilter) (*WorkbenchDownloadAttempt, error) {
	if err := validateWorkbenchDownloadFilter(filter); err != nil {
		return nil, err
	}
	return d.getWorkbenchDownloadAttempt(ctx, `a.attempt_id=? AND a.profile_id=? AND a.node_key=? AND a.node_identity_key=? AND a.config_revision_key=?`, attemptID, filter.ProfileID, filter.NodeKey, filter.NodeIdentityKey, filter.ConfigRevisionKey)
}

func (d *DB) getWorkbenchDownloadAttempt(ctx context.Context, where string, args ...any) (*WorkbenchDownloadAttempt, error) {
	attempt, err := scanWorkbenchDownloadAttempt(d.db.QueryRowContext(ctx, workbenchDownloadSelect+` WHERE `+where, args...))
	if err == sql.ErrNoRows {
		return nil, ErrWorkbenchDownloadAttemptNotFound
	}
	return attempt, err
}

func (d *DB) QueryWorkbenchDownloadAttempts(ctx context.Context, filter WorkbenchDownloadFilter) (*WorkbenchDownloadQueryResult, error) {
	if err := validateWorkbenchDownloadFilter(filter); err != nil {
		return nil, err
	}
	since, until, err := normalizeLatencyWindow(filter.Since, filter.Until)
	if err != nil {
		return nil, err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	if (filter.BeforeAt == nil) != (filter.BeforeAttemptID == "") {
		return nil, fmt.Errorf("Workbench download history cursor requires before_at and before_attempt_id")
	}
	where := `a.profile_id=? AND a.node_key=? AND a.node_identity_key=? AND a.config_revision_key=?`
	args := []any{filter.ProfileID, filter.NodeKey, filter.NodeIdentityKey, filter.ConfigRevisionKey}
	if since != nil {
		where += ` AND COALESCE(a.finished_at,a.started_at,a.requested_at)>=? AND COALESCE(a.finished_at,a.started_at,a.requested_at)<?`
		args = append(args, *since, *until)
	}
	if filter.BeforeAt != nil {
		where += ` AND (COALESCE(a.finished_at,a.started_at,a.requested_at)<? OR (COALESCE(a.finished_at,a.started_at,a.requested_at)=? AND a.attempt_id<?))`
		args = append(args, filter.BeforeAt.UTC(), filter.BeforeAt.UTC(), filter.BeforeAttemptID)
	}
	args = append(args, limit+1)
	rows, err := d.db.QueryContext(ctx, workbenchDownloadSelect+` WHERE `+where+` ORDER BY COALESCE(a.finished_at,a.started_at,a.requested_at) DESC,a.attempt_id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("query Workbench download history: %w", err)
	}
	defer rows.Close()
	result := &WorkbenchDownloadQueryResult{Attempts: make([]*WorkbenchDownloadAttempt, 0, limit)}
	for rows.Next() {
		attempt, err := scanWorkbenchDownloadAttempt(rows)
		if err != nil {
			return nil, fmt.Errorf("scan Workbench download attempt: %w", err)
		}
		result.Attempts = append(result.Attempts, attempt)
		if len(result.Attempts) > limit {
			result.HasMore = true
			result.Attempts = result.Attempts[:limit]
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read Workbench download history: %w", err)
	}
	return result, nil
}

const workbenchDownloadSelect = `SELECT a.attempt_id,a.request_id,a.profile_id,a.node_key,a.node_identity_key,a.config_revision_key,
	a.display_name,a.node_type,a.source,a.requested_at,a.started_at,a.finished_at,a.execution_state,
	CASE WHEN r.attempt_id IS NOT NULL THEN 'saved' ELSE a.persistence_state END,
	CASE WHEN r.attempt_id IS NOT NULL THEN '' ELSE a.persistence_error END,
	a.rule_snapshot_json,COALESCE(r.result_json,a.staged_result_json)
	FROM workbench_download_attempts a LEFT JOIN workbench_download_results r ON r.attempt_id=a.attempt_id`

type workbenchDownloadScanner interface{ Scan(dest ...any) error }

func scanWorkbenchDownloadAttempt(scanner workbenchDownloadScanner) (*WorkbenchDownloadAttempt, error) {
	attempt := &WorkbenchDownloadAttempt{}
	var started, finished sql.NullTime
	var ruleJSON, resultJSON string
	if err := scanner.Scan(&attempt.AttemptID, &attempt.RequestID, &attempt.ProfileID, &attempt.NodeKey,
		&attempt.NodeIdentityKey, &attempt.ConfigRevisionKey, &attempt.DisplayName, &attempt.NodeType, &attempt.Source,
		&attempt.RequestedAt, &started, &finished, &attempt.ExecutionState, &attempt.PersistenceState,
		&attempt.PersistenceError, &ruleJSON, &resultJSON); err != nil {
		return nil, err
	}
	attempt.RequestedAt = attempt.RequestedAt.UTC()
	if started.Valid {
		value := started.Time.UTC()
		attempt.StartedAt = &value
	}
	if finished.Valid {
		value := finished.Time.UTC()
		attempt.FinishedAt = &value
	}
	if err := json.Unmarshal([]byte(ruleJSON), &attempt.Rule); err != nil {
		return nil, fmt.Errorf("decode Workbench download rule: %w", err)
	}
	if resultJSON != "" {
		var result WorkbenchDownloadMeasurement
		if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
			return nil, fmt.Errorf("decode Workbench download result: %w", err)
		}
		attempt.Result = &result
	}
	return attempt, nil
}

func validateWorkbenchDownloadFilter(filter WorkbenchDownloadFilter) error {
	if strings.TrimSpace(filter.ProfileID) == "" || strings.TrimSpace(filter.NodeKey) == "" || strings.TrimSpace(filter.NodeIdentityKey) == "" || strings.TrimSpace(filter.ConfigRevisionKey) == "" {
		return fmt.Errorf("Workbench download history requires profile, node, stable identity, and revision")
	}
	return nil
}
