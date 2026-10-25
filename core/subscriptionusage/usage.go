package subscriptionusage

import (
	"fmt"
	"sort"
	"time"
)

var Location = time.FixedZone("Asia/Shanghai", 8*60*60)

type Snapshot struct {
	AccountKey       string    `json:"account_key"`
	AirportID        string    `json:"airport_id"`
	AirportName      string    `json:"airport_name"`
	SubscriptionID   string    `json:"subscription_id"`
	SubscriptionName string    `json:"subscription_name"`
	CapturedAt       time.Time `json:"captured_at"`
	Status           string    `json:"status"`
	Source           string    `json:"source"`
	CounterKind      string    `json:"counter_kind,omitempty"`
	Upload           *int64    `json:"upload"`
	Download         *int64    `json:"download"`
	Remaining        *int64    `json:"remaining,omitempty"`
	Total            *int64    `json:"total,omitempty"`
	Message          string    `json:"message,omitempty"`
	RawError         string    `json:"raw_error,omitempty"`
}

type Interval struct {
	AccountKey  string     `json:"account_key"`
	Name        string     `json:"name"`
	From        *time.Time `json:"from,omitempty"`
	Until       time.Time  `json:"until"`
	CounterKind string     `json:"counter_kind,omitempty"`
	Upload      *int64     `json:"upload"`
	Download    *int64     `json:"download"`
	Bytes       *int64     `json:"bytes"`
	Status      string     `json:"status"`
	Bucket      string     `json:"bucket,omitempty"`
	Estimated   bool       `json:"estimated"`
	Message     string     `json:"message,omitempty"`
}

type Bucket struct {
	Date               string `json:"date"`
	Bytes              *int64 `json:"bytes"`
	Upload             *int64 `json:"upload"`
	Download           *int64 `json:"download"`
	Intervals          int    `json:"intervals"`
	EstimatedIntervals int    `json:"estimated_intervals"`
}

// TodayTotal is the account delta between the last snapshot today and the last
// snapshot yesterday. LowerBound means there was no yesterday snapshot, so the
// value only covers first-today to last-today and is not a full-day total.
type TodayTotal struct {
	AccountKey string     `json:"account_key"`
	Bytes      *int64     `json:"bytes"`
	Estimated  bool       `json:"estimated"`
	LowerBound bool       `json:"lower_bound"`
	Since      *time.Time `json:"since,omitempty"`
}

type Report struct {
	From             string        `json:"from"`
	Until            string        `json:"until"`
	Period           string        `json:"period"`
	Timezone         string        `json:"timezone"`
	Buckets          []Bucket      `json:"buckets"`
	Intervals        []Interval    `json:"intervals"`
	Accounts         []Snapshot    `json:"accounts"`
	Today            []TodayTotal  `json:"today"`
	ObservedBytes    *int64        `json:"observed_bytes"`
	UnallocatedBytes *int64        `json:"unallocated_bytes"`
	UnknownIntervals int           `json:"unknown_intervals"`
	Refresh          *RefreshState `json:"refresh,omitempty"`
	DailyEnabled     bool          `json:"daily_enabled"`
}

type RefreshState struct {
	LastAttempt time.Time `json:"last_attempt"`
	NextRefresh time.Time `json:"next_refresh"`
	State       string    `json:"state"`
	Message     string    `json:"message"`
}

func NextDaily(now time.Time) time.Time {
	local := now.In(Location)
	next := time.Date(local.Year(), local.Month(), local.Day(), 0, 5, 0, 0, Location)
	if !next.After(local) {
		next = next.AddDate(0, 0, 1)
	}
	return next.UTC()
}

// TodayUsage follows the explicit daily rule: today's last snapshot minus
// yesterday's last snapshot. Without a yesterday snapshot the delta between
// today's first and last observation is a lower bound and carries its start.
func TodayUsage(snapshots []Snapshot, now time.Time) []TodayTotal {
	local := now.In(Location)
	todayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, Location)
	tomorrow, yesterday := todayStart.AddDate(0, 0, 1), todayStart.AddDate(0, 0, -1)
	byAccount := map[string][]Snapshot{}
	order := []string{}
	for _, s := range snapshots {
		kind := snapshotCounterKind(s)
		if s.Status != "ok" || !validCounter(s, kind) {
			continue
		}
		if _, exists := byAccount[s.AccountKey]; !exists {
			order = append(order, s.AccountKey)
		}
		byAccount[s.AccountKey] = append(byAccount[s.AccountKey], s)
	}
	out := []TodayTotal{}
	for _, key := range order {
		list := byAccount[key]
		sort.SliceStable(list, func(i, j int) bool { return list[i].CapturedAt.Before(list[j].CapturedAt) })
		var firstToday, lastToday, lastYesterday *Snapshot
		for index := range list {
			s := &list[index]
			at := s.CapturedAt.In(Location)
			switch {
			case !at.Before(todayStart) && at.Before(tomorrow):
				if firstToday == nil {
					firstToday = s
				}
				lastToday = s
			case !at.Before(yesterday) && at.Before(todayStart):
				lastYesterday = s
			}
		}
		if firstToday == nil || lastToday == nil {
			continue
		}
		kind := snapshotCounterKind(*lastToday)
		if lastYesterday != nil && snapshotCounterKind(*lastYesterday) == kind {
			if current, ok := counterValue(*lastToday, kind); ok {
				if previous, ok := counterValue(*lastYesterday, kind); ok {
					delta, measured := todayDelta(kind, previous, current)
					if measured {
						out = append(out, TodayTotal{AccountKey: key, Bytes: &delta, Estimated: kind == "notice_remaining"})
						continue
					}
				}
			}
		}
		// No usable yesterday snapshot (or a counter reset): only today's own
		// observation window is known, so report its delta as a lower bound.
		if !lastToday.CapturedAt.Before(firstToday.CapturedAt) {
			current, currentOK := counterValue(*lastToday, kind)
			first, firstOK := counterValue(*firstToday, kind)
			if delta, measured := todayDelta(kind, first, current); currentOK && firstOK && measured {
				since := firstToday.CapturedAt
				out = append(out, TodayTotal{AccountKey: key, Bytes: &delta, Estimated: kind == "notice_remaining", LowerBound: true, Since: &since})
			}
		}
	}
	return out
}

// todayDelta returns upload+download growth or remaining-balance decrease.
// Counters going the other way mean a reset; that window is not measurable.
func todayDelta(kind string, earlier, later int64) (int64, bool) {
	if kind == "notice_remaining" {
		if later > earlier {
			return 0, false
		}
		return earlier - later, true
	}
	if later < earlier {
		return 0, false
	}
	return later - earlier, true
}

func counterValue(s Snapshot, kind string) (int64, bool) {
	if !validCounter(s, kind) {
		return 0, false
	}
	switch kind {
	case "subscription_header":
		if *s.Upload > (1<<62)-*s.Download {
			return 0, false
		}
		return *s.Upload + *s.Download, true
	case "notice_remaining":
		return *s.Remaining, true
	default:
		return 0, false
	}
}

func ParseRange(from, until, period string) (time.Time, time.Time, error) {
	if period != "day" && period != "week" && period != "month" {
		return time.Time{}, time.Time{}, fmt.Errorf("汇总周期须为 day、week 或 month")
	}
	a, err := time.ParseInLocation("2006-01-02", from, Location)
	if err != nil {
		return a, a, fmt.Errorf("开始日期无效")
	}
	b, err := time.ParseInLocation("2006-01-02", until, Location)
	if err != nil {
		return a, b, fmt.Errorf("结束日期无效")
	}
	if b.Before(a) || b.After(a.AddDate(5, 0, 0)) {
		return a, b, fmt.Errorf("请选择不超过五年的日期范围")
	}
	return a, b.AddDate(0, 0, 1), nil
}

func bucketKey(at time.Time, period string) string {
	t := at.In(Location)
	if period == "month" {
		return t.Format("2006-01")
	}
	if period == "week" {
		t = t.AddDate(0, 0, -(int(t.Weekday())+6)%7)
	}
	return t.Format("2006-01-02")
}

func add(value **int64, n int64) {
	if value == nil {
		return
	}
	if *value == nil {
		v := int64(0)
		*value = &v
	}
	**value += n
}

func snapshotCounterKind(s Snapshot) string {
	if s.CounterKind != "" {
		return s.CounterKind
	}
	if s.Upload != nil && s.Download != nil {
		return "subscription_header" // older snapshots used only the two provider counters
	}
	if s.Remaining != nil {
		return "notice_remaining"
	}
	return ""
}

func validCounter(s Snapshot, kind string) bool {
	switch kind {
	case "subscription_header":
		return s.Upload != nil && s.Download != nil && *s.Upload >= 0 && *s.Download >= 0
	case "notice_remaining":
		return s.Remaining != nil && *s.Remaining >= 0
	default:
		return false
	}
}

// Build uses provider counters, never local probe bytes. Gaps and resets remain
// explicit. Near-midnight daily intervals are attributed to the preceding day
// as an estimate; other cross-bucket intervals are not split or extrapolated.
func Build(snapshots []Snapshot, from, until time.Time, period string) Report {
	r := Report{From: from.Format("2006-01-02"), Until: until.AddDate(0, 0, -1).Format("2006-01-02"), Period: period, Timezone: "Asia/Shanghai", Buckets: []Bucket{}, Intervals: []Interval{}, Accounts: []Snapshot{}}
	buckets := map[string]*Bucket{}
	for day := from; day.Before(until); day = day.AddDate(0, 0, 1) {
		key := bucketKey(day, period)
		if buckets[key] == nil {
			buckets[key] = &Bucket{Date: key}
		}
	}
	ordered := append([]Snapshot(nil), snapshots...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].CapturedAt.Before(ordered[j].CapturedAt) })
	previous := map[string]Snapshot{}
	latest := map[string]Snapshot{}
	for _, s := range ordered {
		latest[s.AccountKey] = s
		i := Interval{AccountKey: s.AccountKey, Name: s.AirportName + " · " + s.SubscriptionName, Until: s.CapturedAt, Status: s.Status, Message: s.Message}
		p, exists := previous[s.AccountKey]
		kind := snapshotCounterKind(s)
		i.CounterKind = kind
		if s.Status == "ok" && validCounter(s, kind) {
			if !exists || snapshotCounterKind(p) != kind || !validCounter(p, kind) {
				i.Status = "baseline"
			} else {
				start := p.CapturedAt
				i.From = &start
				if kind == "notice_remaining" {
					if *s.Remaining > *p.Remaining {
						i.Status = "reset"
						i.Message = "公告剩余量增加，可能是重置或套餐变更；本区间用量未知"
					} else {
						delta := *p.Remaining - *s.Remaining
						i.Bytes = &delta
						i.Status = "observed"
						i.Estimated = true
						i.Message = "根据公告剩余量下降估算；无法拆分上传和下载"
					}
				} else if *s.Upload < *p.Upload || *s.Download < *p.Download {
					i.Status = "reset"
					i.Message = "机场计数下降，本区间用量未知；从新计数继续记录"
				} else {
					up, down := *s.Upload-*p.Upload, *s.Download-*p.Download
					sum := up + down
					i.Upload = &up
					i.Download = &down
					i.Bytes = &sum
					i.Status = "observed"
				}
				if i.Bytes != nil {
					startKey, endKey := bucketKey(start, period), bucketKey(s.CapturedAt, period)
					startLocal, endLocal := start.In(Location), s.CapturedAt.In(Location)
					startDay := time.Date(startLocal.Year(), startLocal.Month(), startLocal.Day(), 0, 0, 0, 0, Location)
					// A daily closing observation belongs to the preceding date in
					// every view, including Sunday/month-end closes. Longer gaps
					// may belong to one week/month but are never split into days.
					if !startDay.Before(from) && startDay.Before(until) && endLocal.Format("2006-01-02") == startDay.AddDate(0, 0, 1).Format("2006-01-02") && endLocal.Hour() == 0 && s.CapturedAt.Sub(start) <= 26*time.Hour {
						i.Bucket = startKey
						i.Estimated = true
						if i.Message == "" {
							i.Message = "每日更新区间，近午夜增量归前一日；并非精确自然日账单"
						} else {
							i.Message += "；近午夜区间归前一日"
						}
					} else if startKey == endKey && !start.Before(from) && s.CapturedAt.Before(until) {
						i.Bucket = endKey
					}
					if i.Bucket == "" {
						i.Status = "cross_period"
						if kind == "notice_remaining" {
							i.Message = "公告余额变化跨日，未按天数摊分；无法拆分上传和下载"
						} else {
							i.Message = "跨日、周或月的观测区间，未按天数摊分"
						}
					}
				}
			}
			previous[s.AccountKey] = s
		}
		// Include only in-range observations or the daily closing observation
		// that was explicitly attributed to an in-range bucket.
		_, allocated := buckets[i.Bucket]
		inRange := !s.CapturedAt.Before(from) && s.CapturedAt.Before(until)
		if !inRange && !allocated {
			continue
		}
		r.Intervals = append(r.Intervals, i)
		if i.Bytes != nil {
			if allocated {
				b := buckets[i.Bucket]
				add(&b.Bytes, *i.Bytes)
				if i.Upload != nil {
					add(&b.Upload, *i.Upload)
				}
				if i.Download != nil {
					add(&b.Download, *i.Download)
				}
				b.Intervals++
				if i.Estimated {
					b.EstimatedIntervals++
				}
				add(&r.ObservedBytes, *i.Bytes)
			} else {
				add(&r.UnallocatedBytes, *i.Bytes)
			}
		} else {
			r.UnknownIntervals++
		}
	}
	for _, b := range buckets {
		r.Buckets = append(r.Buckets, *b)
	}
	sort.Slice(r.Buckets, func(i, j int) bool { return r.Buckets[i].Date > r.Buckets[j].Date })
	for _, s := range latest {
		r.Accounts = append(r.Accounts, s)
	}
	sort.Slice(r.Accounts, func(i, j int) bool {
		a, b := r.Accounts[i], r.Accounts[j]
		return a.AirportName+a.SubscriptionName+a.AccountKey < b.AirportName+b.SubscriptionName+b.AccountKey
	})
	sort.SliceStable(r.Intervals, func(i, j int) bool { return r.Intervals[i].Until.After(r.Intervals[j].Until) })
	return r
}
