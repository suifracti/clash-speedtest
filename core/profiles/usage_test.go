package profiles

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestSubscriptionUsageHeader(t *testing.T) {
	u := parseSubscriptionUsage("upload=1024; download=3072; total=8192; expire=2000000000")
	if u == nil || u.Upload == nil || *u.Upload != 1024 || u.Download == nil || *u.Download != 3072 || u.Total == nil || *u.Total != 8192 || u.Expire == nil || *u.Expire != 2000000000 || u.UpdatedAt.IsZero() {
		t.Fatalf("unexpected usage: %+v", u)
	}
	for _, input := range []string{"", "   "} {
		if parseSubscriptionUsage(input) != nil {
			t.Fatalf("accepted invalid header %q", input)
		}
	}
	u = parseSubscriptionUsage("upload=0; download=0; total=0; expire=0")
	if u == nil || u.Total != nil || u.Expire != nil {
		t.Fatal("zero provider limits must not imply known capacity or expiry")
	}
	u = parseSubscriptionUsage("upload=1024; download=3072; total=8192; expire=")
	if u == nil || u.Upload == nil || *u.Upload != 1024 || u.Download == nil || *u.Download != 3072 || u.Total == nil || *u.Total != 8192 || u.Expire != nil {
		t.Fatal("empty optional expiry must not discard valid traffic usage")
	}
}

func TestDefaultSubscriptionUserAgentUsesClashVergeCompatibility(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.UserAgent(); got != "clash-verge/v2.5.8" {
			t.Errorf("default subscription user agent = %q", got)
		}
		w.Header().Set("Subscription-Userinfo", "upload=1024;download=3072;total=8192")
		_, _ = w.Write([]byte("proxies: [fixture]"))
	}))
	defer server.Close()

	_, usage, err := FetchSubscriptionWithUsage(server.URL, "")
	if err != nil || usage == nil || usage.Total == nil || *usage.Total != 8192 {
		t.Fatalf("default compatible fetch failed: usage=%+v err=%v", usage, err)
	}
}

func TestFetchSubscriptionUsageRecognizesObjectStorageMetadataHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Oss-Meta-Subscription-Userinfo", "upload=1024;download=3072;total=8192;expire=2000000000")
		_, _ = w.Write([]byte("proxies: [fixture]"))
	}))
	defer server.Close()

	_, usage, err := FetchSubscriptionWithUsage(server.URL, "fixture")
	if err != nil || usage == nil || usage.Upload == nil || *usage.Upload != 1024 || usage.Download == nil || *usage.Download != 3072 || usage.Total == nil || *usage.Total != 8192 {
		t.Fatalf("object-storage usage header was not parsed: usage=%+v err=%v", usage, err)
	}
}

func TestCompatiblePrimaryUserAgentKeepsUsageAndBody(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.UserAgent() != "clash.meta" {
			t.Errorf("unexpected user agent %q", r.UserAgent())
		}
		w.Header().Set("Subscription-Userinfo", "upload=1024;download=3072;total=8192")
		_, _ = w.Write([]byte("proxies: [fixture]"))
	}))
	defer server.Close()
	body, usage, err := FetchSubscriptionWithUsage(server.URL, "clash.meta")
	if err != nil || string(body) != "proxies: [fixture]" || usage == nil || usage.Download == nil || *usage.Download != 3072 || calls != 1 {
		t.Fatalf("single compatible primary request failed: calls=%d usage=%+v err=%v", calls, usage, err)
	}
}

func TestMissingUsageHeaderTriesOnlyOneHeaderFallback(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("proxies: []"))
	}))
	defer server.Close()

	body, usage, err := FetchSubscriptionWithUsageContext(context.Background(), server.URL, "fixture")
	if err != nil || string(body) != "proxies: []" || usage != nil {
		t.Fatalf("fetch result = (%q, %v, %v)", body, usage, err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("missing usage header caused %d requests, want primary plus one header fallback", got)
	}
}
