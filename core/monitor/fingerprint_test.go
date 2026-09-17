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

func TestComputeNodeIdentityKey_DecoupledFromCredentials(t *testing.T) {
	secret1 := "super-password-alpha"
	secret2 := "super-password-beta-updated"

	cfgOriginal := map[string]any{
		"name":     "Tokyo-01",
		"type":     "vmess",
		"server":   "tokyo.node.com",
		"port":     443,
		"uuid":     secret1,
		"network":  "ws",
		"sni":      "cdn.node.com",
		"ws-opts": map[string]any{
			"path": "/ws",
		},
	}

	// 1. Password changed, transport intact
	cfgCredUpdated := map[string]any{
		"name":     "Tokyo-01",
		"type":     "vmess",
		"server":   "tokyo.node.com",
		"port":     443,
		"uuid":     secret2, // Changed!
		"network":  "ws",
		"sni":      "cdn.node.com",
		"ws-opts": map[string]any{
			"path": "/ws",
		},
	}

	// 2. Display name changed, credentials and transport intact
	cfgNameChanged := map[string]any{
		"name":     "⚡ Tokyo Premium Line", // Changed!
		"type":     "vmess",
		"server":   "tokyo.node.com",
		"port":     443,
		"uuid":     secret1,
		"network":  "ws",
		"sni":      "cdn.node.com",
		"ws-opts": map[string]any{
			"path": "/ws",
		},
	}

	// 3. Transport endpoint changed
	cfgServerChanged := map[string]any{
		"name":     "Tokyo-01",
		"type":     "vmess",
		"server":   "osaka.node.com", // Changed!
		"port":     443,
		"uuid":     secret1,
		"network":  "ws",
		"sni":      "cdn.node.com",
		"ws-opts": map[string]any{
			"path": "/ws",
		},
	}

	nidOrig := ComputeNodeIdentityKeyFromConfig(cfgOriginal)
	revOrig := ComputeConfigRevisionKey(nidOrig, cfgOriginal)

	nidCred := ComputeNodeIdentityKeyFromConfig(cfgCredUpdated)
	revCred := ComputeConfigRevisionKey(nidCred, cfgCredUpdated)

	nidName := ComputeNodeIdentityKeyFromConfig(cfgNameChanged)
	revName := ComputeConfigRevisionKey(nidName, cfgNameChanged)

	nidServer := ComputeNodeIdentityKeyFromConfig(cfgServerChanged)

	// Invariant 1: Credential update must preserve NodeIdentityKey, but change ConfigRevisionKey
	if nidOrig != nidCred {
		t.Errorf("NodeIdentityKey must be decoupled from credentials: orig=%q, cred=%q", nidOrig, nidCred)
	}
	if revOrig == revCred {
		t.Errorf("ConfigRevisionKey must change when credential is changed: revOrig=%q, revCred=%q", revOrig, revCred)
	}

	// Invariant 2: Name change must preserve both NodeIdentityKey and ConfigRevisionKey
	if nidOrig != nidName {
		t.Errorf("NodeIdentityKey must be immune to display name changes: orig=%q, name=%q", nidOrig, nidName)
	}
	if revOrig != revName {
		t.Errorf("ConfigRevisionKey must be immune to display name changes: revOrig=%q, revName=%q", revOrig, revName)
	}

	// Invariant 3: Server change must produce a distinct NodeIdentityKey
	if nidOrig == nidServer {
		t.Errorf("different server must produce different NodeIdentityKey: orig=%q, server=%q", nidOrig, nidServer)
	}

	// Invariant 4: No plaintext secret in NodeIdentityKey or ConfigRevisionKey
	for _, k := range []string{nidOrig, nidCred, revOrig, revCred} {
		if strings.Contains(k, secret1) || strings.Contains(k, secret2) {
			t.Fatalf("SECURITY VIOLATION: plaintext secret leaked in key: %s", k)
		}
	}
}

func TestPopulateNodesKeys(t *testing.T) {
	nodes := []MonitoredNode{
		{
			Type:   "ss",
			Server: "1.2.3.4",
			Port:   8388,
			RawConfig: map[string]any{
				"type":     "ss",
				"server":   "1.2.3.4",
				"port":     8388,
				"password": "secret-pass-word",
			},
		},
	}

	PopulateNodesKeys(nodes)

	if nodes[0].NodeKey == "" {
		t.Errorf("expected NodeKey populated")
	}
	if nodes[0].NodeIdentityKey == "" {
		t.Errorf("expected NodeIdentityKey populated")
	}
	if nodes[0].ConfigRevisionKey == "" {
		t.Errorf("expected ConfigRevisionKey populated")
	}
}

