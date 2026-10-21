package publicservice

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestGoogleRegionReusesYouTubeResult(t *testing.T) {
	for _, tc := range []struct{ name, body, want, region string }{
		{"premium page region", `<title>YouTube Premium</title>{"countryCode":"HK"}`, "profiled", "HK"},
		{"no premium evidence", `<html>Welcome</html>`, "unknown", ""},
		{"mainland signal", `<a href="https://www.google.cn/">link</a>`, "profiled", "CN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return fixtureResponse(req, 200, "text/html", tc.body), nil
			})}
			result := checkGoogleRegion(context.Background(), client, googleRegionRule(), time.Now())
			if result.Outcome != tc.want || (tc.region != "" && result.Details["service_region"] != tc.region) {
				t.Fatalf("got %+v, want %s/%s", result, tc.want, tc.region)
			}
		})
	}
}
