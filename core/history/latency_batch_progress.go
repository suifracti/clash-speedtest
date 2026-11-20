package history

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// LatencyBatchProgressItem is a bounded live-feed item for an active latency batch.
type LatencyBatchProgressItem struct {
	ItemID            string     `json:"item_id"`
	AttemptID         string     `json:"attempt_id,omitempty"`
	DisplayName       string     `json:"display_name"`
	ProfileID         string     `json:"profile_id"`
	NodeKey           string     `json:"node_key,omitempty"`
	NodeIdentityKey   string     `json:"node_identity_key,omitempty"`
	ConfigRevisionKey string     `json:"config_revision_key,omitempty"`
	NodeType          string     `json:"node_type,omitempty"`
	ExecutionState    string     `json:"execution_state"`
	SuccessSamples    int        `json:"success_samples"`
	TotalSamples      int        `json:"total_samples"`
	LatencyMs         int64      `json:"latency_ms,omitempty"`
	ErrorMessage      string     `json:"error_message,omitempty"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
}

// LatencyBatchProgress reads counters and bounded live slices from the existing batch/item states.
type LatencyBatchProgress struct {
	BatchID         string                     `json:"batch_id"`
	State           string                     `json:"state"`
	RequestedAt     time.Time                  `json:"requested_at,omitempty"`
	Total           int                        `json:"total"`
	Processed       int                        `json:"processed"`
	Running         int                        `json:"running"`
	Queued          int                        `json:"queued"`
	AllSuccess      int                        `json:"all_success"`
	PartialResponse int                        `json:"partial_response"`
	NoResponse      int                        `json:"no_response"`
	SaveFailed      int                        `json:"save_failed"`
	NotExecuted     int                        `json:"not_executed"`
	RunningItems    []LatencyBatchProgressItem `json:"running_items,omitempty"`
	RecentItems     []LatencyBatchProgressItem `json:"recent_items,omitempty"`
}

func (s *Store) GetLatencyBatchProgress(ctx context.Context, batchID string) (*LatencyBatchProgress, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	p := &LatencyBatchProgress{BatchID: batchID}
	var requestedAt time.Time
	err := s.db.db.QueryRowContext(ctx, `WITH items AS (
 SELECT i.*,COALESCE(t.total_samples,json_extract(NULLIF(i.result_json,''),'$.total_samples'),0) AS samples,
 COALESCE(t.success_samples,json_extract(NULLIF(i.result_json,''),'$.success_samples'),0) AS successes
 FROM workbench_latency_batch_items i LEFT JOIN workbench_latency_tests t ON t.attempt_id=i.attempt_id WHERE i.batch_id=?
 ) SELECT b.state,b.requested_at,COUNT(i.item_id),
 COALESCE(SUM(i.execution_state NOT IN ('queued','running','cancelling')),0),
 COALESCE(SUM(i.execution_state IN ('running','cancelling')),0),
 COALESCE(SUM(i.execution_state='queued'),0),
 COALESCE(SUM(i.samples>0 AND i.successes=i.samples),0),
 COALESCE(SUM(i.successes>0 AND i.successes<i.samples),0),
 COALESCE(SUM(i.samples>0 AND i.successes=0 AND i.execution_state NOT IN ('not_executed','cancelled')),0),
 COALESCE(SUM(i.persistence_state='failed'),0),
 COALESCE(SUM(i.execution_state IN ('not_executed','cancelled')),0)
 FROM workbench_latency_batches b LEFT JOIN items i ON i.batch_id=b.batch_id WHERE b.batch_id=? GROUP BY b.batch_id`, batchID, batchID).Scan(
		&p.State, &requestedAt, &p.Total, &p.Processed, &p.Running, &p.Queued,
		&p.AllSuccess, &p.PartialResponse, &p.NoResponse, &p.SaveFailed, &p.NotExecuted,
	)
	if err != nil {
		return nil, err
	}
	p.RequestedAt = requestedAt.UTC()

	// Bounded active items (at most 16) so large batches stay O(1) payload.
	if runRows, rErr := s.db.db.QueryContext(ctx, `
 SELECT i.item_id,i.display_name,i.profile_id,COALESCE(i.node_type,''),i.execution_state,i.started_at
 FROM workbench_latency_batch_items i
 WHERE i.batch_id=? AND i.execution_state IN ('running','cancelling')
 ORDER BY i.ordinal ASC LIMIT 16`, batchID); rErr == nil {
		defer runRows.Close()
		for runRows.Next() {
			var item LatencyBatchProgressItem
			var started sql.NullTime
			if err := runRows.Scan(&item.ItemID, &item.DisplayName, &item.ProfileID, &item.NodeType, &item.ExecutionState, &started); err == nil {
				if started.Valid {
					t := started.Time.UTC()
					item.StartedAt = &t
				}
				p.RunningItems = append(p.RunningItems, item)
			}
		}
	}

	// Bounded recently completed items (latest 100) for interactive filtering & drill-down.
	if recRows, rErr := s.db.db.QueryContext(ctx, `
 SELECT i.item_id,COALESCE(i.attempt_id,''),i.display_name,i.profile_id,i.node_key,i.node_identity_key,i.config_revision_key,COALESCE(i.node_type,''),i.execution_state,
 COALESCE(t.success_samples,CAST(json_extract(NULLIF(i.result_json,''),'$.success_samples') AS INTEGER),0),
 COALESCE(t.total_samples,CAST(json_extract(NULLIF(i.result_json,''),'$.total_samples') AS INTEGER),0),
 COALESCE(t.latency_ms,CAST(json_extract(NULLIF(i.result_json,''),'$.latency_ms') AS INTEGER),0),
 COALESCE(NULLIF(i.error_message,''),NULLIF(t.error_message,''),''),
 i.finished_at
 FROM workbench_latency_batch_items i
 LEFT JOIN workbench_latency_tests t ON t.attempt_id=i.attempt_id
 WHERE i.batch_id=? AND i.execution_state NOT IN ('queued','running','cancelling')
 ORDER BY COALESCE(i.finished_at, i.started_at, i.requested_at) DESC, i.ordinal DESC LIMIT 100`, batchID); rErr == nil {
		defer recRows.Close()
		for recRows.Next() {
			var item LatencyBatchProgressItem
			var finished sql.NullTime
			if err := recRows.Scan(&item.ItemID, &item.AttemptID, &item.DisplayName, &item.ProfileID, &item.NodeKey, &item.NodeIdentityKey, &item.ConfigRevisionKey, &item.NodeType, &item.ExecutionState, &item.SuccessSamples, &item.TotalSamples, &item.LatencyMs, &item.ErrorMessage, &finished); err == nil {
				if finished.Valid {
					t := finished.Time.UTC()
					item.FinishedAt = &t
				}
				p.RecentItems = append(p.RecentItems, item)
			}
		}
	}

	return p, nil
}
