package profiles

import (
	"math/big"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

const maxUsageInteger int64 = 9007199254740991

// SubscriptionUsage is a provider-reported snapshot, not local traffic accounting.
// Nil counters are unknown; zero is a known value. Older numeric JSON remains readable.
type SubscriptionUsage struct {
	Upload    *int64    `json:"upload"`
	Download  *int64    `json:"download"`
	Total     *int64    `json:"total,omitempty"`
	Expire    *int64    `json:"expire,omitempty"`
	RawHeader string    `json:"raw_header,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UsedBytes requires both counters and keeps the sum exact for JavaScript consumers.
func (u *SubscriptionUsage) UsedBytes() (int64, bool) {
	if u == nil || u.Upload == nil || u.Download == nil || *u.Upload < 0 || *u.Download < 0 || *u.Upload > maxUsageInteger || *u.Download > maxUsageInteger-*u.Upload {
		return 0, false
	}
	return *u.Upload + *u.Download, true
}

// Bound the input and exponent before exact decimal parsing. Fractional bytes
// (and expiry seconds) are truncated; no float rounding at the JS integer limit.
var usageNumber = regexp.MustCompile(`^\+?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]{1,3})?$`)

func parseUsageNumber(value string) *int64 {
	if len(value) > 128 || !usageNumber.MatchString(value) {
		return nil
	}
	n, ok := new(big.Rat).SetString(value)
	if !ok || n.Sign() < 0 || n.Cmp(new(big.Rat).SetInt64(maxUsageInteger)) > 0 {
		return nil
	}
	result := new(big.Int).Quo(n.Num(), n.Denom()).Int64()
	return &result
}

func parseSubscriptionUsage(header string) *SubscriptionUsage {
	if strings.TrimSpace(header) == "" {
		return nil
	}
	result := &SubscriptionUsage{RawHeader: header, UpdatedAt: time.Now().UTC()}
	values := map[string]*int64{}
	for _, part := range strings.Split(header, ";") {
		pair := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(pair) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(pair[0]))
		if key != "upload" && key != "download" && key != "total" && key != "expire" {
			continue
		}
		n := parseUsageNumber(strings.TrimSpace(pair[1]))
		if n == nil {
			continue
		} // A bad field never discards another valid one.
		if (key == "total" && *n == 0) || (key == "expire" && (*n == 0 || *n > 253402300799)) {
			continue
		}
		values[key] = n // Last valid duplicate wins; invalid repeats do not erase it.
	}
	result.Upload, result.Download, result.Total, result.Expire = values["upload"], values["download"], values["total"], values["expire"]
	return result
}

func usageHeaderScore(u *SubscriptionUsage) int {
	if u == nil {
		return -1
	}
	score := 0
	if u.Upload != nil {
		score += 4
	}
	if u.Download != nil {
		score += 4
	}
	if u.Total != nil {
		score++
	}
	if u.Expire != nil {
		score++
	}
	return score
}

func parseSubscriptionUsageHeaders(headers http.Header) *SubscriptionUsage {
	standard, metadata := []string{}, []string{}
	for name := range headers {
		lower := strings.ToLower(strings.TrimSpace(name))
		if lower == "subscription-userinfo" {
			standard = append(standard, name)
		} else if strings.HasSuffix(lower, "-subscription-userinfo") {
			metadata = append(metadata, name)
		}
	}
	sort.Strings(standard)
	sort.Strings(metadata)
	var best *SubscriptionUsage
	for _, name := range append(standard, metadata...) {
		values := headers[name]
		usage := parseSubscriptionUsage(strings.Join(values, ";"))
		if usage != nil {
			usage.RawHeader = strings.Join(values, "\n")
		}
		if usageHeaderScore(usage) > usageHeaderScore(best) {
			best = usage
		}
	}
	return best
}
