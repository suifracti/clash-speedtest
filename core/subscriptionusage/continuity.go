package subscriptionusage

import "sort"

// ContinueAccounts joins changed subscription links for display only.
func ContinueAccounts(snapshots []Snapshot) []Snapshot {
	type span struct {
		first, last Snapshot
		end         Snapshot
	}
	spans := map[string]*span{}
	for _, s := range snapshots {
		p := spans[s.AccountKey]
		if p == nil {
			p = &span{end: s}
			spans[s.AccountKey] = p
		}
		if s.CapturedAt.After(p.end.CapturedAt) {
			p.end = s
		}
		if s.Status != "ok" || s.Total == nil || s.Upload == nil || s.Download == nil {
			continue
		}
		if p.first.AccountKey == "" || s.CapturedAt.Before(p.first.CapturedAt) {
			p.first = s
		}
		if p.last.AccountKey == "" || s.CapturedAt.After(p.last.CapturedAt) {
			p.last = s
		}
	}
	keys := []string{}
	for k, p := range spans {
		if p.first.AccountKey != "" {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return spans[keys[i]].first.CapturedAt.Before(spans[keys[j]].first.CapturedAt) })
	next := map[string]string{}
	for _, k := range keys {
		n := spans[k].first
		best := ""
		var delta int64
		for _, old := range keys {
			p := spans[old]
			if old == k || next[old] != "" || !p.end.CapturedAt.Before(n.CapturedAt) || n.AirportID == "" || p.last.AirportID != n.AirportID || *p.last.Total != *n.Total {
				continue
			}
			d := *n.Upload + *n.Download - *p.last.Upload - *p.last.Download
			if d >= 0 && (best == "" || d < delta) {
				best, delta = old, d
			}
		}
		if best != "" {
			next[best] = k
		}
	}
	out := append([]Snapshot(nil), snapshots...)
	for i := range out {
		for next[out[i].AccountKey] != "" {
			out[i].AccountKey = next[out[i].AccountKey]
		}
	}
	return out
}
