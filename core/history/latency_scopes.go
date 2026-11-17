package history

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// LatencyHistoryScope identifies saved evidence, not a current proxy config.
// Display-name continuity is confined to one subscription profile.
type LatencyHistoryScope struct {
	ProfileID         string `json:"profile_id"`
	NodeKey           string `json:"node_key"`
	NodeIdentityKey   string `json:"node_identity_key"`
	ConfigRevisionKey string `json:"config_revision_key"`
}

func (d *DB) ListLatencyHistoryScopes(ctx context.Context, profileID, displayName string, since, until time.Time) ([]LatencyHistoryScope, error) {
	if d == nil || d.db == nil {
		return nil, fmt.Errorf("history database is closed")
	}
	if strings.TrimSpace(profileID) == "" || strings.TrimSpace(displayName) == "" || since.IsZero() || until.IsZero() || !since.Before(until) {
		return nil, fmt.Errorf("latency history scopes require a profile, exact display name and valid time window")
	}
	// Match the raw-history readers' half-open window. A round that straddles
	// until still belongs when it has samples inside; preflight skips have none.
	rows, err := d.db.QueryContext(ctx, `
		SELECT node_key, node_identity_key, config_revision_key FROM (
			SELECT node_key, node_identity_key, config_revision_key
			FROM workbench_latency_tests AS t
			WHERE profile_id = ? AND display_name = ? AND (
				EXISTS (SELECT 1 FROM workbench_latency_samples AS s
					WHERE s.attempt_id = t.attempt_id AND s.timestamp >= ? AND s.timestamp < ?)
				OR (status = 'not_executed' AND finished_at >= ? AND finished_at < ?)
			)
			UNION
			SELECT node_key, node_identity_key, config_revision_key
			FROM monitor_samples
			WHERE profile_id = ? AND display_name_snapshot = ? AND probe_type = 'rtt'
				AND timestamp >= ? AND timestamp < ?
		)
		WHERE node_key <> '' AND node_identity_key <> '' AND config_revision_key <> ''
		ORDER BY node_identity_key, config_revision_key, node_key
	`, profileID, displayName, since.UTC(), until.UTC(), since.UTC(), until.UTC(), profileID, displayName, since.UTC(), until.UTC())
	if err != nil {
		return nil, fmt.Errorf("list latency history scopes: %w", err)
	}
	defer rows.Close()
	scopes := make([]LatencyHistoryScope, 0)
	for rows.Next() {
		item := LatencyHistoryScope{ProfileID: profileID}
		if err := rows.Scan(&item.NodeKey, &item.NodeIdentityKey, &item.ConfigRevisionKey); err != nil {
			return nil, fmt.Errorf("scan latency history scope: %w", err)
		}
		scopes = append(scopes, item)
	}
	return scopes, rows.Err()
}
