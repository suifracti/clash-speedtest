package publicservice

import (
	"context"
	"fmt"
	"github.com/faceair/clash-speedtest/core/monitor"
	"net/http"
	"strings"
	"testing"
)

func TestReputationCatalogUsesSeparatePing0FamiliesWithoutIPOK(t *testing.T) {
	seen := map[string]bool{}
	for _, rule := range Catalog() {
		seen[rule.ServiceID] = true
		if strings.HasPrefix(rule.ServiceID, "ipok_") || rule.ServiceID == "ping0_ip_quality" {
			t.Fatalf("retired rule still selectable: %s", rule.ServiceID)
		}
	}
	for _, id := range []string{"ping0_ipv4", "ping0_ipv6", "ippure_ip_quality"} {
		if !seen[id] {
			t.Fatalf("missing %s", id)
		}
	}
	// Legacy snapshots remain readable without entering the current catalog.
	if _, ok := RuleFor("ping0_ip_quality"); !ok {
		t.Fatal("lost legacy rule")
	}
}

func TestPing0ValidatesRequestedAddressFamily(t *testing.T) {
	for _, tc := range []struct {
		id, ip, family string
		valid          bool
	}{
		{"ping0_ipv4", "8.8.8.8", "IPv4", true},
		{"ping0_ipv6", "2001:4860:4860::8888", "IPv6", true},
		{"ping0_ipv4", "2001:4860:4860::8888", "IPv4", false},
		{"ping0_ipv6", "8.8.8.8", "IPv6", false},
	} {
		t.Run(tc.id+tc.ip, func(t *testing.T) {
			rule, ok := RuleFor(tc.id)
			if !ok {
				t.Fatalf("missing %s", tc.id)
			}
			checker := Checker{ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
				host := "ipv4.ping0.cc"
				if tc.family == "IPv6" {
					host = "ipv6.ping0.cc"
				}
				if req.URL.Host != host || req.URL.Scheme != "https" {
					t.Fatalf("wrong family target: %s", req.URL)
				}
				return fixtureResponse(req, 200, "application/json", fmt.Sprintf(`{"ip":%q,"iprisk":0}`, tc.ip)), nil
			})}
			result := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
			if tc.valid {
				if result.Outcome != "profiled" || result.Details["risk_score"] != "0" || result.Details["ip_family"] != tc.family {
					t.Fatalf("missing family or zero: %+v", result)
				}
			} else if result.Outcome == "profiled" || result.FailurePhase != "ip_family" {
				t.Fatalf("wrong family accepted: %+v", result)
			}
		})
	}
}
