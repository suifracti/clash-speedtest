package publicservice

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/faceair/clash-speedtest/core/monitor"
)

func TestIPQueryProbeParsesScoreAndFlags(t *testing.T) {
	rule, ok := RuleFor("ipquery_ip_quality")
	if !ok {
		t.Fatal("expected ipquery_ip_quality in RuleFor")
	}
	t.Run("datacenter ipv4", func(t *testing.T) {
		checker := Checker{DriftSamples: 1, ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
			return fixtureResponse(req, 200, "application/json", `{
				"ip":"203.0.113.10",
				"isp":{"asn":"AS16509","org":"Amazon.com","isp":"Amazon AWS"},
				"location":{"country":"Japan","country_code":"JP","city":"Tokyo"},
				"risk":{"is_mobile":false,"is_vpn":true,"is_tor":false,"is_proxy":false,"is_datacenter":true,"risk_score":42}
			}`), nil
		})}
		res := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
		if res.Outcome != "profiled" || res.Details["risk_score"] != "42" || res.Details["ip_type"] != "机房 IDC" || res.Details["country_code"] != "JP" || res.Details["provider"] != "IPQuery" || res.Details["ip_family"] != "IPv4" {
			t.Fatalf("unexpected ipquery result: %+v", res)
		}
	})
	t.Run("rejects out of range risk score", func(t *testing.T) {
		checker := Checker{DriftSamples: 1, ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
			return fixtureResponse(req, 200, "application/json", `{"ip":"203.0.113.10","risk":{"risk_score":150}}`), nil
		})}
		res := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
		if res.Outcome != "criteria_mismatch" || res.FailurePhase != "risk_score" {
			t.Fatalf("expected criteria_mismatch, got %+v", res)
		}
	})
}

func TestIPLogsProbeNormalizesScoreAndClassifies(t *testing.T) {
	rule, ok := RuleFor("iplogs_ip_quality")
	if !ok {
		t.Fatal("expected iplogs_ip_quality in RuleFor")
	}
	t.Run("hosting vpn normalized 0..1 to 0..100", func(t *testing.T) {
		checker := Checker{DriftSamples: 1, ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", req.Method)
			}
			return fixtureResponse(req, 200, "application/json", `{
				"score":0.85,
				"verdict":"vpn_detected",
				"reputation":"greylist",
				"is_vpn":true,
				"is_proxy":true,
				"is_hosting":true,
				"proxy_type":"datacenter",
				"ip_info":{"ip":"198.51.100.7","asn":"AS9009","org":"M247","country":"United States","country_code":"US","city":"Phoenix","type":"datacenter"}
			}`), nil
		})}
		res := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
		if res.Outcome != "profiled" || res.Details["risk_score"] != "85" || res.Details["ip_type"] != "机房 IDC" || res.Details["country_code"] != "US" || res.Details["provider"] != "IPLogs" || !strings.Contains(res.Details["risk_tier"], "vpn_detected") {
			t.Fatalf("unexpected iplogs result: %+v", res)
		}
	})
	t.Run("clean residential", func(t *testing.T) {
		checker := Checker{DriftSamples: 1, ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
			return fixtureResponse(req, 200, "application/json", `{
				"score":0.05,
				"verdict":"clean",
				"is_hosting":false,
				"ip_info":{"ip":"2001:db8::1","asn":"12345","org":"Home ISP","country":"Japan","country_code":"JP","type":"residential"}
			}`), nil
		})}
		res := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
		if res.Outcome != "profiled" || res.Details["risk_score"] != "5" || res.Details["ip_type"] != "住宅（第三方判断）" || res.Details["ip_family"] != "IPv6" || res.Details["asn"] != "AS12345" {
			t.Fatalf("unexpected residential iplogs result: %+v", res)
		}
	})
	t.Run("missing ip rejected", func(t *testing.T) {
		checker := Checker{DriftSamples: 1, ClientFactory: fixtureClient(func(req *http.Request) (*http.Response, error) {
			return fixtureResponse(req, 200, "application/json", `{"score":0.2}`), nil
		})}
		res := checker.Check(context.Background(), monitor.MonitoredNode{}, rule, DefaultTimeout)
		if res.Outcome != "criteria_mismatch" {
			t.Fatalf("expected criteria_mismatch, got %+v", res)
		}
	})
}
