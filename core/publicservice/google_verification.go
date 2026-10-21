package publicservice

import (
	"net/http"
	"net/url"
	"strings"
)

const googleVerificationReason = "Google 对该 IP 要求验证，实际使用时也会遇到"

func googleVerificationURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return (host == "google.com" || strings.HasSuffix(host, ".google.com") || host == "youtube.com" || strings.HasSuffix(host, ".youtube.com")) && (strings.HasPrefix(u.Path, "/sorry") || strings.HasPrefix(u.Path, "/recaptcha"))
}

func googleVerification(result *Result, response *http.Response, body []byte) bool {
	raw := ""
	if response.Request != nil && response.Request.URL != nil {
		raw = response.Request.URL.String()
	}
	redirect := ""
	if location, err := response.Location(); err == nil {
		redirect = location.String()
	}
	text := strings.ToLower(string(body))
	google := false
	if u, err := url.Parse(raw); err == nil {
		h := u.Hostname()
		google = h == "google.com" || strings.HasSuffix(h, ".google.com") || h == "youtube.com" || strings.HasSuffix(h, ".youtube.com")
	}
	if !googleVerificationURL(raw) && !googleVerificationURL(redirect) && !(google && (strings.Contains(text, "our systems have detected unusual traffic") || strings.Contains(text, `id="captcha-form"`))) {
		return false
	}
	result.Outcome, result.Summary, result.ErrorMessage = "verification_required", "需人机验证", googleVerificationReason
	if result.Details == nil {
		result.Details = map[string]string{}
	}
	result.Details["final_url"] = raw
	if u, err := url.Parse(redirect); err == nil && googleVerificationURL(redirect) {
		u.RawQuery = ""
		result.Details["verification_url"] = u.String()
	}
	return true
}
