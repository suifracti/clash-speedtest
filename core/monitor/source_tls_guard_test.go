package monitor

import (
	"strings"
	"testing"
	"time"
)

func TestLegacyDialerDoesNotIgnoreSourceCertificatePolicy(t *testing.T) {
	node := MonitoredNode{NodeKey: "fixture", RawConfig: map[string]any{"name": "fixture", "type": "vless", "server": "tls.invalid", "port": 443, "uuid": "11111111-1111-1111-1111-111111111111", "tls": true, "skip-cert-verify": true, "_cst_tls_pins": []string{strings.Repeat("a", 64)}}}
	client, err := NewDefaultNodeDialer().CreateClient(node, time.Second)
	if err == nil || client != nil {
		t.Fatal("legacy client ignored source certificate policy")
	}
}
