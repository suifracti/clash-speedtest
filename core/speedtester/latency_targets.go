package speedtester

import (
	"fmt"
	"strings"
)

// LatencyTargetURL is an allowlist, not an arbitrary outbound URL supplied by a client.
func LatencyTargetURL(id string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "all":
		return "multi://latency-four-v1", nil
	case "", "cloudflare":
		return "https://speed.cloudflare.com/__down?bytes=1", nil
	case "google":
		return "https://www.gstatic.com/generate_204", nil
	case "github":
		return "https://github.com/favicon.ico", nil
	case "apple":
		return "https://captive.apple.com/hotspot-detect.html", nil
	case "microsoft":
		return "http://www.msftconnecttest.com/connecttest.txt", nil
	case "firefox":
		return "https://detectportal.firefox.com/success.txt", nil
	default:
		return "", fmt.Errorf("未知延迟检测目标: %s", id)
	}
}

// LatencyTargetMatchURLs returns the current target URL together with any
// historical URLs that belong to the same site identity for history queries.
func LatencyTargetMatchURLs(id string) ([]string, error) {
	url, err := LatencyTargetURL(id)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "", "cloudflare":
		return []string{url, "https://cp.cloudflare.com/generate_204"}, nil
	case "github":
		return []string{url, "https://api.github.com/zen"}, nil
	default:
		return []string{url}, nil
	}
}

// LatencyProbeURLs expands the versioned suite while preserving single-target callers.
func LatencyProbeURLs(target string) []string {
	if target == "multi://latency-four-v1" {
		return []string{"https://speed.cloudflare.com/__down?bytes=1", "https://www.gstatic.com/generate_204", "https://github.com/favicon.ico", "https://captive.apple.com/hotspot-detect.html"}
	}
	if target != "multi://latency-v1" {
		return []string{target}
	}
	urls := make([]string, 0, 6)
	for _, id := range []string{"cloudflare", "google", "github", "apple", "microsoft", "firefox"} {
		url, _ := LatencyTargetURL(id)
		urls = append(urls, url)
	}
	return urls
}
