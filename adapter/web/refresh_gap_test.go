package web

import (
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/core/profiles"
)

// ageUsageAttempts clears the per-airport 30-minute gap so a test can exercise
// a second fetch without waiting half an hour.
func ageUsageAttempts(t *testing.T, paths profiles.Paths) {
	t.Helper()
	store, err := profiles.LoadStore(paths.StoreFile())
	if err != nil {
		t.Fatal(err)
	}
	for _, airport := range store.Airports {
		if airport == nil {
			continue
		}
		for _, sub := range airport.Subscriptions {
			if sub != nil {
				sub.LastUsageAttemptAt = time.Time{}
				sub.LastUsageFailed = false
			}
		}
	}
	if err := profiles.SaveStore(paths.StoreFile(), store); err != nil {
		t.Fatal(err)
	}
}
