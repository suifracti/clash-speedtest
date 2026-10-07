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

// Round plans contain only stable references and request IDs. The original
// measurement tables remain the sole source of execution and save results.
type MeasurementRound struct {
	RoundID     string                 `json:"round_id"`
	TriggerType string                 `json:"trigger_type"`
	StartedAt   time.Time              `json:"started_at"`
	State       string                 `json:"state"`
	Items       []MeasurementRoundItem `json:"items"`
}
type MeasurementRoundItem struct {
	RequestID         string                    `json:"request_id"`
	Project           string                    `json:"project"`
	ServiceID         string                    `json:"service_id,omitempty"`
	ProfileID         string                    `json:"profile_id"`
	NodeKey           string                    `json:"node_key"`
	NodeIdentityKey   string                    `json:"node_identity_key"`
	ConfigRevisionKey string                    `json:"config_revision_key"`
	DisplayName       string                    `json:"display_name,omitempty"`
	ExecutionState    string                    `json:"execution_state"`
	PersistenceState  string                    `json:"persistence_state"`
	NotExecutedReason string                    `json:"not_executed_reason,omitempty"`
	Download          *WorkbenchDownloadAttempt `json:"download,omitempty"`
	Service           *PublicServiceAttempt     `json:"service,omitempty"`
	Latency           *LatencyTest              `json:"latency,omitempty"`
}

const measurementRoundSchema = `CREATE TABLE IF NOT EXISTS measurement_rounds (
 round_id TEXT PRIMARY KEY,trigger_type TEXT NOT NULL,started_at TIMESTAMP NOT NULL,
 state TEXT NOT NULL DEFAULT 'running',plan_json TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS measurement_round_items (
 round_id TEXT NOT NULL REFERENCES measurement_rounds(round_id),request_id TEXT NOT NULL,
 project TEXT NOT NULL,service_id TEXT NOT NULL,profile_id TEXT NOT NULL,node_key TEXT NOT NULL,
 node_identity_key TEXT NOT NULL,config_revision_key TEXT NOT NULL,display_name TEXT NOT NULL,
 not_executed_reason TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(round_id,request_id,profile_id,node_identity_key,config_revision_key,project,service_id));
 CREATE UNIQUE INDEX IF NOT EXISTS measurement_request_scope ON measurement_round_items(request_id,profile_id,node_identity_key,config_revision_key,project,service_id);
 CREATE INDEX IF NOT EXISTS measurement_node_round ON measurement_round_items(profile_id,node_key,node_identity_key,config_revision_key,round_id);`

func (d *DB) CreateMeasurementRound(ctx context.Context, r MeasurementRound) error {
	if strings.TrimSpace(r.RoundID) == "" || len(r.RoundID) > 96 || r.StartedAt.IsZero() || len(r.Items) == 0 || len(r.Items) > 25000 {
		return fmt.Errorf("invalid round identity or bounded plan")
	}
	if r.TriggerType != "manual" && r.TriggerType != "scheduled" && r.TriggerType != "diagnostic" {
		return fmt.Errorf("invalid round trigger")
	}
	for _, i := range r.Items {
		if i.RequestID == "" || len(i.RequestID) > 128 || i.ProfileID == "" || i.NodeKey == "" || i.NodeIdentityKey == "" || i.ConfigRevisionKey == "" || (i.Project != "service" && i.Project != "download" && i.Project != "latency") || (i.Project == "service" && i.ServiceID == "") {
			return fmt.Errorf("incomplete frozen round item")
		}
	}
	plan, err := json.Marshal(r.Items)
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var old, trigger string
	err = tx.QueryRowContext(ctx, "SELECT plan_json,trigger_type FROM measurement_rounds WHERE round_id=?", r.RoundID).Scan(&old, &trigger)
	if err == nil {
		if old != string(plan) || trigger != r.TriggerType {
			return fmt.Errorf("round identity already has a different frozen plan")
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO measurement_rounds(round_id,trigger_type,started_at,plan_json) VALUES(?,?,?,?)", r.RoundID, r.TriggerType, r.StartedAt.UTC(), string(plan))
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO measurement_round_items(round_id,request_id,project,service_id,profile_id,node_key,node_identity_key,config_revision_key,display_name) VALUES(?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, i := range r.Items {
		if _, err = stmt.ExecContext(ctx, r.RoundID, i.RequestID, i.Project, i.ServiceID, i.ProfileID, i.NodeKey, i.NodeIdentityKey, i.ConfigRevisionKey, i.DisplayName); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (d *DB) FinishMeasurementRound(ctx context.Context, id, state string) error {
	if state != "finished" && state != "interrupted" {
		return fmt.Errorf("invalid finished round state")
	}
	_, err := d.db.ExecContext(ctx, "UPDATE measurement_rounds SET state=? WHERE round_id=? AND state='running'", state, id)
	return err
}
func (d *DB) ReconcileMeasurementRounds(ctx context.Context) error {
	_, err := d.db.ExecContext(ctx, "UPDATE measurement_rounds SET state='interrupted' WHERE state='running'")
	return err
}
func (d *DB) MeasurementRequestTrigger(ctx context.Context, requestID, profile, nid, rev, project, service string) (string, error) {
	var trigger string
	err := d.db.QueryRowContext(ctx, `SELECT r.trigger_type FROM measurement_rounds r JOIN measurement_round_items i ON i.round_id=r.round_id WHERE i.request_id=? AND i.profile_id=? AND i.node_identity_key=? AND i.config_revision_key=? AND i.project=? AND i.service_id=?`, requestID, profile, nid, rev, project, service).Scan(&trigger)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return trigger, err
}
func (d *DB) MarkMeasurementNotExecuted(ctx context.Context, requestID, reason string) error {
	_, err := d.db.ExecContext(ctx, "UPDATE measurement_round_items SET not_executed_reason=? WHERE request_id=?", reason, requestID)
	return err
}
func (d *DB) QueryMeasurementRoundNodes(ctx context.Context, profile, node, nid, rev string, limit int) ([]MeasurementRound, error) {
	if profile == "" || node == "" || nid == "" || rev == "" {
		return nil, fmt.Errorf("round query requires exact node scope")
	}
	if limit < 1 || limit > 80 {
		limit = 16
	}
	rows, err := d.db.QueryContext(ctx, `SELECT round_id,trigger_type,started_at,state FROM measurement_rounds r WHERE EXISTS(SELECT 1 FROM measurement_round_items i WHERE i.round_id=r.round_id AND i.profile_id=? AND i.node_key=? AND i.node_identity_key=? AND i.config_revision_key=?) ORDER BY started_at DESC,round_id DESC LIMIT ?`, profile, node, nid, rev, limit)
	if err != nil {
		return nil, err
	}
	out := []MeasurementRound{}
	for rows.Next() {
		var r MeasurementRound
		if err = rows.Scan(&r.RoundID, &r.TriggerType, &r.StartedAt, &r.State); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for k := range out {
		r := &out[k]
		rows, err = d.db.QueryContext(ctx, `SELECT request_id,project,service_id,display_name,not_executed_reason FROM measurement_round_items WHERE round_id=? AND profile_id=? AND node_key=? AND node_identity_key=? AND config_revision_key=? ORDER BY project,service_id,request_id`, r.RoundID, profile, node, nid, rev)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			i := MeasurementRoundItem{ProfileID: profile, NodeKey: node, NodeIdentityKey: nid, ConfigRevisionKey: rev, ExecutionState: "queued", PersistenceState: "not_started"}
			if err = rows.Scan(&i.RequestID, &i.Project, &i.ServiceID, &i.DisplayName, &i.NotExecutedReason); err != nil {
				rows.Close()
				return nil, err
			}
			r.Items = append(r.Items, i)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		batches := map[string]*LatencyBatch{}
		for j := range r.Items {
			i := &r.Items[j]
			switch i.Project {
			case "service":
				a, e := d.GetPublicServiceAttemptByRequestID(ctx, i.RequestID)
				if e == nil {
					if a.ProfileID != profile || a.NodeIdentityKey != nid || a.ConfigRevisionKey != rev || a.NodeKey != node || a.ServiceID != i.ServiceID {
						return nil, fmt.Errorf("round child scope differs")
					}
					i.Service = a
					i.ExecutionState = a.ExecutionState
					i.PersistenceState = a.PersistenceState
				} else if !errors.Is(e, ErrPublicServiceAttemptNotFound) {
					return nil, e
				}
			case "download":
				a, e := d.GetWorkbenchDownloadAttemptByRequestID(ctx, i.RequestID)
				if e == nil {
					if a.ProfileID != profile || a.NodeIdentityKey != nid || a.ConfigRevisionKey != rev || a.NodeKey != node {
						return nil, fmt.Errorf("round child scope differs")
					}
					i.Download = a
					i.ExecutionState = a.ExecutionState
					i.PersistenceState = a.PersistenceState
				} else if !errors.Is(e, ErrWorkbenchDownloadAttemptNotFound) {
					return nil, e
				}
			case "latency":
				b, ok := batches[i.RequestID]
				if !ok {
					b, err = d.FindLatencyBatchByRequestID(ctx, i.RequestID)
					if err != nil {
						return nil, err
					}
					batches[i.RequestID] = b
				}
				if b != nil {
					for _, child := range b.Items {
						if child.ProfileID == profile && child.NodeKey == node && child.NodeIdentityKey == nid && child.ConfigRevisionKey == rev {
							i.Latency = child.Result
							i.ExecutionState = child.ExecutionState
							i.PersistenceState = child.PersistenceState
							i.NotExecutedReason = child.ErrorMessage
						}
					}
				}
			}
			if i.ExecutionState == "queued" && (i.NotExecutedReason != "" || r.State != "running") {
				i.ExecutionState = "not_executed"
				if i.NotExecutedReason == "" {
					i.NotExecutedReason = "轮次结束或中断前未启动该项"
				}
			}
		}
	}
	return out, nil
}

func (d *DB) FinishManualMeasurementRound(ctx context.Context, id, state string) error {
	var trigger string
	if err := d.db.QueryRowContext(ctx, "SELECT trigger_type FROM measurement_rounds WHERE round_id=?", id).Scan(&trigger); err != nil {
		return err
	}
	if trigger != "manual" {
		return fmt.Errorf("only manual round lifecycle may be updated by this action")
	}
	return d.FinishMeasurementRound(ctx, id, state)
}

// Single compatibility requests have no external plan runner to close their
// parent. Never close a multi-item/user-created plan from one child callback.
func (d *DB) FinishAutomaticMeasurementRound(ctx context.Context, requestID string) error {
	_, err := d.db.ExecContext(ctx, `UPDATE measurement_rounds SET state='finished' WHERE state='running' AND round_id LIKE 'manual-%' AND round_id IN(SELECT round_id FROM measurement_round_items WHERE request_id=?) AND (SELECT COUNT(*) FROM measurement_round_items i WHERE i.round_id=measurement_rounds.round_id)=1`, requestID)
	return err
}
