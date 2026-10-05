package history

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/faceair/clash-speedtest/core/subscriptionusage"
)

const subscriptionUsageDDL = `
CREATE TABLE IF NOT EXISTS subscription_usage_snapshots (
 account_key TEXT NOT NULL,
 captured_at INTEGER NOT NULL,
 payload_json TEXT NOT NULL,
 PRIMARY KEY(account_key, captured_at)
);
CREATE INDEX IF NOT EXISTS idx_subscription_usage_time ON subscription_usage_snapshots(captured_at);
CREATE TABLE IF NOT EXISTS subscription_usage_refresh (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),
 last_attempt INTEGER NOT NULL DEFAULT 0,
 next_refresh INTEGER NOT NULL DEFAULT 0,
 state TEXT NOT NULL DEFAULT '',
 message TEXT NOT NULL DEFAULT ''
);
INSERT OR IGNORE INTO subscription_usage_refresh(singleton) VALUES(1);
`

func (s *Store) SaveSubscriptionUsage(ctx context.Context, snapshot subscriptionusage.Snapshot) error {
	if snapshot.AccountKey == "" || snapshot.CapturedAt.IsZero() {
		return fmt.Errorf("incomplete subscription usage snapshot")
	}
	if snapshot.Status == "ok" && (snapshot.Upload == nil || snapshot.Download == nil || *snapshot.Upload < 0 || *snapshot.Download < 0) {
		return fmt.Errorf("invalid subscription counters")
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return fmt.Errorf("history store is closed")
	}
	s.db.mu.Lock()
	defer s.db.mu.Unlock()
	if s.db.db == nil {
		return fmt.Errorf("history database is closed")
	}
	_, err = s.db.db.ExecContext(ctx, `INSERT INTO subscription_usage_snapshots(account_key,captured_at,payload_json) VALUES(?,?,?) ON CONFLICT(account_key,captured_at) DO NOTHING`, snapshot.AccountKey, snapshot.CapturedAt.UnixNano(), string(raw))
	return err
}

func (s *Store) HasSubscriptionUsage(ctx context.Context, accountKey string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return false, fmt.Errorf("history store is closed")
	}
	s.db.mu.Lock()
	defer s.db.mu.Unlock()
	if s.db.db == nil {
		return false, fmt.Errorf("history database is closed")
	}
	var n int
	err := s.db.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM subscription_usage_snapshots WHERE account_key=?)`, accountKey).Scan(&n)
	return n == 1, err
}

func (s *Store) QuerySubscriptionUsage(ctx context.Context, from, until time.Time) ([]subscriptionusage.Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil, fmt.Errorf("history store is closed")
	}
	s.db.mu.Lock()
	defer s.db.mu.Unlock()
	if s.db.db == nil {
		return nil, fmt.Errorf("history database is closed")
	}
	rows, err := s.db.db.QueryContext(ctx, `SELECT payload_json FROM subscription_usage_snapshots WHERE captured_at<? AND (captured_at>=? OR (account_key,captured_at) IN (SELECT account_key,MAX(captured_at) FROM subscription_usage_snapshots WHERE captured_at<? AND json_extract(payload_json,'$.status')='ok' GROUP BY account_key)) ORDER BY captured_at,account_key`, until.UnixNano(), from.UnixNano(), from.UnixNano())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []subscriptionusage.Snapshot{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var snap subscriptionusage.Snapshot
		if err := json.Unmarshal([]byte(raw), &snap); err != nil {
			return nil, err
		}
		out = append(out, snap)
	}
	return out, rows.Err()
}

func (s *Store) ClaimSubscriptionRefresh(ctx context.Context, now, next time.Time, force bool) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return false, fmt.Errorf("history store is closed")
	}
	s.db.mu.Lock()
	defer s.db.mu.Unlock()
	if s.db.db == nil {
		return false, fmt.Errorf("history database is closed")
	}
	result, err := s.db.db.ExecContext(ctx, `UPDATE subscription_usage_refresh SET last_attempt=?,next_refresh=?,state='running',message='更新中；若服务中断请手动重试' WHERE singleton=1 AND (? OR next_refresh<=?)`, now.UnixNano(), next.UnixNano(), force, now.UnixNano())
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func (s *Store) FinishSubscriptionRefresh(ctx context.Context, state, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return fmt.Errorf("history store is closed")
	}
	s.db.mu.Lock()
	defer s.db.mu.Unlock()
	if s.db.db == nil {
		return fmt.Errorf("history database is closed")
	}
	_, err := s.db.db.ExecContext(ctx, `UPDATE subscription_usage_refresh SET state=?,message=? WHERE singleton=1`, state, message)
	return err
}

func (s *Store) SubscriptionRefreshState(ctx context.Context) (*subscriptionusage.RefreshState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil, fmt.Errorf("history store is closed")
	}
	s.db.mu.Lock()
	defer s.db.mu.Unlock()
	if s.db.db == nil {
		return nil, fmt.Errorf("history database is closed")
	}
	var last, next int64
	state := &subscriptionusage.RefreshState{}
	err := s.db.db.QueryRowContext(ctx, `SELECT last_attempt,next_refresh,state,message FROM subscription_usage_refresh WHERE singleton=1`).Scan(&last, &next, &state.State, &state.Message)
	if last != 0 {
		state.LastAttempt = time.Unix(0, last).UTC()
	}
	if next != 0 {
		state.NextRefresh = time.Unix(0, next).UTC()
	}
	return state, err
}
