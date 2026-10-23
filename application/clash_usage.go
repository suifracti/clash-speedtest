package application

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/faceair/clash-speedtest/core/controller"
)

type ClashUsage struct {
	Name   string   `json:"name"`
	Bytes  int64    `json:"bytes"`
	Chains []string `json:"chains"`
}

// This is a live connection snapshot, not browsing history. Only outbound
// names, policy chains and byte counters survive the controller decoder.
func (s *AppService) GetClashUsage(ctx context.Context) []ClashUsage {
	result := []ClashUsage{}
	client, _, err := s.clashClient()
	if err != nil {
		return result
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	connections, err := client.ReadConnectionTraffic(ctx)
	if err != nil {
		return result
	}
	nodes := map[string]*ClashUsage{}
	chainBytes := map[string]map[string]int64{}
	for _, c := range connections {
		if len(c.Chains) == 0 {
			continue
		}
		name := c.Chains[0]
		switch name {
		case "DIRECT", "REJECT", "REJECT-DROP", "PASS", "PASS-RULE", "COMPATIBLE":
			continue
		}
		bytes := c.Upload + c.Download
		if bytes <= 0 {
			continue
		}
		if nodes[name] == nil {
			nodes[name] = &ClashUsage{Name: name, Chains: []string{}}
			chainBytes[name] = map[string]int64{}
		}
		nodes[name].Bytes += bytes
		path := append([]string(nil), c.Chains[1:]...)
		for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
			path[i], path[j] = path[j], path[i]
		}
		chainBytes[name][strings.Join(path, "\x00")] += bytes
	}
	for name, n := range nodes {
		var max int64
		best := ""
		for path, bytes := range chainBytes[name] {
			if bytes > max || bytes == max && path < best {
				max = bytes
				best = path
			}
		}
		if best != "" {
			n.Chains = strings.Split(best, "\x00")
		}
		result = append(result, *n)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Bytes == result[j].Bytes {
			return result[i].Name < result[j].Name
		}
		return result[i].Bytes > result[j].Bytes
	})
	return result
}

type ClashSelectionStep struct {
	Group  string `json:"group"`
	Before string `json:"before"`
	After  string `json:"after"`
}
type ClashGroupOption struct {
	Group string               `json:"group"`
	Path  []string             `json:"path"`
	Steps []ClashSelectionStep `json:"steps"`
}
type ClashSelectRequest struct {
	WorkbenchLatencyBatchSelection
	Group string `json:"group"`
}
type ClashSelectResult struct {
	Verified bool                 `json:"verified"`
	Steps    []ClashSelectionStep `json:"steps"`
}

// Prefer a direct membership. Nested paths only traverse Selector groups:
// selecting a URLTest/Fallback member would not pin the requested node.
func clashGroupPath(groups map[string]controller.Group, group, node string, seen map[string]bool) []string {
	g, ok := groups[group]
	if !ok || !strings.EqualFold(g.Type, "Selector") || seen[group] {
		return nil
	}
	seen[group] = true
	defer delete(seen, group)
	for _, name := range g.All {
		if name == node {
			return []string{group, node}
		}
	}
	for _, name := range g.All {
		if path := clashGroupPath(groups, name, node, seen); len(path) > 0 {
			return append([]string{group}, path...)
		}
	}
	return nil
}
func clashGroupOptions(list []controller.Group, node string) []ClashGroupOption {
	groups := map[string]controller.Group{}
	for _, g := range list {
		groups[g.Name] = g
	}
	options := []ClashGroupOption{}
	for _, g := range list {
		path := clashGroupPath(groups, g.Name, node, map[string]bool{})
		if len(path) < 2 {
			continue
		}
		option := ClashGroupOption{Group: g.Name, Path: path, Steps: []ClashSelectionStep{}}
		for i := 0; i < len(path)-1; i++ {
			option.Steps = append(option.Steps, ClashSelectionStep{Group: path[i], Before: groups[path[i]].Now, After: path[i+1]})
		}
		options = append(options, option)
	}
	sort.Slice(options, func(i, j int) bool { return options[i].Group < options[j].Group })
	return options
}
func (s *AppService) GetClashNodeGroups(ctx context.Context, scope WorkbenchLatencyBatchSelection) ([]ClashGroupOption, error) {
	_, name, proxy, err := s.resolveWorkbenchScopedProxy(scope.ProfileID, scope.NodeKey, scope.NodeIdentityKey, scope.ConfigRevisionKey)
	if err != nil {
		return nil, err
	}
	defer closeWorkbenchDownloadProxy(proxy)
	client, _, err := s.clashClient()
	if err != nil {
		return nil, err
	}
	defer client.Close()
	groups, err := client.ListGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("无法读取 Clash 代理组")
	}
	return clashGroupOptions(groups, name), nil
}
func (s *AppService) UseClashNode(ctx context.Context, req ClashSelectRequest) (*ClashSelectResult, error) {
	_, name, proxy, err := s.resolveWorkbenchScopedProxy(req.ProfileID, req.NodeKey, req.NodeIdentityKey, req.ConfigRevisionKey)
	if err != nil {
		return nil, err
	}
	defer closeWorkbenchDownloadProxy(proxy)
	client, _, err := s.clashClient()
	if err != nil {
		return nil, err
	}
	defer client.Close()
	groups, err := client.ListGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("无法读取 Clash 代理组")
	}
	var chosen *ClashGroupOption
	for _, option := range clashGroupOptions(groups, name) {
		if option.Group == req.Group {
			chosen = &option
			break
		}
	}
	if chosen == nil {
		return nil, fmt.Errorf("这个代理组不包含所选节点，或不支持手动选择")
	}
	// Set inner groups first so the root points to the requested path last.
	for i := len(chosen.Steps) - 1; i >= 0; i-- {
		step := chosen.Steps[i]
		if step.Before == step.After {
			continue
		}
		if err := client.SelectNode(ctx, step.Group, step.After); err != nil {
			return nil, fmt.Errorf("切换 %s 失败：%w", step.Group, err)
		}
	}
	for _, step := range chosen.Steps {
		current, err := client.GetCurrentSelection(ctx, step.Group)
		if err != nil || current != step.After {
			return nil, fmt.Errorf("未能确认 %s 的选择，请在 Clash 中查看", step.Group)
		}
	}
	return &ClashSelectResult{Verified: true, Steps: chosen.Steps}, nil
}
