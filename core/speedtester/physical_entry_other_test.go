//go:build !darwin && !windows

package speedtester

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestPhysicalEntryOtherRejectsFakeIPAndDisablesSystemProxy(t *testing.T) {
	if _, err := ResolvePhysicalHost(context.Background(), "198.18.0.25", time.Second); err == nil {
		t.Fatal("expected literal Fake-IPv4 to be rejected")
	}
	if _, err := ResolvePhysicalHost(context.Background(), "fdfe:dcba:9876::1", time.Second); err == nil {
		t.Fatal("expected literal Fake-IPv6 to be rejected")
	}
	if _, err := filterPhysicalResolvedAddresses([]string{"198.18.0.1", "fdfe:dcba:9876::2"}); err == nil {
		t.Fatal("expected Fake-IP-only DNS answers to be rejected")
	}
	filtered, err := filterPhysicalResolvedAddresses([]string{"198.18.0.1", "203.0.113.10", "203.0.113.10"})
	if err != nil || len(filtered) != 1 || filtered[0] != "203.0.113.10" {
		t.Fatalf("unexpected filtered addresses: %v %v", filtered, err)
	}

	resolved, err := ResolvePhysicalHost(context.Background(), "203.0.113.20", time.Second)
	if err != nil || resolved.Source != "literal" || len(resolved.Addresses) != 1 || resolved.Addresses[0] != "203.0.113.20" {
		t.Fatalf("unexpected literal resolution: %+v %v", resolved, err)
	}

	client, err := NewPhysicalHTTPClient(2 * time.Second)
	if err != nil || client == nil {
		t.Fatalf("NewPhysicalHTTPClient: %v", err)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport == nil || transport.Proxy != nil {
		t.Fatalf("expected explicit http.Transport with Proxy == nil, got %#v", client.Transport)
	}
}
