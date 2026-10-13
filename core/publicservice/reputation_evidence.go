package publicservice

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// Evidence never copies request headers, cookies, URLs, or arbitrary JSON fields.
// The hash covers the original bounded prefix; the saved body is a projection.
var ipokSensitiveText = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://|bearer\s|authorization|password|passwd|secret|token|rawconfig|subscription|private[-_ ]?key|uuid|set-cookie)`)

func ipokSafeText(s string) string {
	if len(s) > 256 || ipokSensitiveText.MatchString(s) || strings.ContainsAny(s, "\r\n\x00") {
		return "[redacted]"
	}
	return s
}

func ipokSafeJSON(body []byte) string {
	if len(body) > MaximumResponseBytes || !json.Valid(body) {
		return `{"evidence":"withheld_non_json_or_incomplete"}`
	}
	var value any
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.UseNumber()
	if dec.Decode(&value) != nil {
		return `{"evidence":"withheld_non_json_or_incomplete"}`
	}
	// Unknown fields are omitted even inside geo/sources/quota: no debug echoes.
	allowed := map[string]bool{}
	for _, k := range []string{"apiVersion", "geo", "ip", "asn", "asName", "isp", "country", "countryCode", "risk", "ipType", "signals", "sources", "source", "name", "observedAt", "status", "error", "code", "message", "quota", "arbitraryLookup", "remaining", "resetsInSeconds", "limit", "used", "resetAt", "hosting", "proxy", "vpn", "tor", "abuse"} {
		allowed[k] = true
	}
	var project func(any, int) any
	project = func(v any, depth int) any {
		if depth > 16 {
			return "[redacted-depth]"
		}
		switch x := v.(type) {
		case map[string]any:
			out := map[string]any{}
			for k, child := range x {
				if allowed[k] {
					out[k] = project(child, depth+1)
				}
			}
			return out
		case []any:
			out := make([]any, len(x))
			for i, child := range x {
				out[i] = project(child, depth+1)
			}
			return out
		case string:
			return ipokSafeText(x)
		default:
			return v
		}
	}
	data, _ := json.Marshal(project(value, 0))
	if len(data) > MaximumResponseBytes {
		return `{"evidence":"withheld_projection_limit"}`
	}
	return string(data)
}

func ipokResponseEvidence(response *http.Response, body []byte, readComplete bool) map[string]string {
	truncated := len(body) > MaximumResponseBytes
	if truncated {
		body = body[:MaximumResponseBytes]
	}
	headers := map[string]string{}
	for _, k := range []string{"Content-Type", "Date", "Retry-After", "X-IPOK-Upstream"} {
		if v := response.Header.Get(k); v != "" {
			headers[k] = ipokSafeText(v)
		}
	}
	headerJSON, _ := json.Marshal(headers)
	safeBody := ipokSafeJSON(body)
	if truncated || !readComplete {
		safeBody = `{"evidence":"withheld_non_json_or_incomplete"}`
	}
	upstream := "unknown"
	if v := headers["X-IPOK-Upstream"]; v != "" {
		upstream = v
	}
	return map[string]string{
		"provider_response_headers":              string(headerJSON),
		"provider_response_body":                 safeBody,
		"provider_response_body_sha256":          fmt.Sprintf("%x", sha256.Sum256(body)),
		"provider_response_body_redacted_sha256": fmt.Sprintf("%x", sha256.Sum256([]byte(safeBody))),
		"provider_response_body_bytes":           strconv.Itoa(len(body)),
		"provider_response_body_truncated":       strconv.FormatBool(truncated),
		"provider_response_complete":             strconv.FormatBool(readComplete && !truncated),
		"provider_response_body_policy":          "ipok_json_allowlist_v1; sha256_of_original_bounded_prefix",
		"provider_response_http_status":          strconv.Itoa(response.StatusCode),
		"provider_upstream":                      upstream,
	}
}
