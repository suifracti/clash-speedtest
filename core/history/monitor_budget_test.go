package history

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
)

func TestMonitorBudgetLedgerMigratesAndNeverRefundsOnReopenOrClockRollback(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMonitorSamples(ctx, []*monitor.MonitorSample{{SampleID: "old-raw", RunID: "old-run", NodeKey: "node", Timestamp: time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC), Success: true}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	// This is a known version-1 database: all prior tables stay intact, while
	// the version-2-only ledger is absent.
	raw, err := sql.Open("sqlite", filepath.Join(dir, "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec("DROP TABLE monitor_budget_usage; UPDATE schema_meta SET schema_version = 1 WHERE singleton = 1"); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	day1, day2 := "2026-09-23", "2026-09-24"
	for i := 0; i < 2; i++ {
		if _, err := store.ReserveMonitorRequest(ctx, day1, 2); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.ReserveMonitorRequest(ctx, day1, 2); err == nil {
		t.Fatal("third request crossed daily limit")
	}
	readDay, granted, err := store.ReserveMonitorBytes(ctx, day1, 5, 7)
	if err != nil || readDay != day1 || granted != 5 {
		t.Fatalf("first body reservation = %s/%d, %v", readDay, granted, err)
	}
	_, granted, err = store.ReserveMonitorBytes(ctx, day1, 5, 7)
	if err != nil || granted != 2 {
		t.Fatalf("concurrent-safe partial reservation = %d, %v", granted, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	usage, err := store.MonitorBudgetUsage(ctx, day1)
	if err != nil || usage.RequestsUsed != 2 || usage.BytesUsed != 7 {
		t.Fatalf("reopen refunded today's quota: %+v %v", usage, err)
	}
	usage, err = store.MonitorBudgetUsage(ctx, day2)
	if err != nil || usage.UTCDay != day2 || usage.RequestsUsed != 0 || usage.BytesUsed != 0 {
		t.Fatalf("UTC new day failed to reset: %+v %v", usage, err)
	}
	if _, err := store.ReserveMonitorRequest(ctx, day2, 2); err != nil {
		t.Fatal(err)
	}
	usage, err = store.MonitorBudgetUsage(ctx, day1)
	if err != nil || usage.UTCDay != day2 || usage.RequestsUsed != 1 {
		t.Fatalf("clock rollback granted a second day: %+v %v", usage, err)
	}
	if samples, err := store.QueryMonitorSamples(ctx, monitor.SampleFilter{Limit: 10}); err != nil || len(samples) != 1 || samples[0].SampleID != "old-raw" {
		t.Fatalf("v1 raw history changed: %+v %v", samples, err)
	}
	var version int
	if err := store.db.db.QueryRow("SELECT schema_version FROM schema_meta WHERE singleton = 1").Scan(&version); err != nil || version != CurrentSchemaVersion {
		t.Fatalf("schema upgrade version=%d, %v", version, err)
	}
	if _, err := store.ReserveMonitorRequest(ctx, day1, 1); err == nil {
		t.Fatal("rollback should not reset used request")
	} else {
		var block *monitor.BudgetBlockError
		if !errors.As(err, &block) || block.Code != "requests_exhausted" {
			t.Fatalf("wrong exhaustion error: %v", err)
		}
	}
}
