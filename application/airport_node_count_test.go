package application

import (
	"testing"

	"github.com/faceair/clash-speedtest/core/profiles"
)

func TestListAirportsCountsLargestCachedSubscriptionInsteadOfSumming(t *testing.T) {
	service, paths := newAirportURLTestService(t)
	if err := profiles.SaveStore(paths.StoreFile(), &profiles.Store{Airports: []*profiles.Airport{{
		ID: "airport-a", Name: "Fixture airport",
		Subscriptions: []*profiles.Subscription{
			{ID: "sub-a", Name: "Primary"},
			{ID: "sub-b", Name: "Mirror"},
		},
	}}}); err != nil {
		t.Fatal(err)
	}
	cache := []byte("proxies:\n  - name: fixture-1\n    type: ss\n    server: 192.0.2.1\n    port: 8388\n    cipher: aes-128-gcm\n    password: fixture-password\n  - name: fixture-2\n    type: ss\n    server: 192.0.2.2\n    port: 8388\n    cipher: aes-128-gcm\n    password: fixture-password\n")
	for _, id := range []string{"sub-a", "sub-b"} {
		if err := paths.WriteCache(id, cache); err != nil {
			t.Fatal(err)
		}
	}

	airports, err := service.ListAirports()
	if err != nil {
		t.Fatal(err)
	}
	if len(airports) != 1 || airports[0].NodeCount != 2 {
		t.Fatalf("airport count should represent one node inventory, got %+v", airports)
	}
	for _, sub := range airports[0].Subscriptions {
		if sub.NodeCount != 2 {
			t.Fatalf("subscription %q lost its own count: %+v", sub.ID, sub)
		}
	}
}
