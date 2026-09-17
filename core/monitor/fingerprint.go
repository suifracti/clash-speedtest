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

// ComputeNodeKey calculates a stable, deterministic identifier for a proxy node.
// It incorporates non-mutable transport and connection parameters:
// - Protocol Type (lowercased)
// - Server Hostname or IP (lowercased)
// - Server Port
// - Network / Transport Type (e.g. "ws", "grpc", "tcp")
// - Host / SNI / ServerName
// - Path (e.g. ws path / grpc serviceName)
//
// Sensitive Information Handling:
// Sensitive fields (passwords, UUIDs, secret tokens, private keys) are hashed using
// SHA-256 with the domain separator before entering the fingerprint calculation.
// Plaintext secrets are NEVER included in the NodeKey string, logs, or SQLite indexes.
// Renaming a node's display name does NOT alter its NodeKey.
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

	// URL and filesystem safe server tag
	safeServer := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '-' {
			return r
		}
		return '_'
	}, cleanServer)

	if len(safeServer) > 32 {
		safeServer = safeServer[:32]
	}

	return fmt.Sprintf("nk_%s_%s_%d_%s", cleanType, safeServer, port, digest[:8])
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
