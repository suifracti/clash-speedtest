package monitor

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// fingerprintDomainSeparator is a fixed domain separator ensuring deterministic NodeKey derivation
// across application restarts. It is NOT a secret credential salt, but an architectural prefix
// to avoid hash collision with other digests across the system.
//
// TODO (Architectural Evolution):
// Currently, NodeKey conflates transport endpoint identity and credential configuration.
// In a future release, NodeIdentityKey (transport endpoint: type, server, port, network, sni, path)
// should be decoupled from ConfigRevisionKey (credentials, encryption params, headers) so that credential
// rotation or minor header tweaks will not orphan long-term time-series probe history.
const fingerprintDomainSeparator = "clash-speedtest-nodekey-v1"

// sanitizeServerTag produces a filesystem- and URL-safe server string slice of max 32 chars.
func sanitizeServerTag(cleanServer string) string {
	safeServer := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '-' {
			return r
		}
		return '_'
	}, cleanServer)

	if len(safeServer) > 32 {
		safeServer = safeServer[:32]
	}
	return safeServer
}

// ComputeNodeIdentityKey calculates a stable, long-term transport endpoint identifier for a proxy node.
// It incorporates purely transport and connection parameters:
// - Protocol Type (lowercased)
// - Server Hostname or IP (lowercased)
// - Server Port
// - Network / Transport Type (e.g. "ws", "grpc", "tcp")
// - Host / SNI / ServerName
// - Path (e.g. ws path / grpc serviceName)
//
// It strictly EXCLUDES credentials (passwords, tokens, UUIDs, private keys) and display names.
// This key enables long-term historical continuity even across credential rotations or name changes.
func ComputeNodeIdentityKey(nodeType, server string, port int, network, sni, path string) string {
	cleanType := strings.ToLower(strings.TrimSpace(nodeType))
	cleanServer := strings.ToLower(strings.TrimSpace(server))
	cleanNetwork := strings.ToLower(strings.TrimSpace(network))
	cleanSNI := strings.ToLower(strings.TrimSpace(sni))
	cleanPath := strings.TrimSpace(path)

	h := sha256.New()
	h.Write([]byte(fingerprintDomainSeparator))
	h.Write([]byte(":nid\n"))
	h.Write([]byte(cleanType))
	h.Write([]byte("\n"))
	h.Write([]byte(cleanServer))
	h.Write([]byte("\n"))
	h.Write([]byte(strconv.Itoa(port)))
	h.Write([]byte("\n"))
	h.Write([]byte(cleanNetwork))
	h.Write([]byte("\n"))
	h.Write([]byte(cleanSNI))
	h.Write([]byte("\n"))
	h.Write([]byte(cleanPath))

	digest := hex.EncodeToString(h.Sum(nil))
	safeServer := sanitizeServerTag(cleanServer)

	return fmt.Sprintf("nid_%s_%s_%d_%s", cleanType, safeServer, port, digest[:8])
}

// ComputeConfigRevisionKey hashes the credential parameters and connection options for a node,
// binding it to its NodeIdentityKey.
// When credentials rotate or TLS parameters change, ConfigRevisionKey changes,
// while NodeIdentityKey remains stable.
func ComputeConfigRevisionKey(identityKey string, rawConfig map[string]any) string {
	credHash := hashSensitiveCredentials(rawConfig)

	var extraFlags []string
	if rawConfig != nil {
		flagKeys := []string{"alpn", "skip-cert-verify", "client-fingerprint", "udp", "tls"}
		for _, k := range flagKeys {
			if val, exists := rawConfig[k]; exists && val != nil {
				extraFlags = append(extraFlags, fmt.Sprintf("%s=%v", k, val))
			}
		}
		sort.Strings(extraFlags)
	}

	h := sha256.New()
	h.Write([]byte(fingerprintDomainSeparator))
	h.Write([]byte(":rev\n"))
	h.Write([]byte(identityKey))
	h.Write([]byte("\n"))
	h.Write([]byte(credHash))
	h.Write([]byte("\n"))
	h.Write([]byte(strings.Join(extraFlags, ";")))

	digest := hex.EncodeToString(h.Sum(nil))
	return fmt.Sprintf("rev_%s", digest[:16])
}

// ComputeNodeKey calculates a backward-compatible composite identifier for a proxy node.
// It incorporates both transport endpoint parameters and credential digests.
func ComputeNodeKey(nodeType, server string, port int, network, sni, path string, rawConfig map[string]any) string {
	cleanType := strings.ToLower(strings.TrimSpace(nodeType))
	cleanServer := strings.ToLower(strings.TrimSpace(server))
	cleanNetwork := strings.ToLower(strings.TrimSpace(network))
	cleanSNI := strings.ToLower(strings.TrimSpace(sni))
	cleanPath := strings.TrimSpace(path)

	// Extract and hash sensitive credentials deterministically
	credHash := hashSensitiveCredentials(rawConfig)

	h := sha256.New()
	h.Write([]byte(fingerprintDomainSeparator))
	h.Write([]byte("\n"))
	h.Write([]byte(cleanType))
	h.Write([]byte("\n"))
	h.Write([]byte(cleanServer))
	h.Write([]byte("\n"))
	h.Write([]byte(strconv.Itoa(port)))
	h.Write([]byte("\n"))
	h.Write([]byte(cleanNetwork))
	h.Write([]byte("\n"))
	h.Write([]byte(cleanSNI))
	h.Write([]byte("\n"))
	h.Write([]byte(cleanPath))
	h.Write([]byte("\n"))
	h.Write([]byte(credHash))

	digest := hex.EncodeToString(h.Sum(nil))
	safeServer := sanitizeServerTag(cleanServer)

	return fmt.Sprintf("nk_%s_%s_%d_%s", cleanType, safeServer, port, digest[:8])
}

// ComputeNodeIdentityKeyFromConfig extracts transport parameters from raw config and computes NodeIdentityKey.
func ComputeNodeIdentityKeyFromConfig(cfg map[string]any) string {
	if cfg == nil {
		return "nid_unknown"
	}

	nodeType, _ := cfg["type"].(string)
	server, _ := cfg["server"].(string)
	port := extractPort(cfg["port"])

	network, _ := cfg["network"].(string)
	sni, _ := cfg["sni"].(string)
	if sni == "" {
		sni, _ = cfg["servername"].(string)
	}

	path := ""
	if wsOpts, ok := cfg["ws-opts"].(map[string]any); ok {
		path, _ = wsOpts["path"].(string)
	} else if grpcOpts, ok := cfg["grpc-opts"].(map[string]any); ok {
		path, _ = grpcOpts["grpc-service-name"].(string)
	}

	return ComputeNodeIdentityKey(nodeType, server, port, network, sni, path)
}

// ComputeNodeIdentityKeyFromNode computes the NodeIdentityKey for a MonitoredNode.
func ComputeNodeIdentityKeyFromNode(node MonitoredNode) string {
	if len(node.RawConfig) > 0 {
		return ComputeNodeIdentityKeyFromConfig(node.RawConfig)
	}
	return ComputeNodeIdentityKey(node.Type, node.Server, node.Port, "", "", "")
}

// ComputeConfigRevisionKeyFromNode computes the ConfigRevisionKey for a MonitoredNode.
func ComputeConfigRevisionKeyFromNode(identityKey string, node MonitoredNode) string {
	return ComputeConfigRevisionKey(identityKey, node.RawConfig)
}

// PopulateNodeKeys populates NodeKey, NodeIdentityKey, and ConfigRevisionKey on a MonitoredNode if empty.
func PopulateNodeKeys(node *MonitoredNode) {
	if node == nil {
		return
	}
	if node.NodeKey == "" {
		node.NodeKey = ComputeNodeKeyFromNode(*node)
	}
	if node.NodeIdentityKey == "" {
		node.NodeIdentityKey = ComputeNodeIdentityKeyFromNode(*node)
	}
	if node.ConfigRevisionKey == "" {
		node.ConfigRevisionKey = ComputeConfigRevisionKeyFromNode(node.NodeIdentityKey, *node)
	}
}

// PopulateNodesKeys populates NodeKey, NodeIdentityKey, and ConfigRevisionKey for a slice of MonitoredNode.
func PopulateNodesKeys(nodes []MonitoredNode) {
	for i := range nodes {
		PopulateNodeKeys(&nodes[i])
	}
}

// ComputeNodeKeyFromConfig parses raw proxy config map and produces its stable NodeKey.
func ComputeNodeKeyFromConfig(cfg map[string]any) string {
	if cfg == nil {
		return "nk_unknown"
	}

	nodeType, _ := cfg["type"].(string)
	server, _ := cfg["server"].(string)
	port := extractPort(cfg["port"])

	network, _ := cfg["network"].(string)
	sni, _ := cfg["sni"].(string)
	if sni == "" {
		sni, _ = cfg["servername"].(string)
	}

	path := ""
	if wsOpts, ok := cfg["ws-opts"].(map[string]any); ok {
		path, _ = wsOpts["path"].(string)
	} else if grpcOpts, ok := cfg["grpc-opts"].(map[string]any); ok {
		path, _ = grpcOpts["grpc-service-name"].(string)
	}

	return ComputeNodeKey(nodeType, server, port, network, sni, path, cfg)
}

// ComputeNodeKeyFromNode computes the stable NodeKey for a MonitoredNode.
func ComputeNodeKeyFromNode(node MonitoredNode) string {
	if len(node.RawConfig) > 0 {
		return ComputeNodeKeyFromConfig(node.RawConfig)
	}
	return ComputeNodeKey(node.Type, node.Server, node.Port, "", "", "", nil)
}

func extractPort(raw any) int {
	switch v := raw.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		if p, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return p
		}
	}
	return 0
}

// hashSensitiveCredentials collects credential fields from raw config and produces
// a single-way cryptographic digest so different users on the same server have distinct keys
// without exposing plaintext secrets.
func hashSensitiveCredentials(rawConfig map[string]any) string {
	if rawConfig == nil {
		return ""
	}

	sensitiveKeys := []string{"uuid", "password", "token", "private-key", "auth-str", "psk"}
	var foundCreds []string

	for _, k := range sensitiveKeys {
		if val, exists := rawConfig[k]; exists && val != nil {
			strVal := fmt.Sprintf("%v", val)
			if strVal != "" {
				// SHA-256 of the individual credential with domain separator
				h := sha256.Sum256([]byte(fingerprintDomainSeparator + ":" + k + ":" + strVal))
				foundCreds = append(foundCreds, k+"="+hex.EncodeToString(h[:16]))
			}
		}
	}

	if len(foundCreds) == 0 {
		return ""
	}

	sort.Strings(foundCreds)
	combined := strings.Join(foundCreds, ";")
	h := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(h[:])
}

// SanitizeNodeConfig returns a shallow copy of node config with all sensitive credential fields masked.
func SanitizeNodeConfig(rawConfig map[string]any) map[string]any {
	if rawConfig == nil {
		return nil
	}

	clean := make(map[string]any, len(rawConfig))
	sensitiveKeys := map[string]bool{
		"uuid":        true,
		"password":    true,
		"token":       true,
		"private-key": true,
		"auth-str":    true,
		"psk":         true,
	}

	for k, v := range rawConfig {
		if sensitiveKeys[strings.ToLower(k)] {
			clean[k] = "******"
		} else {
			clean[k] = v
		}
	}
	return clean
}
