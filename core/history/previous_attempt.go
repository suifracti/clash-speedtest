package history

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

const previousAttemptOrder = ` ORDER BY COALESCE(a.finished_at,a.started_at,a.requested_at) DESC,a.attempt_id DESC LIMIT 1`

// Compare frozen rule fields rather than names or other presentation text.
func previousAttemptWhere(profile, node, identity, revision, source, id string, at time.Time, rule any, fields []string) (string, []any, error) {
	data, err := json.Marshal(rule)
	if err != nil {
		return "", nil, err
	}
	where := `a.profile_id=? AND a.node_key=? AND a.node_identity_key=? AND a.config_revision_key=? AND a.source=? AND r.attempt_id IS NOT NULL AND (COALESCE(a.finished_at,a.started_at,a.requested_at)<? OR (COALESCE(a.finished_at,a.started_at,a.requested_at)=? AND a.attempt_id<?))`
	args := []any{profile, node, identity, revision, source, at, at, id}
	for _, field := range fields {
		where += ` AND json_extract(a.rule_snapshot_json,'$.` + field + `') IS json_extract(?,'$.` + field + `')`
		args = append(args, string(data))
	}
	return where + previousAttemptOrder, args, nil
}

func (d *DB) PreviousWorkbenchDownloadAttempt(ctx context.Context, current *WorkbenchDownloadAttempt) (*WorkbenchDownloadAttempt, error) {
	at := current.RequestedAt
	if current.FinishedAt != nil {
		at = *current.FinishedAt
	}
	target := current.Rule.TargetURL
	if current.Result != nil && current.Result.TargetURL != "" {
		target = current.Result.TargetURL
	}
	where, args, err := previousAttemptWhere(current.ProfileID, current.NodeKey, current.NodeIdentityKey, current.ConfigRevisionKey, current.Source, current.AttemptID, at, current.Rule, strings.Fields("rule_version method maximum_bytes maximum_duration_ns sample_every_bytes sample_every_ns network_path_method physical_interface dns_mode"))
	if err != nil {
		return nil, err
	}
	where = strings.TrimSuffix(where, previousAttemptOrder)
	where += ` AND COALESCE(NULLIF(json_extract(COALESCE(r.result_json,a.staged_result_json),'$.target_url'),''),json_extract(a.rule_snapshot_json,'$.target_url')) IS ?` + previousAttemptOrder
	args = append(args, target)
	previous, err := d.getWorkbenchDownloadAttempt(ctx, where, args...)
	if err == ErrWorkbenchDownloadAttemptNotFound {
		return nil, nil
	}
	return previous, err
}

func (d *DB) PreviousPublicServiceAttempt(ctx context.Context, current *PublicServiceAttempt) (*PublicServiceAttempt, error) {
	at := current.RequestedAt
	if current.FinishedAt != nil {
		at = *current.FinishedAt
	}
	where, args, err := previousAttemptWhere(current.ProfileID, current.NodeKey, current.NodeIdentityKey, current.ConfigRevisionKey, current.Source, current.AttemptID, at, current.Rule, strings.Fields("service_id rule_version target_url method success_criterion redirect_policy timeout_seconds drift_samples maximum_body_bytes accept api_version_header result_kind region"))
	if err != nil {
		return nil, err
	}
	previous, err := d.getPublicServiceAttempt(ctx, where, args...)
	if err == ErrPublicServiceAttemptNotFound {
		return nil, nil
	}
	return previous, err
}
