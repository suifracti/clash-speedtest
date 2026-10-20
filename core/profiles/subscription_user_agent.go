package profiles

import "strings"

// EffectiveUserAgent resolves the account override before the app/default UA.
func (s *Subscription) EffectiveUserAgent(fallback string) string {
	if s != nil && strings.TrimSpace(s.UserAgent) != "" {
		return strings.TrimSpace(s.UserAgent)
	}
	if fallback = strings.TrimSpace(fallback); fallback != "" {
		return fallback
	}
	return DefaultUserAgent()
}
