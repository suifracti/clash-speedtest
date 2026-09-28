package profiles

import (
	"strconv"
	"strings"
	"time"
)

// SubscriptionUsage is a provider-reported snapshot, not local traffic accounting.
type SubscriptionUsage struct {
	Upload    int64     `json:"upload"`
	Download  int64     `json:"download"`
	Total     *int64    `json:"total,omitempty"`
	Expire    *int64    `json:"expire,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

func parseSubscriptionUsage(header string) *SubscriptionUsage {
	values := map[string]int64{}
	for _, part := range strings.Split(header, ";") {
		pair := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(pair) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(pair[0]))
		if key != "upload" && key != "download" && key != "total" && key != "expire" {
			continue
		}
		value := strings.TrimSpace(pair[1])
		if value == "" && (key == "total" || key == "expire") {
			continue
		}
		n, err := strconv.ParseInt(value, 10, 64)
		// Keep values exactly representable in the JavaScript DTO consumer.
		if err != nil || n < 0 || n > 9007199254740991 {
			return nil
		}
		if _, duplicate := values[key]; duplicate {
			return nil
		}
		values[key] = n
	}
	up, upOK := values["upload"]
	down, downOK := values["download"]
	if !upOK || !downOK || up > 9007199254740991-down {
		return nil
	}
	result := &SubscriptionUsage{Upload: up, Download: down, UpdatedAt: time.Now().UTC()}
	if total, ok := values["total"]; ok && total > 0 {
		result.Total = &total
	}
	if expire, ok := values["expire"]; ok && expire > 0 && expire <= 253402300799 {
		result.Expire = &expire
	}
	return result
}
