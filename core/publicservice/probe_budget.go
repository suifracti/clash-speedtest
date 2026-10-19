package publicservice

// Reserve physical node sockets for the frozen observation/redirect plan, not
// just four sockets for every rule. This permits existing composites on TCP
// proxies without adding HTTP retries, changing byte limits, or extending time.
func serviceSocketBudget(rule Rule, observations int) int {
	samples := max(1, min(observations, 10))
	perSample := 1
	switch rule.ServiceID {
	case "netflix_unlock", netflixIPv4Service:
		perSample = 10 // two titles, each initial request plus at most four redirects
	case "fod_unlock", "youtube_premium", "prime_video", "hulu_jp_unlock", "google_search":
		perSample = 5 // readPage's initial request plus at most four redirects
	case "ippure_ip_quality":
		if rule.RuleVersion >= 3 {
			perSample = 4
		} // IPv4, basic, one nonce replay, risk
		if rule.RuleVersion >= 6 {
			perSample = 9
		} // plus CF summary/gateway and ASN traffic, each API allows one nonce replay
	case "antigravity":
		perSample = 4 // existing account/catalog/model requests plus independent trace
	case "ipok_ipv4", "ipok_ipv6":
		samples, perSample = 1, 2 // one exit observation and at most one budgeted lookup
	default:
		if isReachabilityRule(rule) && rule.RedirectPolicy == "follow_same_service" {
			perSample = 4
		}
	}
	budget := samples * perSample
	if selectedNodeIPv4Host(rule) != "" {
		budget++
	} // one DoH lookup for the whole attempt
	// Allow the existing first-node-family fallback once. Preparation still has
	// its own bounded DNS budget and the same total attempt deadline.
	return max(4, budget+1)
}
