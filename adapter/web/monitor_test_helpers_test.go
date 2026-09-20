package web

import (
	"fmt"
	"testing"

	"github.com/faceair/clash-speedtest/core/profiles"
	"gopkg.in/yaml.v2"
)

// seedWebMonitorProfile creates the same cache shape the production resolver
// reads. It keeps HTTP tests on the safe node-key creation path rather than
// sending raw subscription credentials through the API.
func seedWebMonitorProfile(t *testing.T, profileDir, profileID, profileName string, nodes ...string) {
	t.Helper()
	paths := profiles.Paths{Dir: profileDir}
	if err := profiles.SaveStore(paths.StoreFile(), &profiles.Store{Airports: []*profiles.Airport{{
		ID:   profileID,
		Name: profileName,
	}}}); err != nil {
		t.Fatalf("SaveStore: %v", err)
	}
	proxies := make([]map[string]any, 0, len(nodes))
	for i, name := range nodes {
		proxies = append(proxies, map[string]any{
			"name":     name,
			"type":     "ss",
			"server":   fmt.Sprintf("192.0.2.%d", i+1),
			"port":     8388,
			"cipher":   "aes-128-gcm",
			"password": "test-password",
		})
	}
	body, err := yaml.Marshal(map[string]any{"proxies": proxies})
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	if err := paths.WriteCache(profileID, body); err != nil {
		t.Fatalf("WriteCache: %v", err)
	}
}
