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
