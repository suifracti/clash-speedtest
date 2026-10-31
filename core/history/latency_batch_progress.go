package history

import (
	"context"
	"fmt"
)

// LatencyBatchProgress reads counters from the existing batch/item states.
type LatencyBatchProgress struct {
	BatchID         string `json:"batch_id"`
	State           string `json:"state"`
	Total           int    `json:"total"`
	Processed       int    `json:"processed"`
	AllSuccess      int    `json:"all_success"`
	PartialResponse int    `json:"partial_response"`
	NoResponse      int    `json:"no_response"`
	SaveFailed      int    `json:"save_failed"`
	NotExecuted     int    `json:"not_executed"`
}

func (s *Store) GetLatencyBatchProgress(ctx context.Context, batchID string) (*LatencyBatchProgress, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	p := &LatencyBatchProgress{BatchID: batchID}
	err := s.db.db.QueryRowContext(ctx, `WITH items AS (
 SELECT i.*,COALESCE(t.total_samples,json_extract(NULLIF(i.result_json,''),'$.total_samples'),0) AS samples,
 COALESCE(t.success_samples,json_extract(NULLIF(i.result_json,''),'$.success_samples'),0) AS successes
 FROM workbench_latency_batch_items i LEFT JOIN workbench_latency_tests t ON t.attempt_id=i.attempt_id WHERE i.batch_id=?
 ) SELECT b.state,COUNT(i.item_id),
 COALESCE(SUM(i.execution_state NOT IN ('queued','running','cancelling')),0),
 COALESCE(SUM(i.samples>0 AND i.successes=i.samples),0),
 COALESCE(SUM(i.successes>0 AND i.successes<i.samples),0),
 COALESCE(SUM(i.samples>0 AND i.successes=0 AND i.execution_state NOT IN ('not_executed','cancelled')),0),
 COALESCE(SUM(i.persistence_state='failed'),0),
 COALESCE(SUM(i.execution_state IN ('not_executed','cancelled')),0)
 FROM workbench_latency_batches b LEFT JOIN items i ON i.batch_id=b.batch_id WHERE b.batch_id=? GROUP BY b.batch_id`, batchID, batchID).Scan(&p.State, &p.Total, &p.Processed, &p.AllSuccess, &p.PartialResponse, &p.NoResponse, &p.SaveFailed, &p.NotExecuted)
	return p, err
}
