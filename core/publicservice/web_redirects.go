package publicservice

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type webRedirectEvidence struct {
	finalURL string
	hops     int
	stop     string
}

// Keep canonical and regional redirects on the selected node's existing client.
// A redirect to another service remains visible instead of proving availability.
func serviceRedirectClient(client *http.Client, rule Rule) (*http.Client, *webRedirectEvidence) {
	if !isReachabilityRule(rule) || rule.RedirectPolicy != "follow_same_service" {
		return client, nil
	}
	initial, err := url.Parse(rule.TargetURL)
	if err != nil {
		return client, nil
	}
	evidence := &webRedirectEvidence{finalURL: rule.TargetURL}
	clone := *client
	clone.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) > 3 {
			evidence.stop = "redirect_limit"
			return http.ErrUseLastResponse
		}
		if next.URL.Scheme != "https" || next.URL.User != nil || (next.URL.Port() != "" && next.URL.Port() != "443") {
			evidence.stop = "unsupported_redirect"
			return http.ErrUseLastResponse
		}
		host := strings.ToLower(next.URL.Hostname())
		base := strings.TrimPrefix(strings.ToLower(initial.Hostname()), "www.")
		allowed := strings.TrimPrefix(host, "www.") == base
		// These sibling entrypoints were returned by the official starting URL.
		if rule.ServiceID == "gitlab_web" && host == "about.gitlab.com" {
			allowed = true
		}
		if rule.ServiceID == "nintendo_jp" && host == "store-jp-wr.nintendo.com" {
			allowed = true
		}
		if rule.ServiceID == "nhk_plus_jp" && host == "www.web.nhk" {
			allowed = true
		}
		if !allowed {
			evidence.stop = "outside_service"
			return http.ErrUseLastResponse
		}
		evidence.hops++
		evidence.finalURL = next.URL.String()
		return nil
	}
	return &clone, evidence
}

func (e *webRedirectEvidence) attach(result *Result) {
	if e == nil || result == nil {
		return
	}
	if result.Details == nil {
		result.Details = map[string]string{}
	}
	result.Details["final_url"] = serviceEvidenceURL(e.finalURL)
	result.Details["redirect_hops"] = strconv.Itoa(e.hops)
	result.Details["redirect_evidence_scope"] = "first_sample"
	if e.stop != "" {
		result.Details["redirect_stop_reason"] = e.stop
	}
}

func serviceEvidenceURL(raw string) string {
	// Response redirects can carry tracking/auth query data; preserve only the endpoint.
	u, err := url.Parse(raw)
	if err != nil {
		return "[invalid redirect URL]"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	safe := []rune(u.String())
	if len(safe) > 512 {
		safe = safe[:512]
	}
	return string(safe)
}
