package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/profiles"
	"gopkg.in/yaml.v3"
)

type ExportClashConfigRequest struct {
	NodeKeys   []string                         `json:"node_keys,omitempty"`
	Selections []WorkbenchLatencyBatchSelection `json:"selections,omitempty"`
	GroupName  string                           `json:"group_name,omitempty"`
}

type ExportClashConfigResponse struct {
	YAMLContent string   `json:"yaml_content"`
	NodeCount   int      `json:"node_count"`
	NodeNames   []string `json:"node_names"`
}

// ExportNodesClashConfig extracts the raw proxy configurations for the requested nodeKeys
// and generates a complete, valid Clash/Mihomo YAML configuration file.
func (s *AppService) ExportNodesClashConfig(_ context.Context, req ExportClashConfigRequest) (*ExportClashConfigResponse, error) {
	if len(req.NodeKeys) == 0 && len(req.Selections) == 0 {
		return nil, monitor.NewValidationError("请先勾选或指定至少一个要导出的节点")
	}
	if len(req.NodeKeys) > 0 && len(req.Selections) > 0 {
		return nil, monitor.NewValidationError("导出请求不能混合旧 node_keys 与完整作用域")
	}
	selected := make([]monitor.MonitoredNode, 0, len(req.Selections)+len(req.NodeKeys))
	seen := map[string]bool{}
	if len(req.Selections) > 0 {
		for _, selection := range req.Selections {
			if strings.TrimSpace(selection.ProfileID) == "" || strings.TrimSpace(selection.NodeKey) == "" || strings.TrimSpace(selection.NodeIdentityKey) == "" || strings.TrimSpace(selection.ConfigRevisionKey) == "" {
				return nil, monitor.NewValidationError("导出必须包含完整 profile/node/identity/revision")
			}
			node, _, _, err := s.resolveWorkbenchScopedProxy(selection.ProfileID, selection.NodeKey, selection.NodeIdentityKey, selection.ConfigRevisionKey)
			if err != nil {
				return nil, err
			}
			id := selection.ProfileID + "\x00" + node.NodeKey + "\x00" + node.NodeIdentityKey + "\x00" + node.ConfigRevisionKey
			if !seen[id] {
				selected = append(selected, node)
				seen[id] = true
			}
		}
	} else {
		// Legacy clients may export only an unambiguous key. Never select the
		// first airport/revision and never silently omit a stale selection.
		allNodes, err := s.collectAllCachedNodes()
		if err != nil {
			return nil, fmt.Errorf("读取本地缓存节点失败: %w", err)
		}
		for _, rawKey := range req.NodeKeys {
			key := strings.TrimSpace(rawKey)
			if key == "" {
				return nil, monitor.NewValidationError("节点标识不能为空")
			}
			candidates := []monitor.MonitoredNode{}
			for _, n := range allNodes {
				if n.NodeKey == key {
					candidates = append(candidates, n)
				}
			}
			if len(candidates) != 1 {
				return nil, monitor.NewValidationError("旧节点标识无法唯一匹配，请使用完整作用域重新选择")
			}
			for _, n := range candidates {
				if !seen[key] {
					selected = append(selected, n)
					seen[key] = true
				}
			}
		}
	}

	// Preserve order of the exact frozen selections
	proxies := make([]map[string]any, 0, len(selected))
	nodeNames := make([]string, 0, len(selected))
	nameSeen := make(map[string]int)

	for _, node := range selected {
		if len(node.RawConfig) == 0 {
			return nil, monitor.NewValidationError("所选节点缺少有效原始配置，未导出任何文件")
		}

		if monitor.HasSourceTLSConstraints(node.RawConfig) {
			return nil, monitor.NewValidationError("普通 Clash/Mihomo 导出无法保留 pcs/vcn 证书约束；未导出任何文件")
		}

		// Deep copy raw config, omitting internal CST runtime markers
		cfg := make(map[string]any, len(node.RawConfig)+1)
		for k, v := range node.RawConfig {
			if strings.HasPrefix(k, "_cst_") {
				continue
			}
			cfg[k] = v
		}

		displayName := strings.TrimSpace(node.DisplayName)
		if displayName == "" {
			displayName = "Node"
		}

		// Ensure unique name in clash config
		nameSeen[displayName]++
		if count := nameSeen[displayName]; count > 1 {
			displayName = fmt.Sprintf("%s (%d)", displayName, count)
		}
		cfg["name"] = displayName

		proxies = append(proxies, cfg)
		nodeNames = append(nodeNames, displayName)
	}

	if len(proxies) == 0 {
		return nil, monitor.NewValidationError("未能匹配到有效节点配置，请确保所选节点的缓存文件存在")
	}

	groupName := strings.TrimSpace(req.GroupName)
	if groupName == "" {
		groupName = "PROXY"
	}

	clashConfig := map[string]any{
		"port":                7890,
		"socks-port":          7891,
		"allow-lan":           false,
		"mode":                "rule",
		"log-level":           "info",
		"external-controller": "127.0.0.1:9090",
		"proxies":             proxies,
		"proxy-groups": []map[string]any{
			{
				"name":    groupName,
				"type":    "select",
				"proxies": append([]string{"AUTO", "FALLBACK"}, nodeNames...),
			},
			{
				"name":      "AUTO",
				"type":      "url-test",
				"url":       "http://www.gstatic.com/generate_204",
				"interval":  300,
				"tolerance": 50,
				"proxies":   nodeNames,
			},
			{
				"name":     "FALLBACK",
				"type":     "fallback",
				"url":      "http://www.gstatic.com/generate_204",
				"interval": 300,
				"proxies":  nodeNames,
			},
		},
		"rules": []string{
			"MATCH," + groupName,
		},
	}

	yamlBytes, err := yaml.Marshal(clashConfig)
	if err != nil {
		return nil, fmt.Errorf("生成 Clash YAML 失败: %w", err)
	}

	return &ExportClashConfigResponse{
		YAMLContent: string(yamlBytes),
		NodeCount:   len(proxies),
		NodeNames:   nodeNames,
	}, nil
}

func (s *AppService) collectAllCachedNodes() ([]monitor.MonitoredNode, error) {
	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return nil, err
	}

	var allNodes []monitor.MonitoredNode
	for _, airport := range store.Airports {
		if airport == nil {
			continue
		}
		seenInAirport := make(map[string]bool)
		appendAirportNodes := func(nodes []monitor.MonitoredNode) {
			for _, n := range nodes {
				key := n.NodeKey + "\x00" + n.NodeIdentityKey + "\x00" + n.ConfigRevisionKey
				if !seenInAirport[key] {
					seenInAirport[key] = true
					allNodes = append(allNodes, n)
				}
			}
		}

		loadedAnySub := false
		if len(airport.Subscriptions) > 0 {
			for _, sub := range airport.Subscriptions {
				if sub == nil {
					continue
				}
				nodes, err := s.loadMonitorNodes(sub.ID)
				if err == nil && len(nodes) > 0 {
					loadedAnySub = true
					appendAirportNodes(nodes)
				}
			}
		}

		if !loadedAnySub {
			nodes, err := s.loadMonitorNodes(airport.ID)
			if err == nil && len(nodes) > 0 {
				appendAirportNodes(nodes)
			}
		}
	}
	return allNodes, nil
}
