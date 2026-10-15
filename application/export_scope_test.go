package application

import (
	"context"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/profiles"
	"gopkg.in/yaml.v3"
	"os"
	"testing"
)

func TestExportSelectedRevisionAndRefusePartialOrAmbiguous(t *testing.T) {
	a, requests := scopedWorkbenchFixture(t, scopeHTTPConfig("plain", false), scopeHTTPConfig("tls", true))
	cfg := scopeHTTPConfig("tls", true)
	identity := monitor.ComputeNodeIdentityKeyFromConfig(cfg)
	selected := WorkbenchLatencyBatchSelection{ProfileID: "scope-a", NodeKey: monitor.ComputeNodeKeyFromConfig(cfg), NodeIdentityKey: identity, ConfigRevisionKey: monitor.ComputeConfigRevisionKey(identity, cfg)}
	result, err := a.ExportNodesClashConfig(context.Background(), ExportClashConfigRequest{Selections: []WorkbenchLatencyBatchSelection{selected}})
	if err != nil {
		t.Fatal(err)
	}
	var content struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if err := yaml.Unmarshal([]byte(result.YAMLContent), &content); err != nil {
		t.Fatal(err)
	}
	if len(content.Proxies) != 1 || content.Proxies[0]["tls"] != true {
		t.Fatal("wrong TLS config exported")
	}
	stale := selected
	stale.ConfigRevisionKey = "stale"
	if _, err := a.ExportNodesClashConfig(context.Background(), ExportClashConfigRequest{Selections: []WorkbenchLatencyBatchSelection{selected, stale}}); err == nil {
		t.Fatal("partial export accepted stale selection")
	}
	if _, err := a.ExportNodesClashConfig(context.Background(), ExportClashConfigRequest{NodeKeys: []string{selected.NodeKey}}); err == nil {
		t.Fatal("legacy ambiguous key silently chose a revision")
	}
	if requests.Load() != 0 {
		t.Fatal("export issued a target request")
	}
}

func TestExportAcrossAirportsSameKeyKeepsSelectedConfiguration(t *testing.T) {
	first := scopeHTTPConfig("same-name", false)
	second := scopeHTTPConfig("same-name", true)
	a, requests := scopedWorkbenchFixture(t, first)
	if err := profiles.SaveStore(a.profilePaths.StoreFile(), &profiles.Store{Airports: []*profiles.Airport{{ID: "scope-a", Name: "A"}, {ID: "scope-b", Name: "B"}}}); err != nil {
		t.Fatal(err)
	}
	data, _ := yaml.Marshal(map[string]any{"proxies": []map[string]any{second}})
	if err := os.WriteFile(a.profilePaths.CacheFile("scope-b"), data, 0600); err != nil {
		t.Fatal(err)
	}
	selectNode := func(profile string, cfg map[string]any) WorkbenchLatencyBatchSelection {
		identity := monitor.ComputeNodeIdentityKeyFromConfig(cfg)
		return WorkbenchLatencyBatchSelection{ProfileID: profile, NodeKey: monitor.ComputeNodeKeyFromConfig(cfg), NodeIdentityKey: identity, ConfigRevisionKey: monitor.ComputeConfigRevisionKey(identity, cfg)}
	}
	x, y := selectNode("scope-a", first), selectNode("scope-b", second)
	if x.NodeKey != y.NodeKey {
		t.Fatal("fixture must collide on legacy key")
	}
	result, err := a.ExportNodesClashConfig(context.Background(), ExportClashConfigRequest{Selections: []WorkbenchLatencyBatchSelection{y, x}})
	if err != nil {
		t.Fatal(err)
	}
	var content struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if err := yaml.Unmarshal([]byte(result.YAMLContent), &content); err != nil {
		t.Fatal(err)
	}
	if len(content.Proxies) != 2 || content.Proxies[0]["tls"] != true || content.Proxies[1]["tls"] != false || content.Proxies[0]["name"] == content.Proxies[1]["name"] {
		t.Fatal("export changed selected order/config or duplicated names")
	}
	if _, err := a.ExportNodesClashConfig(context.Background(), ExportClashConfigRequest{NodeKeys: []string{x.NodeKey}}); err == nil {
		t.Fatal("cross-airport legacy key must reject")
	}
	if requests.Load() != 0 {
		t.Fatal("target request during export")
	}
}

func TestExportStripsInternalCSTMarkersAndPreservesStandardFields(t *testing.T) {
	cfg := map[string]any{
		"name":                 "vless-ws-reality",
		"type":                 "vless",
		"server":               "203.0.113.10",
		"port":                 443,
		"uuid":                 "a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d",
		"tls":                  true,
		"network":              "ws",
		"ws-opts":              map[string]any{"path": "/graphql", "headers": map[string]any{"Host": "example.com"}},
		"reality-opts":         map[string]any{"public-key": "AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE", "short-id": "01"},
		"_cst_generated_ws_ua": true,
	}
	a, requests := scopedWorkbenchFixture(t, cfg)
	nodes, err := a.loadMonitorNodes("scope-a")
	if err != nil {
		t.Fatalf("loadMonitorNodes failed: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	selected := WorkbenchLatencyBatchSelection{
		ProfileID:         "scope-a",
		NodeKey:           nodes[0].NodeKey,
		NodeIdentityKey:   nodes[0].NodeIdentityKey,
		ConfigRevisionKey: nodes[0].ConfigRevisionKey,
	}
	result, err := a.ExportNodesClashConfig(context.Background(), ExportClashConfigRequest{Selections: []WorkbenchLatencyBatchSelection{selected}})
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}
	var content struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if err := yaml.Unmarshal([]byte(result.YAMLContent), &content); err != nil {
		t.Fatalf("failed to unmarshal yaml: %v", err)
	}
	if len(content.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(content.Proxies))
	}
	proxy := content.Proxies[0]
	if _, ok := proxy["_cst_generated_ws_ua"]; ok {
		t.Fatal("exported proxy config leaked internal _cst_generated_ws_ua marker")
	}
	if proxy["type"] != "vless" || proxy["network"] != "ws" || proxy["tls"] != true {
		t.Fatalf("standard proxy fields corrupted: %+v", proxy)
	}
	if proxy["reality-opts"] == nil {
		t.Fatal("reality-opts was not preserved in exported proxy")
	}
	if proxy["ws-opts"] == nil {
		t.Fatal("ws-opts was not preserved in exported proxy")
	}
	if requests.Load() != 0 {
		t.Fatal("target request during export")
	}
}

func TestExportAirportMultipleSubscriptionsFallbackNoDuplication(t *testing.T) {
	cfg := scopeHTTPConfig("unique-node", true)
	a, _ := scopedWorkbenchFixture(t, cfg)
	store := &profiles.Store{
		Airports: []*profiles.Airport{
			{
				ID:   "scope-a",
				Name: "Multi-Sub-Airport",
				Subscriptions: []*profiles.Subscription{
					{ID: "sub-1", Name: "Sub 1", URL: "https://example.com/sub1"},
					{ID: "sub-2", Name: "Sub 2", URL: "https://example.com/sub2"},
				},
			},
		},
	}
	if err := profiles.SaveStore(a.profilePaths.StoreFile(), store); err != nil {
		t.Fatal(err)
	}

	nodeKey := monitor.ComputeNodeKeyFromConfig(cfg)
	res, err := a.ExportNodesClashConfig(context.Background(), ExportClashConfigRequest{
		NodeKeys: []string{nodeKey},
	})
	if err != nil {
		t.Fatalf("expected export to succeed without false duplicate error, got: %v", err)
	}
	if res.NodeCount != 1 {
		t.Fatalf("expected 1 exported node, got %d", res.NodeCount)
	}
}

