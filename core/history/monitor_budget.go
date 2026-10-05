package history

import (
	"context"
	"fmt"

	"github.com/faceair/clash-speedtest/core/monitor"
)

// withMonitorBudget serializes one ledger decision with all other Monitor
// reservations. Unknown/closed stores fail before any request is permitted.
func (d *DB) withMonitorBudget(ctx context.Context, utcDay string, decide func(*monitor.BudgetUsage) (bool, error)) (monitor.BudgetUsage, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.db == nil {
		return monitor.BudgetUsage{}, fmt.Errorf("Monitor budget database is closed")
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return monitor.BudgetUsage{}, fmt.Errorf("begin Monitor budget transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var usage monitor.BudgetUsage
	if err := tx.QueryRowContext(ctx, "SELECT utc_day, requests_used, bytes_used FROM monitor_budget_usage WHERE singleton = 1").Scan(&usage.UTCDay, &usage.RequestsUsed, &usage.BytesUsed); err != nil {
		return monitor.BudgetUsage{}, fmt.Errorf("read Monitor budget usage: %w", err)
	}
	changed := false
	if utcDay > usage.UTCDay {
		usage = monitor.BudgetUsage{UTCDay: utcDay}
		changed = true
	}
	mutated, err := decide(&usage)
	if err != nil {
		return monitor.BudgetUsage{}, err
	}
	changed = changed || mutated
	if changed {
		if _, err := tx.ExecContext(ctx, "UPDATE monitor_budget_usage SET utc_day = ?, requests_used = ?, bytes_used = ? WHERE singleton = 1", usage.UTCDay, usage.RequestsUsed, usage.BytesUsed); err != nil {
			return monitor.BudgetUsage{}, fmt.Errorf("write Monitor budget usage: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return monitor.BudgetUsage{}, fmt.Errorf("commit Monitor budget usage: %w", err)
	}
	return usage, nil
}

func (d *DB) MonitorBudgetUsage(ctx context.Context, utcDay string) (monitor.BudgetUsage, error) {
	return d.withMonitorBudget(ctx, utcDay, func(*monitor.BudgetUsage) (bool, error) { return false, nil })
}

func (d *DB) ReserveMonitorRequest(ctx context.Context, utcDay string) (monitor.BudgetUsage, error) {
	return d.withMonitorBudget(ctx, utcDay, func(usage *monitor.BudgetUsage) (bool, error) {
		usage.RequestsUsed++
		return true, nil
	})
}

func (d *DB) RefundMonitorRequest(ctx context.Context, utcDay string) error {
	_, err := d.withMonitorBudget(ctx, "", func(usage *monitor.BudgetUsage) (bool, error) {
		if usage.UTCDay != utcDay || usage.RequestsUsed == 0 {
			return false, nil
		}
		usage.RequestsUsed--
		return true, nil
	})
	return err
}

func (d *DB) ReserveMonitorBytes(ctx context.Context, utcDay string, want int64) (string, int64, error) {
	if want < 0 {
		return "", 0, fmt.Errorf("Monitor byte reservation cannot be negative")
	}
	var granted int64
	usage, err := d.withMonitorBudget(ctx, utcDay, func(usage *monitor.BudgetUsage) (bool, error) {
		granted = want
		usage.BytesUsed += granted
		return true, nil
	})
	if err != nil {
		return "", 0, err
	}
	return usage.UTCDay, granted, nil
}

func (d *DB) RefundMonitorBytes(ctx context.Context, utcDay string, unused int64) error {
	if unused <= 0 {
		return nil
	}
	_, err := d.withMonitorBudget(ctx, "", func(usage *monitor.BudgetUsage) (bool, error) {
		if usage.UTCDay != utcDay {
			return false, nil
		} // A new UTC day already began; overcount conservatively.
		if unused > usage.BytesUsed {
			unused = usage.BytesUsed
		}
		usage.BytesUsed -= unused
		return true, nil
	})
	return err
}

func (s *Store) MonitorBudgetUsage(ctx context.Context, utcDay string) (monitor.BudgetUsage, error) {
	if s == nil || s.db == nil {
		return monitor.BudgetUsage{}, fmt.Errorf("history store is not initialized")
	}
	return s.db.MonitorBudgetUsage(ctx, utcDay)
}
func (s *Store) ReserveMonitorRequest(ctx context.Context, utcDay string) (monitor.BudgetUsage, error) {
	if s == nil || s.db == nil {
		return monitor.BudgetUsage{}, fmt.Errorf("history store is not initialized")
	}
	return s.db.ReserveMonitorRequest(ctx, utcDay)
}
func (s *Store) RefundMonitorRequest(ctx context.Context, utcDay string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("history store is not initialized")
	}
	return s.db.RefundMonitorRequest(ctx, utcDay)
}
func (s *Store) ReserveMonitorBytes(ctx context.Context, utcDay string, want int64) (string, int64, error) {
	if s == nil || s.db == nil {
		return "", 0, fmt.Errorf("history store is not initialized")
	}
	return s.db.ReserveMonitorBytes(ctx, utcDay, want)
}
func (s *Store) RefundMonitorBytes(ctx context.Context, utcDay string, unused int64) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("history store is not initialized")
	}
	return s.db.RefundMonitorBytes(ctx, utcDay, unused)
}

var _ monitor.BudgetLedger = (*Store)(nil)
