package history

import (
	"context"
	"testing"
	"time"
)

func TestIPReputationCacheQuotaSurviveReopen(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	if err = db.SaveIPReputation(ctx, "ipok-v1|IPv4|8.8.8.8", "real-provider-json", now); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 15; i++ {
		ok, e := db.ReserveIPReputation(ctx, now.Add(time.Duration(i)*3*time.Second))
		if e != nil || !ok {
			t.Fatalf("reserve %d %v %v", i, ok, e)
		}
	}
	db.Close()
	db, err = OpenDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	data, err := db.LoadIPReputation(ctx, "ipok-v1|IPv4|8.8.8.8", now.Add(time.Hour))
	if err != nil || data != "real-provider-json" {
		t.Fatal(data, err)
	}
	if ok, err := db.ReserveIPReputation(ctx, now.Add(time.Minute)); ok || err != nil {
		t.Fatal("daily quota reset on reopen", ok, err)
	}
	data, _ = db.LoadIPReputation(ctx, "ipok-v1|IPv4|8.8.8.8", now.Add(25*time.Hour))
	if data != "" {
		t.Fatal("expired cached response reused")
	}
}

func TestIPReputationBudgetPreflightIsPersistentAndReadOnly(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name    string
		used    int
		next    string
		at      time.Time
		allowed bool
		wantErr bool
	}{
		{"available", 3, "2026-10-09T09:00:00Z", now, true, false},
		{"day_exhausted", 15, "2026-10-09T09:00:00Z", now, false, false},
		{"cooldown", 3, "2026-10-09T11:00:00Z", now, false, false},
		{"new_day", 15, "2026-10-09T09:00:00Z", now.Add(24 * time.Hour), true, false},
		{"cooldown_across_day", 3, "2026-10-10T11:00:00Z", now.Add(24 * time.Hour), false, false},
		{"corrupt_time", 3, "invalid", now, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			db, err := OpenDB(dir)
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.db.ExecContext(ctx, "INSERT INTO ip_reputation_budget VALUES(1,?,?,?)", "2026-10-09", tc.used, tc.next)
			if err != nil {
				t.Fatal(err)
			}
			db.Close()
			db, err = OpenDB(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			checker, ok := any(db).(interface {
				CheckIPReputationBudget(context.Context, time.Time) (bool, error)
			})
			if !ok {
				t.Fatal("persistent preflight method absent")
			}
			for i := 0; i < 2; i++ {
				allowed, err := checker.CheckIPReputationBudget(ctx, tc.at)
				if allowed != tc.allowed || (err != nil) != tc.wantErr {
					t.Fatalf("preflight %v %v", allowed, err)
				}
			}
			var used int
			var day, next string
			db.db.QueryRowContext(ctx, "SELECT utc_day,used,next_allowed FROM ip_reputation_budget WHERE singleton=1").Scan(&day, &used, &next)
			if day != "2026-10-09" || used != tc.used || next != tc.next {
				t.Fatal("preflight modified quota/cooldown")
			}
		})
	}
}

func TestIPReputationPreflightDoesNotInitializeOrConsumeBudget(t *testing.T) {
	db, err := OpenDB(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		ok, err := db.CheckIPReputationBudget(ctx, time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC))
		if !ok || err != nil {
			t.Fatal("unused budget blocked")
		}
	}
	var rows int
	if err = db.db.QueryRowContext(ctx, "SELECT count(*) FROM ip_reputation_budget").Scan(&rows); err != nil || rows != 0 {
		t.Fatal("preflight initialized mutable budget")
	}
	db.Close()
	if ok, err := db.CheckIPReputationBudget(ctx, time.Now()); ok || err == nil {
		t.Fatal("closed database admitted request")
	}
}

func TestIPReputationCacheTTLUsesActualInstant(t *testing.T) {
	db, err := OpenDB(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	at := time.Date(2026, 10, 13, 10, 0, 0, 500000000, time.UTC)
	if err = db.SaveIPReputation(context.Background(), "fixture", "fixture", at); err != nil {
		t.Fatal(err)
	}
	data, err := db.LoadIPReputation(context.Background(), "fixture", at.Add(24*time.Hour-500*time.Millisecond))
	if err != nil || data != "fixture" {
		t.Fatal("cache expired early across fractional timestamp", data, err)
	}
}
