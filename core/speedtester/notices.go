package speedtester

import (
	"sort"
)

// SubscriptionNoticeNames returns announcement-like proxy names embedded in
// the subscription itself. It parses the body only and does not load remote
// proxy providers or open node connections.
func SubscriptionNoticeNames(body []byte) ([]string, error) {
	config, err := parseConfigBody(body)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0)
	for _, proxy := range config.Proxies {
		name, ok := proxy["name"].(string)
		if ok && isSubscriptionNoticeName(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}
