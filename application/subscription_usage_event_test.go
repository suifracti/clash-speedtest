package application

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/profiles"
)

// Start-up and scheduled runs have no client request behind them; the pages
// showing counters only refresh when the run announces itself.
func TestUsageRefreshEmitsCompletionEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Subscription-Userinfo", fmtUsage(100))
		_, _ = w.Write([]byte("proxies: []"))
	}))
	defer server.Close()
	root := t.TempDir()
	paths := profiles.Paths{Dir: filepath.Join(root, "profiles")}
	if err := paths.InitializeEmpty(); err != nil {
		t.Fatalf("InitializeEmpty: %v", err)
	}
	historyStore, err := history.NewStore(filepath.Join(root, "history"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	emitter := NewMemoryEventEmitter()
	var mu sync.Mutex
	var events []Event
	emitter.Subscribe(func(event Event) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	})
	service := NewAppService(historyStore, paths, emitter)
	t.Cleanup(func() { _ = service.Close() })
	if _, err := service.CreateAirport("Event fixture", server.URL, "fixture"); err != nil {
		t.Fatalf("CreateAirport: %v", err)
	}
	usageEvents := func() []Event {
		mu.Lock()
		defer mu.Unlock()
		var found []Event
		for _, event := range events {
			if event.Type == "subscription_usage_updated" {
				found = append(found, event)
			}
		}
		return found
	}
	now := time.Now()
	if _, err := service.collectSubscriptionUsage(context.Background(), now, "fixture", true); err != nil {
		t.Fatalf("collect: %v", err)
	}
	emitted := usageEvents()
	if len(emitted) != 1 {
		t.Fatalf("usage run emitted %d events, want 1", len(emitted))
	}
	if payload, ok := emitted[0].Payload.(map[string]any); !ok || payload["state"] != "completed" {
		t.Fatalf("unexpected payload: %+v", emitted[0].Payload)
	}
	// A run that never leaves the local cache must stay silent.
	if _, err := service.collectSubscriptionUsage(context.Background(), now.Add(time.Minute), "fixture", true); err != nil {
		t.Fatalf("cached collect: %v", err)
	}
	if got := len(usageEvents()); got != 1 {
		t.Fatalf("cached run emitted an extra event: %d", got)
	}
}
