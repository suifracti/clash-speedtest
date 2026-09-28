package speedtester

import "fmt"

// LatencyTargetURL is an allowlist, not an arbitrary outbound URL supplied by a client.
func LatencyTargetURL(id string) (string, error) {
	switch id {
	case "all":
		return "multi://latency-v1", nil
	case "", "cloudflare":
		return "https://speed.cloudflare.com/__down?bytes=1", nil
	case "google":
		return "https://www.gstatic.com/generate_204", nil
	case "github":
		return "https://api.github.com/zen", nil
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

// LatencyProbeURLs expands the versioned suite while preserving single-target callers.
func LatencyProbeURLs(target string) []string {
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
