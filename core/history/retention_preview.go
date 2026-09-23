package history

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
)

func retentionCutoff(req monitor.RetentionRequest, now time.Time) (time.Time, error) {
	if req.Policy == "" || req.Policy == monitor.RetentionKeepAll {
		return time.Time{}, nil
	}
	if req.CutoffTime != nil && !req.CutoffTime.IsZero() {
		cutoff := req.CutoffTime.UTC()
		if cutoff.After(now) {
			return time.Time{}, monitor.WrapValidationError(monitor.ErrFutureCutoff)
		}
		switch req.Policy {
		case monitor.Retention30d, monitor.Retention90d, monitor.Retention180d, monitor.RetentionCustom:
			return cutoff, nil
		default:
			return time.Time{}, monitor.WrapValidationError(monitor.ErrInvalidRetentionPolicy)
		}
	}
	var days int
	switch req.Policy {
	case monitor.Retention30d:
		days = 30
	case monitor.Retention90d:
		days = 90
	case monitor.Retention180d:
		days = 180
	case monitor.RetentionCustom:
		if req.CustomDays <= 0 || req.CustomDays > 36500 {
			return time.Time{}, monitor.WrapValidationError(monitor.ErrInvalidCustomDays)
		}
		days = req.CustomDays
	default:
		return time.Time{}, monitor.WrapValidationError(monitor.ErrInvalidRetentionPolicy)
	}
	cutoff := now.AddDate(0, 0, -days)
	return cutoff, nil
}

// PreviewRetention counts eligible raw Monitor rows without writing anything.
// A caller can pass the returned cutoff to ApplyRetention to freeze the date.
func (d *DB) PreviewRetention(ctx context.Context, req monitor.RetentionRequest) (*monitor.RetentionPreview, error) {
	if d == nil || d.db == nil {
		return nil, fmt.Errorf("history database is not initialized")
	}
	policy := req.Policy
	if policy == "" {
		policy = monitor.RetentionKeepAll
	}
	cutoff, err := retentionCutoff(req, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	preview := &monitor.RetentionPreview{Policy: policy, Cutoff: cutoff}
	if policy == monitor.RetentionKeepAll {
		return preview, nil
	}
	tx, err := d.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin retention preview: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// The second count matches the post-sample-deletion orphan predicate in
	// ApplyRetention: only runs with no samples at or after cutoff are eligible.
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM monitor_samples WHERE timestamp < ?`, cutoff).Scan(&preview.SamplesToDelete); err != nil {
		return nil, fmt.Errorf("count expired monitor samples: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM monitor_runs r
		WHERE r.scheduled_at < ?
		  AND r.status IN ('completed', 'failed', 'partial_failed', 'resource_limited')
		  AND NOT EXISTS (
			SELECT 1 FROM monitor_samples s WHERE s.run_id = r.run_id AND s.timestamp >= ?
		  )
	`, cutoff, cutoff).Scan(&preview.RunsToDelete); err != nil {
		return nil, fmt.Errorf("count orphaned monitor runs: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("finish retention preview: %w", err)
	}
	return preview, nil
}

func (s *Store) PreviewRetention(ctx context.Context, req monitor.RetentionRequest) (*monitor.RetentionPreview, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	return s.db.PreviewRetention(ctx, req)
}
