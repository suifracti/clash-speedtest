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
