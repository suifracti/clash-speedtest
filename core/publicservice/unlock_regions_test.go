package publicservice

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestSpotifySelectorUsesCurrentCountryAndRejectsCountryLists(t *testing.T) {
	rule := regionUnlockRule("spotify_unlock", "Spotify 地区判定", "流媒体", "全球", spotifyRegionURL, http.MethodGet, "test")
	run := func(path, body string) Result {
		client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			response := fixtureResponse(request, http.StatusOK, "application/json", body)
			finalRequest := request.Clone(request.Context())
			finalURL := *request.URL
			finalURL.Path = path
			finalRequest.URL = &finalURL
			response.Request = finalRequest
			return response, nil
		})}
		return checkSpotifyRegion(context.Background(), client, rule, time.Now())
	}

	selected := run("/us/api/content/v1/country-selector", `{"countries":[{"countryCode":"JP"}]}`)
	if selected.Outcome != "unlocked" || selected.Details["service_region"] != "US" {
		t.Fatalf("localized selector region was not used: %+v", selected)
	}
	listOnly := run("/api/content/v1/country-selector", `{"countries":[{"countryCode":"JP"},{"countryCode":"GB"}]}`)
	if listOnly.Outcome != "unknown" {
		t.Fatalf("generic country list must remain unknown: %+v", listOnly)
	}
}
