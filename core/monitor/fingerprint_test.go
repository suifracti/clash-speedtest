package monitor

import (
	"strings"
	"testing"
)

func TestComputeNodeKey_StabilityAgainstNameChanges(t *testing.T) {
	cfg1 := map[string]any{
		"name":     "🇭🇰 香港 01 [x1.0]",
		"type":     "vmess",
		"server":   "hk01.example.com",
		"port":     443,
		"uuid":     "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		"network":  "ws",
		"sni":      "cdn.example.com",
		"ws-opts": map[string]any{
			"path": "/v2ray",
		},
	}

	cfg2 := map[string]any{
		"name":     "⚡ [VIP超快] 🇭🇰 香港 01 - 晚高峰优化", // Completely different name!
		"type":     "vmess",
		"server":   "hk01.example.com",
		"port":     443,
		"uuid":     "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		"network":  "ws",
		"sni":      "cdn.example.com",
		"ws-opts": map[string]any{
			"path": "/v2ray",
		},
	}

	key1 := ComputeNodeKeyFromConfig(cfg1)
	key2 := ComputeNodeKeyFromConfig(cfg2)

	if key1 == "" {
		t.Fatalf("expected non-empty NodeKey")
	}
	if key1 != key2 {
		t.Errorf("NodeKey must be immune to name changes: key1=%q, key2=%q", key1, key2)
	}

	// Must begin with nk_vmess_hk01.example.com_443_
	if !strings.HasPrefix(key1, "nk_vmess_hk01.example.com_443_") {
		t.Errorf("unexpected key format: %q", key1)
	}
}

func TestComputeNodeKey_SensitiveInformationProtection(t *testing.T) {
	plainSecret := "super-secret-password-12345"
	plainUUID := "b6732386-8809-411a-8260-eb05e83ec5e9"

	cfg := map[string]any{
		"name":     "SG-Node",
		"type":     "ss",
		"server":   "sg.example.com",
		"port":     8388,
		"password": plainSecret,
		"uuid":     plainUUID,
	}

	key := ComputeNodeKeyFromConfig(cfg)

	// Neither the plain password nor the plain UUID should EVER appear in the NodeKey
	if strings.Contains(key, plainSecret) {
		t.Errorf("SECURITY LEAK: plaintext password found in NodeKey: %q", key)
	}
	if strings.Contains(key, plainUUID) {
		t.Errorf("SECURITY LEAK: plaintext UUID found in NodeKey: %q", key)
	}

	// SanitizeNodeConfig check
	sanitized := SanitizeNodeConfig(cfg)
	if sanitized["password"] != "******" {
		t.Errorf("expected masked password, got %v", sanitized["password"])
	}
	if sanitized["uuid"] != "******" {
		t.Errorf("expected masked uuid, got %v", sanitized["uuid"])
	}
	if sanitized["server"] != "sg.example.com" {
		t.Errorf("expected server preserved, got %v", sanitized["server"])
	}
}

func TestComputeNodeKey_DifferentEndpoints(t *testing.T) {
	cfgA := map[string]any{
		"type":   "trojan",
		"server": "trojan.node.com",
		"port":   443,
	}
	cfgB := map[string]any{
		"type":   "trojan",
		"server": "trojan.node.com",
		"port":   8443, // different port
	}
	cfgC := map[string]any{
		"type":   "ss", // different protocol
		"server": "trojan.node.com",
		"port":   443,
	}

	keyA := ComputeNodeKeyFromConfig(cfgA)
	keyB := ComputeNodeKeyFromConfig(cfgB)
	keyC := ComputeNodeKeyFromConfig(cfgC)

	if keyA == keyB || keyA == keyC || keyB == keyC {
		t.Errorf("distinct endpoints must produce distinct keys: A=%q, B=%q, C=%q", keyA, keyB, keyC)
	}
}
