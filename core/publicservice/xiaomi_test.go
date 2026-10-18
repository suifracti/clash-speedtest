package publicservice

import (
	"context"
	"net/http"
	"testing"

	"github.com/faceair/clash-speedtest/core/monitor"
)

func TestXiaomiNetworkContrastRequires204(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   string
	}{
		{204, "matched"},
		{200, "http_rejected"},
		{302, "redirect"},
	} {
		rule, _ := RuleFor("xiaomi_204")
		calls := 0
		c := Checker{ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
			calls++
			r := fixtureResponse(req, tc.status, "text/html", "<html>intercepted page</html>")
			if tc.status == 302 {
				r.Header.Set("Location", "https://connect.rom.miui.com/other")
			}
			return r, nil
		})}
		r := c.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
		if r.Outcome != tc.want || calls != 1 {
			t.Fatalf("status=%d calls=%d outcome=%s want=%s", tc.status, calls, r.Outcome, tc.want)
		}
	}
}
