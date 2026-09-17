package profiles

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/metacubex/mihomo/constant"
)

func newAirportID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func IsHTTPURL(raw string) bool {
	lower := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func DefaultUserAgent() string {
	return constant.MihomoName + "/" + constant.Version
}

func FetchSubscription(rawURL, userAgent string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, strings.TrimSpace(rawURL), nil)
	if err != nil {
		return nil, err
	}
	if userAgent == "" {
		userAgent = DefaultUserAgent()
	}
	req.Header.Set("User-Agent", userAgent)
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("empty subscription body")
	}
	return body, nil
}

func RedactURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" {
		return compactDisplay(raw, 72)
	}
	query := parsed.Query()
	changed := false
	for key := range query {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "key") || strings.Contains(lower, "password") || lower == "auth" {
			query.Set(key, "REDACTED")
			changed = true
		}
	}
	if changed {
		parsed.RawQuery = query.Encode()
	}
	return compactDisplay(parsed.String(), 72)
}

func compactDisplay(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func ExpandLocalPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || IsHTTPURL(raw) {
		return raw
	}
	if strings.HasPrefix(raw, "~"+string(os.PathSeparator)) || raw == "~" || strings.HasPrefix(raw, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			raw = home + raw[1:]
		}
	}
	return raw
}
