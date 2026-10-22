package application

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/faceair/clash-speedtest/core/entry"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/profiles"
	"github.com/faceair/clash-speedtest/core/speedtester"
)

const (
	entryResolveTimeout = 8 * time.Second
	entryResolveWorkers = 8
)

// WorkbenchEntryScope identifies one node whose entry should be resolved.
type WorkbenchEntryScope struct {
	ProfileID         string `json:"profile_id"`
	NodeKey           string `json:"node_key"`
	NodeIdentityKey   string `json:"node_identity_key"`
	ConfigRevisionKey string `json:"config_revision_key"`
}

// WorkbenchEntryRequest asks for the entry chain of a set of nodes. Refresh
// resolves hosts and queries addresses; without it the call is cache-only and
// never touches the network, so opening a table costs nothing.
type WorkbenchEntryRequest struct {
	Nodes        []WorkbenchEntryScope `json:"nodes"`
	Refresh      bool                  `json:"refresh"`
	IncludeLocal bool                  `json:"include_local"`
}

// WorkbenchEntryAddress is one entry address with its public picture.
type WorkbenchEntryAddress struct {
	IP          string `json:"ip"`
	CountryCode string `json:"country_code,omitempty"`
	Country     string `json:"country,omitempty"`
	City        string `json:"city,omitempty"`
	ISP         string `json:"isp,omitempty"`
	ASN         string `json:"asn,omitempty"`
	QueriedAt   string `json:"queried_at,omitempty"`
	Error       string `json:"error,omitempty"`
}

// WorkbenchNodeEntry is the resolved entry chain of a single node.
type WorkbenchNodeEntry struct {
	ProfileID         string                  `json:"profile_id"`
	NodeKey           string                  `json:"node_key"`
	NodeIdentityKey   string                  `json:"node_identity_key"`
	ConfigRevisionKey string                  `json:"config_revision_key"`
	Host              string                  `json:"host"`
	Source            string                  `json:"source"`
	Addresses         []WorkbenchEntryAddress `json:"addresses"`
	ResolvedAt        string                  `json:"resolved_at,omitempty"`
	Error             string                  `json:"error,omitempty"`
}

// WorkbenchLocalPosition is this machine's own public position, masked.
type WorkbenchLocalPosition struct {
	CountryCode string `json:"country_code,omitempty"`
	Country     string `json:"country,omitempty"`
	ISP         string `json:"isp,omitempty"`
	IPMasked    string `json:"ip_masked,omitempty"`
	QueriedAt   string `json:"queried_at,omitempty"`
	Error       string `json:"error,omitempty"`
}

// WorkbenchEntryResult is the batch answer. Nodes keeps the request order.
type WorkbenchEntryResult struct {
	Local *WorkbenchLocalPosition `json:"local,omitempty"`
	Nodes []WorkbenchNodeEntry    `json:"nodes"`
}

// ResolveWorkbenchEntries resolves node entries and this machine's position.
// Nothing here dials a node: DNS and the IP lookup use the physical interface.
func (s *AppService) ResolveWorkbenchEntries(ctx context.Context, req WorkbenchEntryRequest) (WorkbenchEntryResult, error) {
	if s == nil {
		return WorkbenchEntryResult{}, fmt.Errorf("app service is not available")
	}
	if len(req.Nodes) > 500 {
		return WorkbenchEntryResult{}, monitor.NewValidationError("一次最多解析 500 个节点")
	}
	for _, scope := range req.Nodes {
		if strings.TrimSpace(scope.ProfileID) == "" || strings.TrimSpace(scope.NodeKey) == "" {
			return WorkbenchEntryResult{}, monitor.NewValidationError("解析请求必须包含 profile 和 node_key")
		}
	}
	cache := s.workbenchEntryCache()
	result := WorkbenchEntryResult{Nodes: make([]WorkbenchNodeEntry, len(req.Nodes))}
	nodes, resolveErr := s.resolveEntryNodes(req.Nodes)
	client, clientErr := speedtester.NewPhysicalHTTPClient(entryResolveTimeout)

	pending := make([]int, 0, len(req.Nodes))
	for index, scope := range req.Nodes {
		key := entryScopeKey(scope)
		result.Nodes[index] = WorkbenchNodeEntry{
			ProfileID: scope.ProfileID, NodeKey: scope.NodeKey,
			NodeIdentityKey: scope.NodeIdentityKey, ConfigRevisionKey: scope.ConfigRevisionKey,
		}
		node, ok := nodes[key]
		if !ok {
			reason := "节点配置已变化，无法解析入口"
			if resolveErr != nil {
				reason = resolveErr.Error()
			}
			result.Nodes[index].Error = reason
			continue
		}
		host := strings.TrimSpace(node.Server)
		result.Nodes[index].Host = host
		if host == "" {
			result.Nodes[index].Error = "节点没有 server 地址"
			continue
		}
		if cached, ok := cache.Host(host, entry.HostTTL); ok && !req.Refresh {
			result.Nodes[index].Source = "cache"
			result.Nodes[index].ResolvedAt = cached.ResolvedAt.UTC().Format(time.RFC3339)
			for _, address := range cached.Addresses {
				if picture, ok := cache.Address(address.IP, entry.AddressTTL); ok {
					address = picture
				}
				result.Nodes[index].Addresses = append(result.Nodes[index].Addresses, entryAddressDTO(address))
			}
			continue
		}
		// 读缓存时不发 DNS/查询：只有显式刷新（解析入口或出口检测）才走网络。
		if !req.Refresh {
			result.Nodes[index].Error = "尚未解析入口"
			continue
		}
		pending = append(pending, index)
	}

	if len(pending) > 0 && clientErr != nil {
		for _, index := range pending {
			result.Nodes[index].Error = clientErr.Error()
		}
		pending = nil
	}
	resolved := make([]entry.Host, len(req.Nodes))
	if len(pending) > 0 {
		var wg sync.WaitGroup
		semaphore := make(chan struct{}, entryResolveWorkers)
		for _, index := range pending {
			index := index
			wg.Add(1)
			go func() {
				defer wg.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()
				host := result.Nodes[index].Host
				resolution, err := speedtester.ResolvePhysicalHost(ctx, host, entryResolveTimeout)
				record := entry.Host{Host: host, ResolvedAt: time.Now().UTC()}
				if err != nil {
					record.Error = err.Error()
					resolved[index] = record
					return
				}
				record.Source = resolution.Source
				for _, address := range resolution.Addresses {
					record.Addresses = append(record.Addresses, entry.Address{IP: address})
				}
				resolved[index] = record
			}()
		}
		wg.Wait()
	}

	// One batched lookup covers every address that is not cached yet.
	lookupCtx, cancel := context.WithTimeout(ctx, entryResolveTimeout)
	defer cancel()
	var ips []string
	for _, record := range resolved {
		if record.Error != "" {
			continue
		}
		for _, address := range record.Addresses {
			if _, ok := cache.Address(address.IP, entry.AddressTTL); ok && !req.Refresh {
				continue
			}
			ips = append(ips, address.IP)
		}
	}
	fetched := map[string]entry.Address{}
	failed := map[string]string{}
	if len(ips) > 0 && client != nil {
		if addresses, failures, err := entry.Lookup(lookupCtx, client, ips); err == nil {
			fetched = addresses
			failed = failures
		} else {
			for _, ip := range ips {
				failed[ip] = err.Error()
			}
		}
	}
	for _, record := range resolved {
		if record.Error != "" {
			continue
		}
		cache.PutHost(record)
	}
	for _, address := range fetched {
		cache.PutAddress(address)
	}
	for _, index := range pending {
		record := resolved[index]
		entryDTO := &result.Nodes[index]
		entryDTO.Source = record.Source
		entryDTO.Error = record.Error
		entryDTO.ResolvedAt = record.ResolvedAt.UTC().Format(time.RFC3339)
		for _, address := range record.Addresses {
			if cached, ok := cache.Address(address.IP, entry.AddressTTL); ok {
				entryDTO.Addresses = append(entryDTO.Addresses, entryAddressDTO(cached))
				continue
			}
			item := WorkbenchEntryAddress{IP: address.IP}
			if fetchedAddress, ok := fetched[address.IP]; ok {
				item = entryAddressDTO(fetchedAddress)
			} else if reason, ok := failed[address.IP]; ok {
				item.Error = reason
			}
			entryDTO.Addresses = append(entryDTO.Addresses, item)
		}
	}

	if req.IncludeLocal {
		local := s.workbenchLocalPosition(ctx, cache, client, req.Refresh)
		result.Local = &local
	}
	_ = cache.Save()
	return result, nil
}

func (s *AppService) workbenchLocalPosition(ctx context.Context, cache *entry.Cache, client *http.Client, refresh bool) WorkbenchLocalPosition {
	if cached, ok := cache.Local(entry.LocalTTL); ok && !refresh {
		return WorkbenchLocalPosition{CountryCode: cached.CountryCode, Country: cached.Country, ISP: cached.ISP, IPMasked: cached.IPMasked, QueriedAt: cached.QueriedAt.UTC().Format(time.RFC3339)}
	}
	if !refresh {
		return WorkbenchLocalPosition{Error: "尚未查询本机位置"}
	}
	if client == nil {
		return WorkbenchLocalPosition{Error: "没有可用的物理网卡"}
	}
	queryCtx, cancel := context.WithTimeout(ctx, entryResolveTimeout)
	defer cancel()
	local, err := entry.LookupLocal(queryCtx, client)
	if err != nil {
		return WorkbenchLocalPosition{Error: err.Error()}
	}
	cache.PutLocal(local)
	return WorkbenchLocalPosition{CountryCode: local.CountryCode, Country: local.Country, ISP: local.ISP, IPMasked: local.IPMasked, QueriedAt: local.QueriedAt.UTC().Format(time.RFC3339)}
}

func entryAddressDTO(address entry.Address) WorkbenchEntryAddress {
	item := WorkbenchEntryAddress{
		IP: address.IP, CountryCode: address.CountryCode, Country: address.Country,
		City: address.City, ISP: address.ISP, ASN: address.ASN,
	}
	if !address.QueriedAt.IsZero() {
		item.QueriedAt = address.QueriedAt.UTC().Format(time.RFC3339)
	}
	return item
}

func entryScopeKey(scope WorkbenchEntryScope) string {
	return scope.ProfileID + "\x00" + scope.NodeKey + "\x00" + scope.NodeIdentityKey + "\x00" + scope.ConfigRevisionKey
}

// workbenchEntryCache lazily opens the one cache shared by every caller.
func (s *AppService) workbenchEntryCache() *entry.Cache {
	s.entryMu.Lock()
	defer s.entryMu.Unlock()
	if s.entryCacheRef == nil {
		path := ""
		if root := strings.TrimSpace(s.appPaths.DataRoot); root != "" {
			path = filepath.Join(root, "entries.json")
		}
		s.entryCacheRef = entry.Open(path)
	}
	return s.entryCacheRef
}

// resolveEntryNodes loads each profile once and matches the requested node by
// its stable identity, exactly like the measurement paths do.
func (s *AppService) resolveEntryNodes(scopes []WorkbenchEntryScope) (map[string]monitor.MonitoredNode, error) {
	result := make(map[string]monitor.MonitoredNode, len(scopes))
	if len(scopes) == 0 {
		return result, nil
	}
	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil || store == nil {
		return result, fmt.Errorf("加载订阅配置失败")
	}
	grouped := map[string][]WorkbenchEntryScope{}
	order := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if _, ok := grouped[scope.ProfileID]; !ok {
			order = append(order, scope.ProfileID)
		}
		grouped[scope.ProfileID] = append(grouped[scope.ProfileID], scope)
	}
	var firstErr error
	for _, profileID := range order {
		exists := store.Get(profileID) != nil
		if !exists {
			_, subscription := store.FindSubscription(profileID)
			exists = subscription != nil
		}
		if !exists {
			if firstErr == nil {
				firstErr = fmt.Errorf("订阅不存在或已被移除")
			}
			continue
		}
		tester, err := speedtester.New(&speedtester.Config{ConfigPaths: s.profilePaths.CacheFile(profileID), Mode: speedtester.SpeedModeFast, Metrics: speedtester.MetricSet{Latency: true}, Concurrent: 1})
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("初始化订阅配置失败: %w", err)
			}
			continue
		}
		proxies, err := tester.LoadProxies()
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		names := make([]string, 0, len(proxies))
		for name := range proxies {
			names = append(names, name)
		}
		sort.Strings(names)
		exact := make(map[string]monitor.MonitoredNode, len(proxies))
		loose := map[string][]monitor.MonitoredNode{}
		for _, name := range names {
			node := monitoredNodeFromProxy(name, proxies[name])
			exact[entryScopeKey(WorkbenchEntryScope{ProfileID: profileID, NodeKey: node.NodeKey, NodeIdentityKey: node.NodeIdentityKey, ConfigRevisionKey: node.ConfigRevisionKey})] = node
			loose[node.NodeKey] = append(loose[node.NodeKey], node)
		}
		for _, scope := range grouped[profileID] {
			if node, ok := exact[entryScopeKey(scope)]; ok {
				result[entryScopeKey(scope)] = node
				continue
			}
			if candidates := loose[scope.NodeKey]; len(candidates) == 1 {
				result[entryScopeKey(scope)] = candidates[0]
			}
		}
	}
	return result, firstErr
}
