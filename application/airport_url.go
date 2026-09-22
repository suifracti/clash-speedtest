package application

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/faceair/clash-speedtest/core/profiles"
)

// safeAirportURLDisplay is intentionally non-reversible. It gives the user
// enough source identity for ordinary list views without carrying a path,
// query, or credential-like value across the DTO boundary.
func safeAirportURLDisplay(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !profiles.IsHTTPURL(raw) {
		return "本地来源（已配置）"
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "远程订阅（已配置）"
	}
	return strings.ToLower(parsed.Scheme) + "://" + parsed.Host + "/•••"
}

func airportDTO(airport *profiles.Airport, nodeCount int, hasCache bool) AirportDTO {
	if airport == nil {
		return AirportDTO{}
	}
	return AirportDTO{
		ID:            airport.ID,
		Name:          airport.Name,
		URLDisplay:    safeAirportURLDisplay(airport.URL),
		URLConfigured: strings.TrimSpace(airport.URL) != "",
		UpdatedAt:     airport.UpdatedAt,
		NodeCount:     nodeCount,
		HasCache:      hasCache,
	}
}

// airportOperationError deliberately omits the underlying error because
// HTTP/client and filesystem errors can echo the complete configured source.
// The safe display is sufficient for a normal UI error and keeps this boundary
// from becoming another URL disclosure path.
func airportOperationError(action, airportID, rawURL string) error {
	source := safeAirportURLDisplay(rawURL)
	if source == "" {
		return fmt.Errorf("%s（机场 %s）", action, airportID)
	}
	return fmt.Errorf("%s（机场 %s，来源 %s）", action, airportID, source)
}
