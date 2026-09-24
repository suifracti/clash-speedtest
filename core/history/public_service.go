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

var ErrPublicServiceAttemptNotFound = errors.New("public-service attempt not found")

// PublicServiceRuleSnapshot freezes the fixed service rule used for an
// attempt. It intentionally contains no request credentials or response body.
type PublicServiceRuleSnapshot struct {
	ServiceID        string `json:"service_id"`
	Name             string `json:"name"`
	RuleVersion      int    `json:"rule_version"`
	TargetURL        string `json:"target_url"`
	Method           string `json:"method"`
	SuccessCriterion string `json:"success_criterion"`
	RedirectPolicy   string `json:"redirect_policy"`
	TimeoutSeconds   int64  `json:"timeout_seconds"`
	MaximumBodyBytes int64  `json:"maximum_body_bytes"`
	Accept           string `json:"accept,omitempty"`
	APIVersionHeader string `json:"api_version_header,omitempty"`
}

// PublicServiceMeasurement is the bounded, body-free result of one request.
type PublicServiceMeasurement struct {
	Outcome      string    `json:"outcome"`
	HTTPStatus   *int      `json:"http_status,omitempty"`
	BytesRead    int64     `json:"bytes_read"`
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
	DurationMs   int64     `json:"duration_ms"`
	FailurePhase string    `json:"failure_phase,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
}

// PublicServiceAttempt is one explicitly requested Workbench service check.
// The saved result table is the commit marker; staged results remain retryable
// under the same AttemptID and are never remeasured during recovery.
type PublicServiceAttempt struct {
	AttemptID         string                    `json:"attempt_id"`
	RequestID         string                    `json:"request_id"`
	ProfileID         string                    `json:"profile_id"`
	NodeKey           string                    `json:"node_key"`
	NodeIdentityKey   string                    `json:"node_identity_key"`
	ConfigRevisionKey string                    `json:"config_revision_key"`
	DisplayName       string                    `json:"display_name"`
	NodeType          string                    `json:"node_type"`
	Source            string                    `json:"source"`
	ServiceID         string                    `json:"service_id"`
	Rule              PublicServiceRuleSnapshot `json:"rule"`
	RequestedAt       time.Time                 `json:"requested_at"`
	StartedAt         *time.Time                `json:"started_at,omitempty"`
	FinishedAt        *time.Time                `json:"finished_at,omitempty"`
	ExecutionState    string                    `json:"execution_state"`
	PersistenceState  string                    `json:"persistence_state"`
	PersistenceError  string                    `json:"persistence_error,omitempty"`
	Result            *PublicServiceMeasurement `json:"result,omitempty"`
}

type PublicServiceFilter struct {
	ProfileID         string
	NodeKey           string
	NodeIdentityKey   string
	ConfigRevisionKey string
	ServiceID         string
	Since             *time.Time
	Until             *time.Time
	Limit             int
	BeforeAt          *time.Time
	BeforeAttemptID   string
}

type PublicServiceQueryResult struct {
	Attempts []*PublicServiceAttempt
	HasMore  bool
}

func (d *DB) CreatePublicServiceAttempt(ctx context.Context, attempt *PublicServiceAttempt) error {
	if attempt == nil || strings.TrimSpace(attempt.AttemptID) == "" || strings.TrimSpace(attempt.RequestID) == "" {
		return fmt.Errorf("public-service attempt and request IDs are required")
	}
	if attempt.ProfileID == "" || attempt.NodeKey == "" || attempt.NodeIdentityKey == "" || attempt.ConfigRevisionKey == "" || attempt.ServiceID == "" {
		return fmt.Errorf("public-service identity and service scope are required")
	}
	ruleJSON, err := json.Marshal(attempt.Rule)
	if err != nil {
		return fmt.Errorf("encode public-service rule snapshot: %w", err)
	}
	if attempt.RequestedAt.IsZero() {
		attempt.RequestedAt = time.Now().UTC()
	}
	if attempt.Source == "" {
		attempt.Source = "workbench_public_service"
	}
	if attempt.ExecutionState == "" {
		attempt.ExecutionState = "queued"
	}
	if attempt.PersistenceState == "" {
		attempt.PersistenceState = "not_started"
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	_, err = d.db.ExecContext(ctx, `
		INSERT INTO workbench_public_service_attempts (
			attempt_id, request_id, profile_id, node_key, node_identity_key, config_revision_key,
			display_name, node_type, source, service_id, requested_at, execution_state,
			persistence_state, rule_snapshot_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, attempt.AttemptID, attempt.RequestID, attempt.ProfileID, attempt.NodeKey,
		attempt.NodeIdentityKey, attempt.ConfigRevisionKey, attempt.DisplayName, attempt.NodeType,
		attempt.Source, attempt.ServiceID, attempt.RequestedAt.UTC(), attempt.ExecutionState,
		attempt.PersistenceState, string(ruleJSON))
	if err != nil {
		return fmt.Errorf("create public-service attempt %s: %w", attempt.AttemptID, err)
	}
	return nil
}

func (d *DB) BeginPublicServiceAttempt(ctx context.Context, attemptID string, startedAt time.Time) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	res, err := d.db.ExecContext(ctx, `UPDATE workbench_public_service_attempts
		SET execution_state='running', started_at=? WHERE attempt_id=? AND execution_state='queued'`, startedAt.UTC(), attemptID)
	if err != nil {
		return fmt.Errorf("start public-service attempt %s: %w", attemptID, err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("public-service attempt %s is not queued", attemptID)
	}
	return nil
}

func (d *DB) RequestPublicServiceCancellation(ctx context.Context, attemptID string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	res, err := d.db.ExecContext(ctx, `UPDATE workbench_public_service_attempts
		SET execution_state='cancelling' WHERE attempt_id=? AND execution_state='running'`, attemptID)
	if err != nil {
		return false, fmt.Errorf("request public-service cancellation %s: %w", attemptID, err)
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (d *DB) StagePublicServiceResult(ctx context.Context, attemptID, executionState string, measurement PublicServiceMeasurement) error {
	resultJSON, err := json.Marshal(measurement)
	if err != nil {
		return fmt.Errorf("encode public-service result: %w", err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	res, err := d.db.ExecContext(ctx, `UPDATE workbench_public_service_attempts SET
		finished_at=?, execution_state=?, persistence_state='saving', persistence_error='', staged_result_json=?
		WHERE attempt_id=? AND execution_state IN ('running','cancelling')`,
		measurement.FinishedAt.UTC(), executionState, string(resultJSON), attemptID)
	if err != nil {
		return fmt.Errorf("stage public-service result %s: %w", attemptID, err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("public-service attempt %s is not active", attemptID)
	}
	return nil
}

func (d *DB) MarkPublicServiceStageFailed(ctx context.Context, attemptID, executionState string, finishedAt time.Time, safeMessage string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	res, err := d.db.ExecContext(ctx, `UPDATE workbench_public_service_attempts SET
		finished_at=?, execution_state=?, persistence_state='failed', persistence_error=?
		WHERE attempt_id=? AND execution_state IN ('running','cancelling') AND staged_result_json=''`,
		finishedAt.UTC(), executionState, safeMessage, attemptID)
	if err != nil {
		return fmt.Errorf("mark public-service result staging failure %s: %w", attemptID, err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("public-service attempt %s could not be marked after staging failure", attemptID)
	}
	return nil
}

func (d *DB) MarkPublicServiceSaveFailed(ctx context.Context, attemptID, safeMessage string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.ExecContext(ctx, `UPDATE workbench_public_service_attempts SET
		persistence_state='failed', persistence_error=?
		WHERE attempt_id=? AND staged_result_json<>'' AND persistence_state<>'saved'`, safeMessage, attemptID)
	if err != nil {
		return fmt.Errorf("mark public-service save failed %s: %w", attemptID, err)
	}
	return nil
}

// CommitPublicServiceResult atomically moves the staged result into the
// immutable fact table and updates the attempt's save state. Repeating it for
// the same attempt is idempotent.
func (d *DB) CommitPublicServiceResult(ctx context.Context, attemptID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin public-service result transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var staged string
	if err := tx.QueryRowContext(ctx, `SELECT staged_result_json FROM workbench_public_service_attempts WHERE attempt_id=?`, attemptID).Scan(&staged); err != nil {
		return fmt.Errorf("read staged public-service result %s: %w", attemptID, err)
	}
	if staged == "" {
		return fmt.Errorf("public-service attempt %s has no staged result", attemptID)
	}
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT result_json FROM workbench_public_service_results WHERE attempt_id=?`, attemptID).Scan(&existing)
	if err == sql.ErrNoRows {
		if _, err := tx.ExecContext(ctx, `INSERT INTO workbench_public_service_results(attempt_id,result_json,saved_at) VALUES(?,?,?)`, attemptID, staged, time.Now().UTC()); err != nil {
			return fmt.Errorf("commit public-service result %s: %w", attemptID, err)
		}
	} else if err != nil {
		return fmt.Errorf("check public-service result %s: %w", attemptID, err)
	} else if existing != staged {
		return fmt.Errorf("public-service attempt %s already has a different committed result", attemptID)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workbench_public_service_attempts SET persistence_state='saved', persistence_error='' WHERE attempt_id=?`, attemptID); err != nil {
		return fmt.Errorf("mark public-service result saved %s: %w", attemptID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit public-service result transaction %s: %w", attemptID, err)
	}
	return nil
}

func (d *DB) ReconcilePublicServiceAttempts(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin public-service recovery: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE workbench_public_service_attempts SET
		execution_state='interrupted', persistence_state='not_applicable', persistence_error=''
		WHERE execution_state IN ('queued','running','cancelling') AND staged_result_json=''`); err != nil {
		return fmt.Errorf("mark interrupted public-service attempts: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workbench_public_service_attempts SET
		persistence_state='saved', persistence_error=''
		WHERE EXISTS (SELECT 1 FROM workbench_public_service_results r WHERE r.attempt_id=workbench_public_service_attempts.attempt_id)`); err != nil {
		return fmt.Errorf("reconcile saved public-service attempts: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE workbench_public_service_attempts SET
		persistence_state='failed', persistence_error='应用退出时保存未完成；可沿用原 attempt 重试保存'
		WHERE staged_result_json<>'' AND NOT EXISTS (
			SELECT 1 FROM workbench_public_service_results r WHERE r.attempt_id=workbench_public_service_attempts.attempt_id
		)`); err != nil {
		return fmt.Errorf("reconcile staged public-service attempts: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit public-service recovery: %w", err)
	}
	return nil
}

func (d *DB) GetPublicServiceAttemptByRequestID(ctx context.Context, requestID string) (*PublicServiceAttempt, error) {
	return d.getPublicServiceAttempt(ctx, `a.request_id=?`, requestID)
}

func (d *DB) GetPublicServiceAttempt(ctx context.Context, attemptID string, filter PublicServiceFilter) (*PublicServiceAttempt, error) {
	if err := validatePublicServiceFilter(filter, true); err != nil {
		return nil, err
	}
	return d.getPublicServiceAttempt(ctx, `a.attempt_id=? AND a.profile_id=? AND a.node_key=? AND a.node_identity_key=? AND a.config_revision_key=? AND a.service_id=?`,
		attemptID, filter.ProfileID, filter.NodeKey, filter.NodeIdentityKey, filter.ConfigRevisionKey, filter.ServiceID)
}

func (d *DB) getPublicServiceAttempt(ctx context.Context, where string, args ...any) (*PublicServiceAttempt, error) {
	row := d.db.QueryRowContext(ctx, publicServiceAttemptSelect+` WHERE `+where, args...)
	attempt, err := scanPublicServiceAttempt(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrPublicServiceAttemptNotFound
		}
		return nil, err
	}
	return attempt, nil
}

func (d *DB) QueryPublicServiceAttempts(ctx context.Context, filter PublicServiceFilter) (*PublicServiceQueryResult, error) {
	if err := validatePublicServiceFilter(filter, false); err != nil {
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
	where := `a.profile_id=? AND a.node_key=? AND a.node_identity_key=? AND a.config_revision_key=? AND a.service_id=?`
	args := []any{filter.ProfileID, filter.NodeKey, filter.NodeIdentityKey, filter.ConfigRevisionKey, filter.ServiceID}
	if filter.ServiceID == "" {
		where = `a.profile_id=? AND a.node_key=? AND a.node_identity_key=? AND a.config_revision_key=?`
		args = []any{filter.ProfileID, filter.NodeKey, filter.NodeIdentityKey, filter.ConfigRevisionKey}
	}
	if since != nil {
		where += ` AND COALESCE(a.started_at,a.requested_at)>=? AND COALESCE(a.started_at,a.requested_at)<?`
		args = append(args, *since, *until)
	}
	if (filter.BeforeAt == nil) != (filter.BeforeAttemptID == "") {
		return nil, fmt.Errorf("public-service history cursor requires before_at and before_attempt_id")
	}
	if filter.BeforeAt != nil {
		where += ` AND (COALESCE(a.finished_at,a.started_at,a.requested_at)<? OR (COALESCE(a.finished_at,a.started_at,a.requested_at)=? AND a.attempt_id<?))`
		args = append(args, filter.BeforeAt.UTC(), filter.BeforeAt.UTC(), filter.BeforeAttemptID)
	}
	args = append(args, limit+1)
	rows, err := d.db.QueryContext(ctx, publicServiceAttemptSelect+` WHERE `+where+
		` ORDER BY COALESCE(a.finished_at,a.started_at,a.requested_at) DESC,a.attempt_id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("query public-service attempts: %w", err)
	}
	defer rows.Close()
	result := &PublicServiceQueryResult{Attempts: make([]*PublicServiceAttempt, 0, limit)}
	for rows.Next() {
		attempt, err := scanPublicServiceAttempt(rows)
		if err != nil {
			return nil, fmt.Errorf("scan public-service attempt: %w", err)
		}
		result.Attempts = append(result.Attempts, attempt)
		if len(result.Attempts) > limit {
			result.HasMore = true
			result.Attempts = result.Attempts[:limit]
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read public-service attempts: %w", err)
	}
	return result, nil
}

const publicServiceAttemptSelect = `
	SELECT a.attempt_id,a.request_id,a.profile_id,a.node_key,a.node_identity_key,a.config_revision_key,
		a.display_name,a.node_type,a.source,a.service_id,a.requested_at,a.started_at,a.finished_at,
		a.execution_state,
		CASE WHEN r.attempt_id IS NOT NULL THEN 'saved' ELSE a.persistence_state END,
		CASE WHEN r.attempt_id IS NOT NULL THEN '' ELSE a.persistence_error END,
		a.rule_snapshot_json,COALESCE(r.result_json,a.staged_result_json)
	FROM workbench_public_service_attempts a
	LEFT JOIN workbench_public_service_results r ON r.attempt_id=a.attempt_id
`

type publicServiceScanner interface {
	Scan(dest ...any) error
}

func scanPublicServiceAttempt(scanner publicServiceScanner) (*PublicServiceAttempt, error) {
	attempt := &PublicServiceAttempt{}
	var started, finished sql.NullTime
	var ruleJSON, resultJSON string
	err := scanner.Scan(&attempt.AttemptID, &attempt.RequestID, &attempt.ProfileID, &attempt.NodeKey,
		&attempt.NodeIdentityKey, &attempt.ConfigRevisionKey, &attempt.DisplayName, &attempt.NodeType,
		&attempt.Source, &attempt.ServiceID, &attempt.RequestedAt, &started, &finished,
		&attempt.ExecutionState, &attempt.PersistenceState, &attempt.PersistenceError, &ruleJSON, &resultJSON)
	if err != nil {
		return nil, err
	}
	if started.Valid {
		value := started.Time.UTC()
		attempt.StartedAt = &value
	}
	if finished.Valid {
		value := finished.Time.UTC()
		attempt.FinishedAt = &value
	}
	attempt.RequestedAt = attempt.RequestedAt.UTC()
	if err := json.Unmarshal([]byte(ruleJSON), &attempt.Rule); err != nil {
		return nil, fmt.Errorf("decode public-service rule snapshot: %w", err)
	}
	if resultJSON != "" {
		var measurement PublicServiceMeasurement
		if err := json.Unmarshal([]byte(resultJSON), &measurement); err != nil {
			return nil, fmt.Errorf("decode public-service result: %w", err)
		}
		attempt.Result = &measurement
	}
	return attempt, nil
}

func validatePublicServiceFilter(filter PublicServiceFilter, requireService bool) error {
	if strings.TrimSpace(filter.ProfileID) == "" || strings.TrimSpace(filter.NodeKey) == "" ||
		strings.TrimSpace(filter.NodeIdentityKey) == "" || strings.TrimSpace(filter.ConfigRevisionKey) == "" {
		return fmt.Errorf("public-service history requires profile, node, stable identity, and revision")
	}
	if requireService && strings.TrimSpace(filter.ServiceID) == "" {
		return fmt.Errorf("public-service attempt lookup requires service_id")
	}
	return nil
}
