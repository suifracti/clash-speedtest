package history

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const ipReputationDDL = `CREATE TABLE IF NOT EXISTS ip_reputation_cache (cache_key TEXT PRIMARY KEY, result_json TEXT NOT NULL, queried_at TEXT NOT NULL, valid_until TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS ip_reputation_budget (singleton INTEGER PRIMARY KEY CHECK(singleton=1), utc_day TEXT NOT NULL, used INTEGER NOT NULL, next_allowed TEXT NOT NULL);`

func (d *DB) LoadIPReputation(ctx context.Context, key string, now time.Time) (string, error) {
	if d == nil || d.db == nil {
		return "", fmt.Errorf("IPOK budget database is closed")
	}
	var data, validUntil string
	err := d.db.QueryRowContext(ctx, "SELECT result_json,valid_until FROM ip_reputation_cache WHERE cache_key=?", key).Scan(&data, &validUntil)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	expires, err := time.Parse(time.RFC3339Nano, validUntil)
	if err != nil {
		return "", fmt.Errorf("invalid persisted IP reputation cache expiry")
	}
	if !now.Before(expires) {
		return "", nil
	}
	return data, nil
}
func (d *DB) SaveIPReputation(ctx context.Context, key, data string, at time.Time) error {
	if d == nil {
		return fmt.Errorf("IPOK budget database is closed")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.db == nil {
		return fmt.Errorf("IPOK budget database is closed")
	}
	_, err := d.db.ExecContext(ctx, "INSERT INTO ip_reputation_cache(cache_key,result_json,queried_at,valid_until)VALUES(?,?,?,?) ON CONFLICT(cache_key) DO UPDATE SET result_json=excluded.result_json,queried_at=excluded.queried_at,valid_until=excluded.valid_until", key, data, at.UTC().Format(time.RFC3339Nano), at.Add(24*time.Hour).UTC().Format(time.RFC3339Nano))
	return err
}

// CheckIPReputationBudget only reads durable admission state. It never
// resets/consumes quota; ReserveIPReputation still guards each vendor lookup.
func (d *DB) CheckIPReputationBudget(ctx context.Context, at time.Time) (bool, error) {
	if d == nil {
		return false, fmt.Errorf("IPOK budget database is closed")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.db == nil {
		return false, fmt.Errorf("IPOK budget database is closed")
	}
	var day, next string
	var used int
	err := d.db.QueryRowContext(ctx, "SELECT utc_day,used,next_allowed FROM ip_reputation_budget WHERE singleton=1").Scan(&day, &used, &next)
	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	nextTime, err := time.Parse(time.RFC3339Nano, next)
	if err != nil || used < 0 {
		return false, fmt.Errorf("invalid persisted IPOK budget")
	}
	if _, err := time.Parse("2006-01-02", day); err != nil {
		return false, fmt.Errorf("invalid persisted IPOK budget day")
	}
	return !at.Before(nextTime) && (day != at.UTC().Format("2006-01-02") || used < 15), nil
}

func (d *DB) ReserveIPReputation(ctx context.Context, at time.Time) (bool, error) {
	if d == nil {
		return false, fmt.Errorf("IPOK budget database is closed")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.db == nil {
		return false, fmt.Errorf("IPOK budget database is closed")
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var day, next string
	var used int
	err = tx.QueryRowContext(ctx, "SELECT utc_day,used,next_allowed FROM ip_reputation_budget WHERE singleton=1").Scan(&day, &used, &next)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	nextTime, _ := time.Parse(time.RFC3339Nano, next)
	if at.Before(nextTime) {
		return false, nil
	}
	today := at.UTC().Format("2006-01-02")
	if day != today {
		used = 0
	}
	if used >= 15 {
		return false, nil
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO ip_reputation_budget(singleton,utc_day,used,next_allowed)VALUES(1,?,?,?) ON CONFLICT(singleton)DO UPDATE SET utc_day=excluded.utc_day,used=excluded.used,next_allowed=excluded.next_allowed", today, used+1, at.Add(2*time.Second).UTC().Format(time.RFC3339Nano))
	if err != nil {
		return false, err
	}
	return true, tx.Commit()
}
func (d *DB) CoolIPReputation(ctx context.Context, until time.Time) error {
	if d == nil {
		return fmt.Errorf("IPOK budget database is closed")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.db == nil {
		return fmt.Errorf("IPOK budget database is closed")
	}
	_, err := d.db.ExecContext(ctx, "UPDATE ip_reputation_budget SET next_allowed=CASE WHEN next_allowed>? THEN next_allowed ELSE ? END WHERE singleton=1", until.UTC().Format(time.RFC3339Nano), until.UTC().Format(time.RFC3339Nano))
	return err
}
