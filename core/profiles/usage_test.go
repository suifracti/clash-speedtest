package profiles

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSubscriptionUsageHeader(t *testing.T) {
	u := parseSubscriptionUsage("upload=1024; download=3072; total=8192; expire=2000000000")
	if u == nil || u.Upload != 1024 || u.Download != 3072 || u.Total == nil || *u.Total != 8192 || u.Expire == nil || *u.Expire != 2000000000 || u.UpdatedAt.IsZero() {
		t.Fatalf("unexpected usage: %+v", u)
	}
	for _, input := range []string{"", "upload=1", "upload=-1;download=2", "upload=bad;download=2", "upload=1;download=2;total=secret", "upload=1;upload=2;download=3"} {
		if parseSubscriptionUsage(input) != nil {
			t.Fatalf("accepted invalid header %q", input)
		}
	}
	u = parseSubscriptionUsage("upload=0; download=0; total=0; expire=0")
	if u == nil || u.Total != nil || u.Expire != nil {
		t.Fatal("zero provider limits must not imply known capacity or expiry")
	}
	u = parseSubscriptionUsage("upload=1024; download=3072; total=8192; expire=")
	if u == nil || u.Upload != 1024 || u.Download != 3072 || u.Total == nil || *u.Total != 8192 || u.Expire != nil {
		t.Fatal("empty optional expiry must not discard valid traffic usage")
	}
}

func TestSubscriptionUsageFallbackDoesNotReplaceNodeConfig(t *testing.T) {
	var primary, usageRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() == "clash.meta" {
			usageRequests++
			w.Header().Set("Subscription-Userinfo", "upload=1024; download=3072; total=8192")
			_, _ = w.Write([]byte("different provider format"))
			return
		}
		primary++
		_, _ = w.Write([]byte("proxies: [fixture]"))
	}))
	defer server.Close()

	body, usage, err := FetchSubscriptionWithUsage(server.URL+"/sub?token=FAKE_TEST_TOKEN", "mihomo/test")
	if err != nil || string(body) != "proxies: [fixture]" || usage == nil || usage.Download != 3072 {
		t.Fatalf("fallback changed config or missed usage: body=%q usage=%+v err=%v", body, usage, err)
	}
	if primary != 1 || usageRequests != 1 {
		t.Fatalf("unexpected request count: config=%d usage=%d", primary, usageRequests)
	}
	if _, err := FetchSubscription(server.URL, "mihomo/test"); err != nil || usageRequests != 1 {
		t.Fatal("ordinary config fetch should not issue a usage-only request")
	}
}
