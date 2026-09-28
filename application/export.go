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
	NodeKeys  []string `json:"node_keys"`
	GroupName string   `json:"group_name,omitempty"`
}

type ExportClashConfigResponse struct {
	YAMLContent string   `json:"yaml_content"`
	NodeCount   int      `json:"node_count"`
	NodeNames   []string `json:"node_names"`
}

// ExportNodesClashConfig extracts the raw proxy configurations for the requested nodeKeys
// and generates a complete, valid Clash/Mihomo YAML configuration file.
func (s *AppService) ExportNodesClashConfig(_ context.Context, req ExportClashConfigRequest) (*ExportClashConfigResponse, error) {
	if len(req.NodeKeys) == 0 {
		return nil, monitor.NewValidationError("请先勾选或指定至少一个要导出的节点")
	}

	requestedSet := make(map[string]struct{}, len(req.NodeKeys))
	for _, k := range req.NodeKeys {
		k = strings.TrimSpace(k)
		if k != "" {
			requestedSet[k] = struct{}{}
		}
	}
	if len(requestedSet) == 0 {
		return nil, monitor.NewValidationError("节点标识不能为空")
	}

	allNodes, err := s.collectAllCachedNodes()
	if err != nil {
		return nil, fmt.Errorf("读取本地缓存节点失败: %w", err)
	}

	// Index nodes by NodeKey
	nodeByKey := make(map[string]monitor.MonitoredNode, len(allNodes))
	for _, node := range allNodes {
		if node.NodeKey != "" {
			if _, exists := nodeByKey[node.NodeKey]; !exists {
				nodeByKey[node.NodeKey] = node
			}
		}
	}

	// Preserve order of requested keys
	proxies := make([]map[string]any, 0, len(req.NodeKeys))
	nodeNames := make([]string, 0, len(req.NodeKeys))
	nameSeen := make(map[string]int)

	for _, key := range req.NodeKeys {
		node, found := nodeByKey[key]
		if !found || len(node.RawConfig) == 0 {
			continue
		}

		// Deep copy raw config
		cfg := make(map[string]any, len(node.RawConfig)+1)
		for k, v := range node.RawConfig {
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
		if len(airport.Subscriptions) > 0 {
			for _, sub := range airport.Subscriptions {
				if sub == nil {
					continue
				}
				nodes, err := s.loadMonitorNodes(sub.ID)
				if err != nil {
					nodes, err = s.loadMonitorNodes(airport.ID)
					if err != nil {
						continue
					}
				}
				allNodes = append(allNodes, nodes...)
			}
		} else {
			nodes, err := s.loadMonitorNodes(airport.ID)
			if err != nil {
				continue
			}
			allNodes = append(allNodes, nodes...)
		}
	}
	return allNodes, nil
}
