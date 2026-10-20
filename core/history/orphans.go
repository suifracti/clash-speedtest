package history

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
)

type OrphanGroup struct {
	Table  string `json:"table"`
	Parent string `json:"parent"`
	Kind   string `json:"kind"`
	Rows   int64  `json:"rows"`
}

type OrphanReport struct {
	ReadOnly        bool          `json:"read_only"`
	TotalViolations int64         `json:"total_violations"`
	Groups          []OrphanGroup `json:"groups"`
}

// InspectOrphans reads an existing database without OpenDB, migrations or
// repairs. It returns aggregate counts, never node names, URLs or row contents.
// A row violating multiple declared relationships contributes once per violation.
func InspectOrphans(ctx context.Context, path string) (*OrphanReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if path == "" {
		return nil, fmt.Errorf("history database path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	params := url.Values{"mode": {"ro"}, "_pragma": {"query_only(1)", "busy_timeout(5000)"}}
	dsn := (&url.URL{Scheme: "file", Path: filepath.ToSlash(abs), RawQuery: params.Encode()}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("open read-only history snapshot: %w", err)
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return nil, fmt.Errorf("check history foreign keys: %w", err)
	}
	counts := map[OrphanGroup]int64{}
	for rows.Next() {
		var table, parent string
		var rowID sql.NullInt64
		var foreignKeyID int
		if err := rows.Scan(&table, &rowID, &parent, &foreignKeyID); err != nil {
			rows.Close()
			return nil, err
		}
		counts[OrphanGroup{Table: table, Parent: parent, Kind: "foreign_key"}]++
	}
	readErr := rows.Err()
	closeErr := rows.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	// Legacy monitor samples predate declared foreign keys, so check this
	// relationship explicitly. Historical runs may outlive deleted job definitions.
	var hasSamples, hasRuns bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='monitor_samples'), EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='monitor_runs')`).Scan(&hasSamples, &hasRuns); err != nil {
		return nil, err
	}
	if hasSamples {
		query := `SELECT COUNT(*) FROM monitor_samples`
		if hasRuns {
			query += ` s WHERE NOT EXISTS (SELECT 1 FROM monitor_runs r WHERE r.run_id=s.run_id)`
		}
		var count int64
		if err := tx.QueryRowContext(ctx, query).Scan(&count); err != nil {
			return nil, err
		}
		if count > 0 {
			counts[OrphanGroup{Table: "monitor_samples", Parent: "monitor_runs", Kind: "monitor_run"}] = count
		}
	}
	report := &OrphanReport{ReadOnly: true, Groups: make([]OrphanGroup, 0, len(counts))}
	for group, count := range counts {
		group.Rows = count
		report.Groups = append(report.Groups, group)
		report.TotalViolations += count
	}
	sort.Slice(report.Groups, func(i, j int) bool {
		a, b := report.Groups[i], report.Groups[j]
		if a.Table != b.Table {
			return a.Table < b.Table
		}
		if a.Parent != b.Parent {
			return a.Parent < b.Parent
		}
		return a.Kind < b.Kind
	})
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return report, nil
}
