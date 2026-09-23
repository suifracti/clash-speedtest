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

func (d *DB) CreateLatencyBatch(ctx context.Context, batch *LatencyBatch) error {
	if batch == nil || strings.TrimSpace(batch.BatchID) == "" || strings.TrimSpace(batch.RequestID) == "" || len(batch.Items) == 0 {
		return fmt.Errorf("latency batch identity or selection is incomplete")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin latency batch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO workbench_latency_batches(batch_id,request_id,test_project,timeout_seconds,requested_at,state) VALUES(?,?,?,?,?,?)`, batch.BatchID, batch.RequestID, batch.TestProject, batch.TimeoutSeconds, batch.RequestedAt.UTC(), batch.State); err != nil {
		return fmt.Errorf("insert latency batch: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO workbench_latency_batch_items(item_id,batch_id,ordinal,profile_id,node_key,node_identity_key,config_revision_key,display_name,node_type,execution_state,persistence_state,attempt_id,requested_at,error_message,persistence_error,result_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return fmt.Errorf("prepare latency batch items: %w", err)
	}
	defer stmt.Close()
	for i := range batch.Items {
		item := &batch.Items[i]
		item.BatchID = batch.BatchID
		item.Ordinal = i
		if item.ItemID == "" || item.ProfileID == "" || item.NodeKey == "" || item.NodeIdentityKey == "" || item.ConfigRevisionKey == "" {
			return fmt.Errorf("latency batch item %d has incomplete stable identity", i)
		}
		if item.RequestedAt.IsZero() {
			item.RequestedAt = batch.RequestedAt
		}
		if item.ExecutionState == "" {
			item.ExecutionState = "queued"
		}
		if item.PersistenceState == "" {
			item.PersistenceState = "pending"
		}
		if _, err := stmt.ExecContext(ctx, item.ItemID, batch.BatchID, item.Ordinal, item.ProfileID, item.NodeKey, item.NodeIdentityKey, item.ConfigRevisionKey, item.DisplayName, item.NodeType, item.ExecutionState, item.PersistenceState, nullableText(item.AttemptID), item.RequestedAt.UTC(), item.ErrorMessage, item.PersistenceError, ""); err != nil {
			return fmt.Errorf("insert latency batch item %s: %w", item.ItemID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit latency batch: %w", err)
	}
	return nil
}

func (d *DB) UpdateLatencyBatchItem(ctx context.Context, item *LatencyBatchItem) error {
	if item == nil || item.ItemID == "" || item.BatchID == "" {
		return fmt.Errorf("latency batch item identity is incomplete")
	}
	resultJSON := ""
	if item.ResultStaged && item.Result != nil {
		encoded, err := json.Marshal(item.Result)
		if err != nil {
			return fmt.Errorf("encode latency batch result: %w", err)
		}
		resultJSON = string(encoded)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	result, err := d.db.ExecContext(ctx, `UPDATE workbench_latency_batch_items SET execution_state=?,persistence_state=?,attempt_id=?,started_at=?,finished_at=?,error_message=?,persistence_error=?,result_json=? WHERE batch_id=? AND item_id=?`, item.ExecutionState, item.PersistenceState, nullableText(item.AttemptID), nullableTime(item.StartedAt), nullableTime(item.FinishedAt), item.ErrorMessage, item.PersistenceError, resultJSON, item.BatchID, item.ItemID)
	if err != nil {
		return fmt.Errorf("update latency batch item %s: %w", item.ItemID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("latency batch item %s not found", item.ItemID)
	}
	return nil
}

func (d *DB) UpdateLatencyBatchState(ctx context.Context, batchID, state string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	result, err := d.db.ExecContext(ctx, `UPDATE workbench_latency_batches SET state=? WHERE batch_id=?`, state, batchID)
	if err != nil {
		return fmt.Errorf("update latency batch %s: %w", batchID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("latency batch %s not found", batchID)
	}
	return nil
}

func (d *DB) GetLatencyBatch(ctx context.Context, batchID string) (*LatencyBatch, error) {
	batch := &LatencyBatch{}
	err := d.db.QueryRowContext(ctx, `SELECT batch_id,request_id,test_project,timeout_seconds,requested_at,state,(SELECT COUNT(*) FROM workbench_latency_batch_items i WHERE i.batch_id=b.batch_id) FROM workbench_latency_batches b WHERE batch_id=?`, batchID).Scan(&batch.BatchID, &batch.RequestID, &batch.TestProject, &batch.TimeoutSeconds, &batch.RequestedAt, &batch.State, &batch.ItemCount)
	if err != nil {
		return nil, fmt.Errorf("get latency batch %s: %w", batchID, err)
	}
	batch.RequestedAt = batch.RequestedAt.UTC()
	rows, err := d.db.QueryContext(ctx, `SELECT item_id,batch_id,ordinal,profile_id,node_key,node_identity_key,config_revision_key,display_name,node_type,execution_state,persistence_state,attempt_id,requested_at,started_at,finished_at,error_message,persistence_error,result_json FROM workbench_latency_batch_items WHERE batch_id=? ORDER BY ordinal`, batchID)
	if err != nil {
		return nil, fmt.Errorf("query latency batch items: %w", err)
	}
	for rows.Next() {
		item, err := scanLatencyBatchItem(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		batch.Items = append(batch.Items, *item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range batch.Items {
		item := &batch.Items[i]
		if item.AttemptID != "" {
			if test, err := d.GetLatencyTest(ctx, item.AttemptID); err == nil {
				item.Result = test
				item.ResultStaged = false
				item.PersistenceState = "saved"
				item.PersistenceError = ""
			} else if !errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("load attempt %s for batch item %s: %w", item.AttemptID, item.ItemID, err)
			}
		}
	}
	return batch, nil
}

func scanLatencyBatchItem(scanner interface{ Scan(...any) error }) (*LatencyBatchItem, error) {
	item := &LatencyBatchItem{}
	var attempt, resultJSON sql.NullString
	var requested time.Time
	var started, finished sql.NullTime
	if err := scanner.Scan(&item.ItemID, &item.BatchID, &item.Ordinal, &item.ProfileID, &item.NodeKey, &item.NodeIdentityKey, &item.ConfigRevisionKey, &item.DisplayName, &item.NodeType, &item.ExecutionState, &item.PersistenceState, &attempt, &requested, &started, &finished, &item.ErrorMessage, &item.PersistenceError, &resultJSON); err != nil {
		return nil, err
	}
	item.AttemptID = attempt.String
	item.RequestedAt = requested.UTC()
	if started.Valid {
		item.StartedAt = started.Time.UTC()
	}
	if finished.Valid {
		item.FinishedAt = finished.Time.UTC()
	}
	if resultJSON.Valid && resultJSON.String != "" {
		var test LatencyTest
		if err := json.Unmarshal([]byte(resultJSON.String), &test); err != nil {
			return nil, fmt.Errorf("decode staged latency result %s: %w", item.ItemID, err)
		}
		item.Result = &test
		item.ResultStaged = true
	}
	return item, nil
}

func (d *DB) FindLatencyBatchByRequestID(ctx context.Context, requestID string) (*LatencyBatch, error) {
	var batchID string
	if err := d.db.QueryRowContext(ctx, `SELECT batch_id FROM workbench_latency_batches WHERE request_id=?`, requestID).Scan(&batchID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return d.GetLatencyBatch(ctx, batchID)
}

func (d *DB) ListLatencyBatches(ctx context.Context, limit int) ([]LatencyBatch, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := d.db.QueryContext(ctx, `SELECT batch_id,request_id,test_project,timeout_seconds,requested_at,state,(SELECT COUNT(*) FROM workbench_latency_batch_items i WHERE i.batch_id=b.batch_id) FROM workbench_latency_batches b ORDER BY requested_at DESC,batch_id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var batches []LatencyBatch
	for rows.Next() {
		var b LatencyBatch
		if err := rows.Scan(&b.BatchID, &b.RequestID, &b.TestProject, &b.TimeoutSeconds, &b.RequestedAt, &b.State, &b.ItemCount); err != nil {
			return nil, err
		}
		b.RequestedAt = b.RequestedAt.UTC()
		batches = append(batches, b)
	}
	return batches, rows.Err()
}

// ListLatencyBatchesNeedingRecovery includes stale active parent states and
// terminal parents whose child item still has unresolved execution or save
// state. A committed attempt is checked separately when each batch is loaded.
func (d *DB) ListLatencyBatchesNeedingRecovery(ctx context.Context) ([]LatencyBatch, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT b.batch_id,b.request_id,b.test_project,b.timeout_seconds,b.requested_at,b.state,(SELECT COUNT(*) FROM workbench_latency_batch_items i WHERE i.batch_id=b.batch_id) FROM workbench_latency_batches b WHERE b.state IN ('queued','running','cancelling','saving') OR EXISTS (SELECT 1 FROM workbench_latency_batch_items i WHERE i.batch_id=b.batch_id AND (i.execution_state IN ('queued','running') OR i.persistence_state IN ('pending','saving'))) ORDER BY b.requested_at DESC,b.batch_id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var batches []LatencyBatch
	for rows.Next() {
		var batch LatencyBatch
		if err := rows.Scan(&batch.BatchID, &batch.RequestID, &batch.TestProject, &batch.TimeoutSeconds, &batch.RequestedAt, &batch.State, &batch.ItemCount); err != nil {
			return nil, err
		}
		batch.RequestedAt = batch.RequestedAt.UTC()
		batches = append(batches, batch)
	}
	return batches, rows.Err()
}

func nullableText(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}
