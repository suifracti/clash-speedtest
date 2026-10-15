package speedtester

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/metacubex/mihomo/common/convert"
)

func TestParseHysteria2PortsWithImplicit443(t *testing.T) {
	for _, tc := range []struct {
		name, link, wantPorts string
		base64                bool
	}{
		{"hy2_mport", "hy2://fixture@node.invalid/?mport=60000-60001#same", "60000-60001", false},
		{"hysteria2_ports_base64", "hysteria2://fixture@node.invalid/?ports=61000-61001#same", "61000-61001", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.link
			if tc.base64 {
				body = base64.StdEncoding.EncodeToString([]byte(body))
			}
			cfg, err := parseConfigBody([]byte(body))
			if err != nil || len(cfg.Proxies) != 1 {
				t.Fatalf("synthetic link parse failed: err=%v", err)
			}
			if p := cfg.Proxies[0]; p["port"] != "443" || p["ports"] != tc.wantPorts {
				t.Fatalf("implicit 443 lost port range: port=%v ports=%v want=%s", p["port"], p["ports"], tc.wantPorts)
			}
		})
	}
}

func TestHysteria2PortsMatchFullConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, firstAuth, secondAuth, firstQuery, secondQuery string
	}{
		{"credential", "alpha", "bravo", "", ""},
		{"credential_password", "alpha:one", "alpha:two", "", ""},
		{"sni", "fixture", "fixture", "sni=first.invalid&", "sni=second.invalid&"},
		{"certificate_policy", "fixture", "fixture", "insecure=false&", "insecure=true&"},
		{"certificate_pin", "fixture", "fixture", "pinSHA256=" + strings.Repeat("a", 64) + "&", "pinSHA256=" + strings.Repeat("b", 64) + "&"},
		{"alpn", "fixture", "fixture", "alpn=h3&", "alpn=h3,h2&"},
		{"obfuscation", "fixture", "fixture", "", "obfs=salamander&obfs-password=fixture&"},
		{"obfuscation_password", "fixture", "fixture", "obfs=salamander&obfs-password=one&", "obfs=salamander&obfs-password=two&"},
		{"upload_rate", "fixture", "fixture", "up=10&", "up=20&"},
		{"download_rate", "fixture", "fixture", "down=10&", "down=20&"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(fmt.Sprintf("hy2://%s@node.invalid:443/?%smport=60000-60001#same\nhy2://%s@node.invalid:443/?%smport=61000-61001#same", tc.firstAuth, tc.firstQuery, tc.secondAuth, tc.secondQuery))
			proxies, err := convert.ConvertsV2Ray(body)
			if err != nil || len(proxies) != 2 {
				t.Fatalf("synthetic link conversion failed: err=%v", err)
			}
			first, second := proxies[0], proxies[1]
			// Names and output position must not substitute for connection identity.
			first["name"], second["name"] = "same", "same"
			enrichHysteria2Ports([]map[string]any{second, first}, body)
			if first["ports"] != "60000-60001" || second["ports"] != "61000-61001" {
				t.Fatalf("port ranges crossed configurations: first=%v second=%v", first["ports"], second["ports"])
			}
		})
	}
}

func TestParseHysteria2PortRangesStayWithIdenticalSourceOccurrences(t *testing.T) {
	body := []byte("hy2://fixture@node.invalid:443/#same\nhy2://fixture@node.invalid:443/?mport=60000-60001#same\nhy2://fixture@node.invalid:443/?ports=61000-61001#same")
	cfg, err := parseConfigBody(body)
	if err != nil || len(cfg.Proxies) != 3 {
		t.Fatalf("synthetic link parse failed: err=%v", err)
	}
	want := []any{nil, "60000-60001", "61000-61001"}
	for i, p := range cfg.Proxies {
		if p["ports"] != want[i] {
			t.Errorf("source occurrence %d received another range: got=%v want=%v", i, p["ports"], want[i])
		}
	}
}
