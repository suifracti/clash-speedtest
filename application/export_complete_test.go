package application

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/metacubex/mihomo/adapter"
	"gopkg.in/yaml.v3"
)

func legacyExportFixture(t *testing.T, configs ...map[string]any) (*AppService, string) {
	t.Helper()
	a, _ := scopedWorkbenchFixture(t, configs...)
	run := &history.TestRun{AirportID: "scope-a"}
	for _, cfg := range configs {
		run.Results = append(run.Results, &history.RunNodeResult{ProxyName: cfg["name"].(string)})
	}
	id, err := a.historyStore.Save(run)
	if err != nil {
		t.Fatal(err)
	}
	return a, id
}

func TestLegacyExportsPreserveUsableProxySettingsAndSelectionOrder(t *testing.T) {
	for _, fromRun := range []bool{false, true} {
		t.Run(fmt.Sprint("history=", fromRun), func(t *testing.T) {
			first, second := scopeHTTPConfig("first", false), scopeHTTPConfig("second", true)
			first["username"], first["password"] = "fixture-user", "fixture-password"
			second["username"], second["password"] = "fixture-user-2", "fixture-password-2"
			second["_cst_generated_ws_ua"] = true
			a, id := legacyExportFixture(t, first, second)
			export := func(names []string) ([]byte, error) { return a.ExportClashConfig("scope-a", names) }
			if fromRun {
				export = func(names []string) ([]byte, error) { return a.ExportClashConfigFromResults(id, names) }
			}
			data, err := export([]string{"second", "first"})
			if err != nil {
				t.Fatal(err)
			}
			var doc struct {
				Proxies []map[string]any `yaml:"proxies"`
			}
			if err := yaml.Unmarshal(data, &doc); err != nil {
				t.Fatal(err)
			}
			if len(doc.Proxies) != 2 {
				t.Fatalf("lost selected proxies: %d", len(doc.Proxies))
			}
			for i, want := range []map[string]any{second, first} {
				got := doc.Proxies[i]
				for _, field := range []string{"name", "type", "server", "port", "username", "password", "tls", "sni"} {
					if got[field] != want[field] {
						t.Errorf("proxy %d lost or changed %s", i, field)
					}
				}
				if _, leaked := got["_cst_generated_ws_ua"]; leaked {
					t.Error("internal marker leaked into export")
				}
				if _, err := adapter.ParseProxy(got); err != nil {
					t.Errorf("exported proxy cannot be loaded: %v", err)
				}
			}
		})
	}
}

func TestLegacyExportsRejectPartialSelection(t *testing.T) {
	for _, fromRun := range []bool{false, true} {
		t.Run(fmt.Sprint("history=", fromRun), func(t *testing.T) {
			cfg := scopeHTTPConfig("available", true)
			a, id := legacyExportFixture(t, cfg)
			var data []byte
			var err error
			if fromRun {
				data, err = a.ExportClashConfigFromResults(id, []string{"available", "missing"})
			} else {
				data, err = a.ExportClashConfig("scope-a", []string{"available", "missing"})
			}
			if err == nil || len(data) != 0 {
				t.Fatal("partial export silently omitted a selected node")
			}
		})
	}
}

func TestLegacyExportsRefuseToDiscardSourceTLSPins(t *testing.T) {
	for _, fromRun := range []bool{false, true} {
		t.Run(fmt.Sprint("history=", fromRun), func(t *testing.T) {
			cfg := scopeHTTPConfig("pinned", true)
			cfg["_cst_tls_pins"] = []string{strings.Repeat("a", 64)}
			a, id := legacyExportFixture(t, cfg)
			var data []byte
			var err error
			if fromRun {
				data, err = a.ExportClashConfigFromResults(id, nil)
			} else {
				data, err = a.ExportClashConfig("scope-a", nil)
			}
			if err == nil || len(data) != 0 {
				t.Fatal("export silently discarded the source TLS pin")
			}
		})
	}
}

func TestHistoryExportCannotIncludeUnmeasuredCachedNodes(t *testing.T) {
	a, _ := scopedWorkbenchFixture(t, scopeHTTPConfig("measured", false), scopeHTTPConfig("other", false))
	id, err := a.historyStore.Save(&history.TestRun{AirportID: "scope-a", Results: []*history.RunNodeResult{{ProxyName: "measured"}}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := a.ExportClashConfigFromResults(id, nil)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Proxies) != 1 || doc.Proxies[0]["name"] != "measured" {
		t.Fatal("history export included a cached node outside the run")
	}
	if data, err := a.ExportClashConfigFromResults(id, []string{"other"}); err == nil || len(data) != 0 {
		t.Fatal("explicit selection escaped the historical run")
	}
}

func TestWorkbenchExportUsesUniqueNamesAcrossExistingSuffixesAndReservedNames(t *testing.T) {
	names := []string{"AUTO", "AUTO (Proxy)", "AUTO (Proxy) (2)", "PROXY", "PROXY (Proxy)", "PROXY (Proxy) (2)", "DIRECT", "REJECT", "GLOBAL", "COMPATIBLE", "PASS", "REJECT-DROP"}
	configs := make([]map[string]any, 0, len(names))
	for i, name := range names {
		cfg := scopeHTTPConfig(name, false)
		cfg["port"] = 8400 + i
		configs = append(configs, cfg)
	}
	a, requests := scopedWorkbenchFixture(t, configs...)
	var selected []WorkbenchLatencyBatchSelection
	for _, cfg := range configs {
		identity := monitor.ComputeNodeIdentityKeyFromConfig(cfg)
		selected = append(selected, WorkbenchLatencyBatchSelection{ProfileID: "scope-a", NodeKey: monitor.ComputeNodeKeyFromConfig(cfg), NodeIdentityKey: identity, ConfigRevisionKey: monitor.ComputeConfigRevisionKey(identity, cfg)})
	}
	r, err := a.ExportNodesClashConfig(context.Background(), ExportClashConfigRequest{Selections: selected})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{"AUTO": true, "FALLBACK": true, "PROXY": true, "DIRECT": true, "REJECT": true, "GLOBAL": true, "COMPATIBLE": true, "PASS": true, "REJECT-DROP": true}
	for _, name := range r.NodeNames {
		if seen[name] {
			t.Errorf("duplicate or reserved exported name %q", name)
		}
		seen[name] = true
	}
	var doc struct {
		Proxies []map[string]any `yaml:"proxies"`
		Groups  []struct {
			Proxies []string `yaml:"proxies"`
		} `yaml:"proxy-groups"`
	}
	if err := yaml.Unmarshal([]byte(r.YAMLContent), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Proxies) != len(names) || len(doc.Groups) != 3 {
		t.Fatal("export lost selected nodes or proxy groups")
	}
	for i, proxy := range doc.Proxies {
		if proxy["name"] != r.NodeNames[i] || proxy["port"] != 8400+i {
			t.Fatal("renaming changed selection order or selected configuration")
		}
		for _, group := range doc.Groups {
			found := false
			for _, name := range group.Proxies {
				found = found || name == r.NodeNames[i]
			}
			if !found {
				t.Error("proxy group lost a renamed node")
			}
		}
	}
	if requests.Load() != 0 {
		t.Fatal("export issued a network probe")
	}
}

func TestWorkbenchExportGeneratedNameCannotShadowCustomGroup(t *testing.T) {
	cfg := scopeHTTPConfig("AUTO", false)
	a, _ := scopedWorkbenchFixture(t, cfg)
	identity := monitor.ComputeNodeIdentityKeyFromConfig(cfg)
	selection := WorkbenchLatencyBatchSelection{ProfileID: "scope-a", NodeKey: monitor.ComputeNodeKeyFromConfig(cfg), NodeIdentityKey: identity, ConfigRevisionKey: monitor.ComputeConfigRevisionKey(identity, cfg)}
	r, err := a.ExportNodesClashConfig(context.Background(), ExportClashConfigRequest{Selections: []WorkbenchLatencyBatchSelection{selection}, GroupName: "AUTO (Proxy)"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.NodeNames) != 1 || r.NodeNames[0] == "AUTO (Proxy)" {
		t.Fatal("generated proxy name shadows the selected policy group")
	}
}
