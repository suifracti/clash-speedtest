package publicservice

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestGoogleRegionUsesProviderRegionWithoutRequiringPremium(t *testing.T) {
	for _, tc := range []struct{ body, want string }{{`{"countryCode":"HK"}`, "profiled"}, {`<html>Welcome</html>`, "unknown"}} {
		client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return fixtureResponse(req, 200, "text/html", tc.body), nil
		})}
		result := checkGoogleRegion(context.Background(), client, googleRegionRule(), time.Now())
		if result.Outcome != tc.want {
			t.Fatalf("got %+v, want %s", result, tc.want)
		}
	}
}
