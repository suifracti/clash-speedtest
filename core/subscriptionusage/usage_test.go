package subscriptionusage

import (
	"testing"
	"time"
)

func fixture(account, at string, up, down int64) Snapshot {
	when, _ := time.Parse(time.RFC3339, at)
	return Snapshot{AccountKey: account, AirportName: "机场", SubscriptionName: "订阅", CapturedAt: when, Status: "ok", Upload: &up, Download: &down}
}

func TestDailyUsageKeepsResetsMissingAndGapsUnknown(t *testing.T) {
	s := []Snapshot{
		fixture("a", "2026-10-01T00:05:00+08:00", 10, 100),
		fixture("a", "2026-10-01T00:06:00+08:00", 10, 100), // same account imported twice
		fixture("a", "2026-10-01T14:00:00+08:00", 12, 120),
		fixture("a", "2026-10-02T00:06:00+08:00", 15, 150),
		{AccountKey: "a", CapturedAt: time.Date(2026, 10, 2, 4, 0, 0, 0, Location), Status: "missing"},
		fixture("a", "2026-10-04T00:06:00+08:00", 20, 200),
		fixture("a", "2026-10-04T12:00:00+08:00", 1, 2), // provider reset
		fixture("a", "2026-10-04T13:00:00+08:00", 3, 5),
	}
	a, b, _ := ParseRange("2026-10-01", "2026-10-04", "day")
	r := Build(s, a, b, "day")
	if r.ObservedBytes == nil || *r.ObservedBytes != 60 || r.UnallocatedBytes == nil || *r.UnallocatedBytes != 55 {
		t.Fatalf("observed/gap totals: %+v", r)
	}
	if len(r.Accounts) != 1 {
		t.Fatalf("duplicate imports were counted as separate accounts: %+v", r.Accounts)
	}
	for _, bucket := range r.Buckets {
		switch bucket.Date {
		case "2026-10-01":
			if bucket.Bytes == nil || *bucket.Bytes != 55 || bucket.EstimatedIntervals != 1 {
				t.Fatalf("closing observation: %+v", bucket)
			}
		case "2026-10-02", "2026-10-03":
			if bucket.Bytes != nil {
				t.Fatalf("gap fabricated daily usage: %+v", bucket)
			}
		case "2026-10-04":
			if bucket.Bytes == nil || *bucket.Bytes != 5 {
				t.Fatalf("post-reset delta: %+v", bucket)
			}
		}
	}
	for _, i := range r.Intervals {
		if (i.Status == "reset" || i.Status == "missing" || i.Status == "baseline") && i.Bytes != nil {
			t.Fatalf("unknown interval became zero usage: %+v", i)
		}
	}
}

func TestNoticeRemainingUsageUsesItsOwnBaselineAndReset(t *testing.T) {
	remaining := func(n int64) *int64 { return &n }
	notice := func(at string, n int64) Snapshot {
		when, _ := time.Parse(time.RFC3339, at)
		return Snapshot{AccountKey: "a", AirportName: "机场", SubscriptionName: "订阅", CapturedAt: when, Status: "ok", Source: "subscription_notice", CounterKind: "notice_remaining", Remaining: remaining(n)}
	}
	header := func(at string, up, down int64) Snapshot {
		s := fixture("a", at, up, down)
		s.CounterKind = "subscription_header"
		return s
	}
	s := []Snapshot{
		notice("2026-10-20T00:05:00+08:00", 1024),   // baseline
		notice("2026-10-20T00:15:00+08:00", 800),    // inferred decrease
		notice("2026-10-20T00:25:00+08:00", 900),    // reset/top-up
		notice("2026-10-20T00:35:00+08:00", 850),    // inferred after reset
		header("2026-10-20T00:45:00+08:00", 10, 20), // new counter source is a baseline
		header("2026-10-20T00:55:00+08:00", 15, 45),
	}
	a, b, _ := ParseRange("2026-10-20", "2026-10-20", "day")
	r := Build(s, a, b, "day")
	if r.ObservedBytes == nil || *r.ObservedBytes != 304 {
		t.Fatalf("inferred and provider deltas were not kept: %+v", r)
	}
	statuses := map[string]Interval{}
	for _, interval := range r.Intervals {
		statuses[interval.Until.Format(time.RFC3339)] = interval
	}
	inferred := statuses["2026-10-20T00:15:00+08:00"]
	if inferred.Status != "observed" || inferred.Bytes == nil || *inferred.Bytes != 224 || !inferred.Estimated || inferred.Upload != nil || inferred.Download != nil {
		t.Fatalf("notice decrease should be clearly inferred without upload/download split: %+v", inferred)
	}
	reset := statuses["2026-10-20T00:25:00+08:00"]
	if reset.Status != "reset" || reset.Bytes != nil {
		t.Fatalf("remaining increase should be unknown reset/top-up: %+v", reset)
	}
	if got := statuses["2026-10-20T00:45:00+08:00"]; got.Status != "baseline" || got.Bytes != nil {
		t.Fatalf("switching back to provider counters must start a new baseline: %+v", got)
	}
}

func TestWeeklyMonthlyUsageDoesNotSplitBoundaryIntervals(t *testing.T) {
	s := []Snapshot{fixture("a", "2026-09-30T00:05:00+08:00", 0, 100), fixture("a", "2026-10-01T00:05:00+08:00", 0, 160), fixture("a", "2026-10-03T00:05:00+08:00", 0, 200)}
	a, b, _ := ParseRange("2026-09-28", "2026-10-04", "week")
	week := Build(s, a, b, "week")
	if len(week.Buckets) != 1 || week.Buckets[0].Date != "2026-09-28" || week.ObservedBytes == nil || *week.ObservedBytes != 100 {
		t.Fatalf("Monday-based week: %+v", week)
	}
	a, b, _ = ParseRange("2026-09-01", "2026-10-31", "month")
	month := Build(s, a, b, "month")
	if month.ObservedBytes == nil || *month.ObservedBytes != 100 || month.UnallocatedBytes != nil || *month.Buckets[1].Bytes != 60 || month.Buckets[1].EstimatedIntervals != 1 {
		t.Fatalf("month-end daily close was lost: %+v", month)
	}
	gap := Build([]Snapshot{fixture("a", "2026-09-29T00:05:00+08:00", 0, 100), fixture("a", "2026-10-01T00:05:00+08:00", 0, 160)}, a, b, "month")
	if gap.ObservedBytes != nil || gap.UnallocatedBytes == nil || *gap.UnallocatedBytes != 60 {
		t.Fatalf("multi-day month boundary was extrapolated: %+v", gap)
	}
	a, b, _ = ParseRange("2026-09-28", "2026-10-04", "week")
	close := Build([]Snapshot{fixture("a", "2026-10-04T00:05:00+08:00", 0, 100), fixture("a", "2026-10-05T00:05:00+08:00", 0, 125)}, a, b, "week")
	if close.ObservedBytes == nil || *close.ObservedBytes != 25 || close.Buckets[0].EstimatedIntervals != 1 {
		t.Fatalf("Sunday closing interval was lost: %+v", close)
	}
	a, b, _ = ParseRange("2026-10-02", "2026-10-04", "week")
	partial := Build(s, a, b, "week")
	if partial.ObservedBytes != nil {
		t.Fatalf("partial date range included out-of-range bytes: %+v", partial)
	}
}

func TestDailyUpdateUsesShanghaiBoundary(t *testing.T) {
	for _, tc := range []struct{ now, want string }{{"2026-10-02T16:04:00Z", "2026-10-02T16:05:00Z"}, {"2026-10-02T16:05:00Z", "2026-10-03T16:05:00Z"}, {"2026-12-31T16:06:00Z", "2027-01-01T16:05:00Z"}} {
		now, _ := time.Parse(time.RFC3339, tc.now)
		if got := NextDaily(now).Format(time.RFC3339); got != tc.want {
			t.Fatalf("next %s = %s, want %s", tc.now, got, tc.want)
		}
	}
}

func TestAddNilPointerSafety(t *testing.T) {
	// Should safely return without panicking when value is nil
	add(nil, 42)

	var p *int64
	add(&p, 100)
	if p == nil || *p != 100 {
		t.Fatalf("expected p to point to 100, got %v", p)
	}

	add(&p, 50)
	if *p != 150 {
		t.Fatalf("expected *p to be 150, got %d", *p)
	}
}

func TestTodayUsageUsesYesterdayCloseAndLowerBoundFallback(t *testing.T) {
	now := time.Date(2026, 10, 24, 16, 0, 0, 0, Location)
	remaining := func(n int64) *int64 { return &n }
	notice := func(account, at string, n int64) Snapshot {
		when, _ := time.Parse(time.RFC3339, at)
		return Snapshot{AccountKey: account, CapturedAt: when, Status: "ok", CounterKind: "notice_remaining", Remaining: remaining(n)}
	}
	snapshots := []Snapshot{
		// Yesterday's close is usable: the delta spans the day boundary.
		fixture("a", "2026-10-23T23:00:00+08:00", 100, 900),
		fixture("a", "2026-10-24T08:00:00+08:00", 110, 990),
		fixture("a", "2026-10-24T12:00:00+08:00", 130, 1170),
		// No yesterday snapshot: today's own window is a lower bound.
		fixture("b", "2026-10-24T08:00:00+08:00", 0, 500),
		fixture("b", "2026-10-24T12:00:00+08:00", 0, 560),
		// A single observation cannot measure anything yet.
		fixture("c", "2026-10-24T12:00:00+08:00", 0, 700),
		// A counter reset falls back to today's first observation.
		fixture("d", "2026-10-23T23:00:00+08:00", 0, 1000),
		fixture("d", "2026-10-24T09:00:00+08:00", 0, 40),
		fixture("d", "2026-10-24T11:00:00+08:00", 0, 60),
		// Notice balances use the remaining amount, not upload/download.
		notice("e", "2026-10-23T23:00:00+08:00", 100),
		notice("e", "2026-10-24T12:00:00+08:00", 90),
	}
	got := map[string]TodayTotal{}
	for _, today := range TodayUsage(snapshots, now) {
		got[today.AccountKey] = today
	}
	a := got["a"]
	if a.Bytes == nil || *a.Bytes != 300 || a.LowerBound || a.Estimated {
		t.Fatalf("yesterday-close delta: %+v", a)
	}
	b := got["b"]
	if b.Bytes == nil || *b.Bytes != 60 || !b.LowerBound || b.Since == nil || !b.Since.Equal(mustTime("2026-10-24T08:00:00+08:00")) {
		t.Fatalf("lower-bound delta: %+v", b)
	}
	if _, exists := got["c"]; exists {
		t.Fatalf("single observation became a daily total: %+v", got["c"])
	}
	d := got["d"]
	if d.Bytes == nil || *d.Bytes != 20 || !d.LowerBound {
		t.Fatalf("reset should use today's own window: %+v", d)
	}
	e := got["e"]
	if e.Bytes == nil || *e.Bytes != 10 || !e.Estimated || e.LowerBound {
		t.Fatalf("notice balance delta: %+v", e)
	}
}

func mustTime(value string) time.Time {
	at, _ := time.Parse(time.RFC3339, value)
	return at
}
