package history

import "context"

// Return the most recent actual attempt per service for this immutable node
// revision. No zero-filled successes, no current/old revision mixing.
func (d *DB) LatestPublicServiceChecks(ctx context.Context, profile, identity, revision string) ([]*PublicServiceAttempt, error) {
	rows, err := d.db.QueryContext(ctx, publicServiceAttemptSelect+` WHERE a.attempt_id IN (SELECT attempt_id FROM (SELECT b.attempt_id,ROW_NUMBER() OVER(PARTITION BY b.service_id ORDER BY b.requested_at DESC,b.attempt_id DESC) AS ordinal FROM workbench_public_service_attempts b LEFT JOIN workbench_public_service_results br ON br.attempt_id=b.attempt_id WHERE b.profile_id=? AND b.node_identity_key=? AND b.config_revision_key=? AND (b.staged_result_json!='' OR br.attempt_id IS NOT NULL)) WHERE ordinal=1) ORDER BY a.service_id`, profile, identity, revision)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []*PublicServiceAttempt{}
	for rows.Next() {
		a, err := scanPublicServiceAttempt(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}
