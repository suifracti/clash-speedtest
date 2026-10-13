package application

import (
	"context"
	"github.com/faceair/clash-speedtest/core/profiles"
	"strings"
	"testing"
)

func TestCachedVLESSPoliciesRemainSelectableByFullScope(t *testing.T) {
	s, p := newAirportURLTestService(t)
	profiles.SaveStore(p.StoreFile(), &profiles.Store{Airports: []*profiles.Airport{{ID: "same", Name: "same", URL: "/fixture"}}})
	prefix := "vless://11111111-1111-1111-1111-111111111111@example.com:443?security=tls&type=tcp&sni=original.example&"
	p.WriteCache("same", []byte(prefix+"insecure=0#strict\n"+prefix+"insecure=1#insecure\n"+prefix+"pcs="+strings.Repeat("a", 64)+"#pinned"))
	opts, err := s.ListMonitorNodeOptions()
	if err != nil {
		t.Fatal(err)
	}
	if len(opts) != 3 {
		t.Fatalf("TLS policies removed before selection: %d", len(opts))
	}
	for _, o := range opts {
		node, _, proxy, err := s.resolveWorkbenchLatencyProxy("same", o.NodeKey, o.ConfigRevisionKey)
		if err != nil {
			t.Fatal(err)
		}
		if node.ConfigRevisionKey != o.ConfigRevisionKey || proxy.Config["servername"] != "original.example" {
			t.Fatal("wrong source TLS policy selected")
		}
	}
	if _, _, _, err := s.resolveWorkbenchLatencyProxy("same", opts[0].NodeKey); err == nil {
		t.Fatal("ambiguous legacy key silently selected a TLS policy")
	}
}

func TestExportCannotSilentlyDiscardSourceCertificatePins(t *testing.T) {
	s, p := newAirportURLTestService(t)
	profiles.SaveStore(p.StoreFile(), &profiles.Store{Airports: []*profiles.Airport{{ID: "pin", Name: "pin", URL: "/fixture"}}})
	p.WriteCache("pin", []byte("vless://11111111-1111-1111-1111-111111111111@example.com:443?security=tls&type=tcp&insecure=1&pcs="+strings.Repeat("a", 64)+"#pinned"))
	opts, _ := s.ListMonitorNodeOptions()
	if len(opts) != 1 {
		t.Fatal("pin missing")
	}
	if _, err := s.ExportNodesClashConfig(context.Background(), ExportClashConfigRequest{NodeKeys: []string{opts[0].NodeKey}}); err == nil {
		t.Fatal("export silently weakened certificate pin")
	}
	if result, err := s.ExportNodesClashConfig(context.Background(), ExportClashConfigRequest{Selections: []WorkbenchLatencyBatchSelection{{ProfileID: "pin", NodeKey: opts[0].NodeKey, NodeIdentityKey: opts[0].NodeIdentityKey, ConfigRevisionKey: opts[0].ConfigRevisionKey}}}); err == nil || result != nil {
		t.Fatal("scoped export silently weakened certificate pin")
	}

}
