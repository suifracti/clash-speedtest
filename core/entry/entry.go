// Package entry resolves a node's entry address and looks up where that address
// sits. Resolution uses the physical interface so a running TUN never answers
// for the node itself; the geo lookup reuses the same public IP source as
// core/ip (ip-api.com) and is cached per address.
package entry

import (
	"fmt"
	"net/netip"
	"strings"
	"time"
)

// Address is one resolved entry address with its public picture.
type Address struct {
	IP          string    `json:"ip"`
	CountryCode string    `json:"country_code,omitempty"`
	Country     string    `json:"country,omitempty"`
	City        string    `json:"city,omitempty"`
	ISP         string    `json:"isp,omitempty"`
	ASN         string    `json:"asn,omitempty"`
	QueriedAt   time.Time `json:"queried_at,omitempty"`
}

// Host is the DNS outcome for one node server. Addresses keep DNS order, so the
// first record is the primary entry.
type Host struct {
	Host       string    `json:"host"`
	Source     string    `json:"source"` // literal | dns | cache
	Addresses  []Address `json:"addresses"`
	ResolvedAt time.Time `json:"resolved_at"`
	Error      string    `json:"error,omitempty"`
}

// Local is this machine's own public position. The address itself is only kept
// masked: the UI never needs the full value.
type Local struct {
	CountryCode string    `json:"country_code,omitempty"`
	Country     string    `json:"country,omitempty"`
	ISP         string    `json:"isp,omitempty"`
	IPMasked    string    `json:"ip_masked,omitempty"`
	QueriedAt   time.Time `json:"queried_at,omitempty"`
	Error       string    `json:"error,omitempty"`
}

// MaskIP keeps enough of an address to recognise it without publishing it.
func MaskIP(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if addr, err := netip.ParseAddr(trimmed); err == nil {
		if addr.Is4() {
			parts := strings.Split(addr.String(), ".")
			if len(parts) == 4 {
				return fmt.Sprintf("%s.*.*.%s", parts[0], parts[3])
			}
		}
		text := addr.String()
		if index := strings.LastIndex(text, ":"); index > 0 {
			return text[:min(4, index)] + ":…:" + text[index+1:]
		}
	}
	if strings.Contains(trimmed, ":") {
		parts := strings.Split(trimmed, ":")
		return parts[0] + ":…:" + parts[len(parts)-1]
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) == 4 {
		return fmt.Sprintf("%s.*.*.%s", parts[0], parts[3])
	}
	return trimmed
}
