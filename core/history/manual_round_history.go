package history

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type ManualRoundSummary struct {
	RoundID       string    `json:"round_id"`
	StartedAt     time.Time `json:"started_at"`
	State         string    `json:"state"`
	NodeCount     int       `json:"node_count"`
	LatencyCount  int       `json:"latency_count"`
	DownloadCount int       `json:"download_count"`
	ServiceCount  int       `json:"service_count"`
	ServiceIDs    []string  `json:"service_ids,omitempty"`
}

type ManualRoundPage struct {
	Rounds     []ManualRoundSummary `json:"rounds"`
	NextBefore string               `json:"next_before,omitempty"`
}

type ManualRoundDetail struct {
	MeasurementRound
	NextOffset int `json:"next_offset,omitempty"`
}

// ListManualRounds reads plan counts only. Latency-only plans already have a
// batch directory, and scheduled or recommendation runs have their own views.
func (d *DB) ListManualRounds(ctx context.Context, before string) (ManualRoundPage, error) {
	const pageSize = 30
	page := ManualRoundPage{Rounds: []ManualRoundSummary{}}
	if d == nil || d.db == nil {
		return page, fmt.Errorf("history database is closed")
	}
	rows, err := d.db.QueryContext(ctx, `WITH recent AS (
		SELECT r.round_id,r.started_at,r.state FROM measurement_rounds r WHERE r.trigger_type='manual'
		AND EXISTS (SELECT 1 FROM measurement_round_items i WHERE i.round_id=r.round_id AND i.project IN ('download','service'))
		AND (?='' OR (r.started_at,r.round_id)<(SELECT started_at,round_id FROM measurement_rounds WHERE round_id=?))
		ORDER BY r.started_at DESC,r.round_id DESC LIMIT ?
	) SELECT r.round_id,r.started_at,r.state,
		COUNT(DISTINCT json_array(i.profile_id,i.node_key,i.node_identity_key,i.config_revision_key)),
		SUM(i.project='latency'),SUM(i.project='download'),SUM(i.project='service'),
		GROUP_CONCAT(DISTINCT CASE WHEN i.project='service' THEN i.service_id END)
	FROM recent r JOIN measurement_round_items i ON i.round_id=r.round_id
	GROUP BY r.round_id,r.started_at,r.state ORDER BY r.started_at DESC,r.round_id DESC`, before, before, pageSize+1)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var round ManualRoundSummary
		var ids sql.NullString
		if err := rows.Scan(&round.RoundID, &round.StartedAt, &round.State, &round.NodeCount, &round.LatencyCount, &round.DownloadCount, &round.ServiceCount, &ids); err != nil {
			return page, err
		}
		if ids.Valid && ids.String != "" {
			round.ServiceIDs = strings.Split(ids.String, ",")
		}
		page.Rounds = append(page.Rounds, round)
	}
	if len(page.Rounds) > pageSize {
		page.Rounds = page.Rounds[:pageSize]
		page.NextBefore = page.Rounds[pageSize-1].RoundID
	}
	return page, rows.Err()
}

func (d *DB) ReadManualRound(ctx context.Context, id string, offset int) (ManualRoundDetail, error) {
	const pageSize = 50
	var detail ManualRoundDetail
	if d == nil || d.db == nil {
		return detail, fmt.Errorf("history database is closed")
	}
	if id == "" || offset < 0 {
		return detail, fmt.Errorf("invalid round or item offset")
	}
	err := d.db.QueryRowContext(ctx, `SELECT round_id,trigger_type,started_at,state FROM measurement_rounds WHERE round_id=? AND trigger_type='manual'`, id).
		Scan(&detail.RoundID, &detail.TriggerType, &detail.StartedAt, &detail.State)
	if err != nil {
		return detail, err
	}
	detail.Items, err = d.readMeasurementItems(ctx, `WHERE round_id=?
		ORDER BY profile_id,node_key,node_identity_key,config_revision_key,
		CASE project WHEN 'latency' THEN 0 WHEN 'download' THEN 1 ELSE 2 END, service_id,request_id LIMIT ? OFFSET ?`, id, pageSize+1, offset)
	if err != nil {
		return detail, err
	}
	if len(detail.Items) > pageSize {
		detail.Items = detail.Items[:pageSize]
		detail.NextOffset = offset + pageSize
	}
	err = d.readMeasurementResults(ctx, &detail.MeasurementRound)
	return detail, err
}
