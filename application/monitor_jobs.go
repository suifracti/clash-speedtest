package application

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/profiles"
	"github.com/faceair/clash-speedtest/core/speedtester"
)

// MonitorNodeOptionDTO is the safe node-selection read model. It identifies a
// node without exposing the subscription's raw credentials to the UI.
type MonitorNodeOptionDTO struct {
	ProfileID         string `json:"profile_id"`
	ProfileName       string `json:"profile_name"`
	NodeKey           string `json:"node_key"`
	NodeIdentityKey   string `json:"node_identity_key"`
	ConfigRevisionKey string `json:"config_revision_key"`
	DisplayName       string `json:"display_name"`
	Type              string `json:"type"`
	CountryCode       string `json:"country_code"`
	CountryFlag       string `json:"country_flag"`
}

// MonitorNodeSelectionContext is an optional, credential-free snapshot from
// Workbench. It is only a stale-selection guard; current cache resolution
// remains the authority for the runnable node configuration.
type MonitorNodeSelectionContext struct {
	NodeKey           string `json:"node_key"`
	NodeIdentityKey   string `json:"node_identity_key"`
	ConfigRevisionKey string `json:"config_revision_key"`
}

// MonitorJobCreateRequest is the only UI-facing monitor-job creation shape.
// Durations are explicitly seconds at this boundary; monitor.MonitorJob keeps
// Go time.Duration internally (nanoseconds when JSON-marshaled).
type MonitorJobCreateRequest struct {
	Name            string                        `json:"name"`
	ProfileID       string                        `json:"profile_id"`
	NodeKeys        []string                      `json:"node_keys"`
	ProbeSet        monitor.ProbeSetType          `json:"probe_set"`
	IntervalSeconds int64                         `json:"interval_seconds"`
	TimeoutSeconds  int64                         `json:"timeout_seconds"`
	NodeContexts    []MonitorNodeSelectionContext `json:"node_contexts,omitempty"`
}

// MonitorJobNodeDTO is the public, credential-free projection of a monitored node.
type MonitorJobNodeDTO struct {
	NodeKey           string `json:"node_key"`
	NodeIdentityKey   string `json:"node_identity_key"`
	ConfigRevisionKey string `json:"config_revision_key"`
	DisplayName       string `json:"display_name"`
	Type              string `json:"type"`
}

// MonitorJobDTO is the public monitor-job read model. RawConfig is deliberately
// absent: the scheduler retains it in memory, but neither Web nor Wails needs it.
type MonitorJobDTO struct {
	ID               string               `json:"id"`
	Name             string               `json:"name"`
	ProfileID        string               `json:"profile_id"`
	ProfileName      string               `json:"profile_name"`
	NodeKeys         []string             `json:"node_keys"`
	Nodes            []MonitorJobNodeDTO  `json:"nodes"`
	ProbeSet         monitor.ProbeSetType `json:"probe_set"`
	IntervalSeconds  int64                `json:"interval_seconds"`
	TimeoutSeconds   int64                `json:"timeout_seconds"`
	State            monitor.JobState     `json:"state"`
	BlockedReason    string               `json:"blocked_reason,omitempty"`
	PersistenceState string               `json:"persistence_state"`
	PersistenceError string               `json:"persistence_error,omitempty"`
	StorageState     string               `json:"storage_state,omitempty"`
	StorageReason    string               `json:"storage_reason,omitempty"`
	CreatedAt        time.Time            `json:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at"`
}

const (
	minMonitorIntervalSeconds = int64(1)
	maxMonitorIntervalSeconds = int64(24 * 60 * 60)
	minMonitorTimeoutSeconds  = int64(1)
	maxMonitorTimeoutSeconds  = int64(2 * 60)
)

// ListMonitorNodeOptions resolves the currently cached subscription configs into
// stable, credential-free choices for monitor-job creation.
func (s *AppService) ListMonitorNodeOptions() ([]MonitorNodeOptionDTO, error) {
	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return nil, err
	}

	options := make([]MonitorNodeOptionDTO, 0)
	for _, airport := range store.Airports {
		if airport == nil {
			continue
		}
		nodes, err := s.loadMonitorNodes(airport.ID)
		if err != nil {
			// A missing or invalid cache should make that subscription unavailable,
			// not turn a healthy subscription into an unusable global selector.
			continue
		}
		seen := make(map[string]struct{}, len(nodes))
		for _, node := range nodes {
			if node.NodeKey == "" {
				continue
			}
			if _, exists := seen[node.NodeKey]; exists {
				continue
			}
			seen[node.NodeKey] = struct{}{}
			code := profiles.DetectCountry(node.DisplayName)
			options = append(options, MonitorNodeOptionDTO{
				ProfileID:         airport.ID,
				ProfileName:       airport.Name,
				NodeKey:           node.NodeKey,
				NodeIdentityKey:   node.NodeIdentityKey,
				ConfigRevisionKey: node.ConfigRevisionKey,
				DisplayName:       node.DisplayName,
				Type:              node.Type,
				CountryCode:       code,
				CountryFlag:       profiles.FlagFromCode(code),
			})
		}
	}

	sort.Slice(options, func(i, j int) bool {
		if options[i].ProfileName != options[j].ProfileName {
			return options[i].ProfileName < options[j].ProfileName
		}
		return options[i].DisplayName < options[j].DisplayName
	})
	return options, nil
}

// CreateMonitorJobFromRequest resolves node keys against the current cached
// subscription before registering and durably saving the scheduler definition.
func (s *AppService) CreateMonitorJobFromRequest(req MonitorJobCreateRequest) (*MonitorJobDTO, error) {
	profileID := strings.TrimSpace(req.ProfileID)
	if profileID == "" {
		return nil, monitor.NewValidationError("profile_id 不能为空")
	}
	if len(req.NodeKeys) == 0 {
		return nil, monitor.NewValidationError("至少选择一个订阅节点")
	}
	if !validMonitorProbeSet(req.ProbeSet) {
		return nil, monitor.NewValidationError("probe_set 必须是 light、service 或 heavy")
	}
	if req.IntervalSeconds < minMonitorIntervalSeconds || req.IntervalSeconds > maxMonitorIntervalSeconds {
		return nil, monitor.NewValidationError(fmt.Sprintf("interval_seconds 必须在 %d 到 %d 之间", minMonitorIntervalSeconds, maxMonitorIntervalSeconds))
	}
	if req.TimeoutSeconds < minMonitorTimeoutSeconds || req.TimeoutSeconds > maxMonitorTimeoutSeconds {
		return nil, monitor.NewValidationError(fmt.Sprintf("timeout_seconds 必须在 %d 到 %d 之间", minMonitorTimeoutSeconds, maxMonitorTimeoutSeconds))
	}

	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return nil, err
	}
	airport := store.Get(profileID)
	if airport == nil {
		return nil, monitor.NewValidationError("订阅不存在或已被移除")
	}

	available, err := s.loadMonitorNodes(profileID)
	if err != nil {
		return nil, monitor.WrapValidationError(err)
	}
	byKey := make(map[string]monitor.MonitoredNode, len(available))
	for _, node := range available {
		if node.NodeKey != "" {
			byKey[node.NodeKey] = node
		}
	}
	contextByKey, err := monitorNodeContextIndex(req)
	if err != nil {
		return nil, err
	}

	selected := make([]monitor.MonitoredNode, 0, len(req.NodeKeys))
	seen := make(map[string]struct{}, len(req.NodeKeys))
	for _, rawKey := range req.NodeKeys {
		key := strings.TrimSpace(rawKey)
		if key == "" {
			return nil, monitor.NewValidationError("node_keys 不能包含空值")
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, monitor.NewValidationError("node_keys 不能重复")
		}
		seen[key] = struct{}{}
		node, ok := byKey[key]
		if !ok || len(node.RawConfig) == 0 {
			return nil, monitor.NewValidationError("所选节点已不存在或订阅配置已变化，请重新选择")
		}
		if snapshot, guarded := contextByKey[key]; guarded &&
			(node.NodeIdentityKey != snapshot.NodeIdentityKey || node.ConfigRevisionKey != snapshot.ConfigRevisionKey) {
			return nil, monitor.NewValidationError("所选节点的稳定身份或配置 revision 已变化，请返回工作台重新选择")
		}
		selected = append(selected, node)
	}

	job, err := s.CreateMonitorJob(monitor.MonitorJob{
		Name:      strings.TrimSpace(req.Name),
		ProfileID: profileID,
		NodeKeys:  append([]string(nil), req.NodeKeys...),
		Nodes:     selected,
		ProbeSet:  req.ProbeSet,
		Interval:  time.Duration(req.IntervalSeconds) * time.Second,
		Timeout:   time.Duration(req.TimeoutSeconds) * time.Second,
	})
	if err != nil {
		return nil, err
	}
	dto := monitorJobDTO(*job, airport.Name)
	return &dto, nil
}

func monitorNodeContextIndex(req MonitorJobCreateRequest) (map[string]MonitorNodeSelectionContext, error) {
	if len(req.NodeContexts) == 0 {
		return nil, nil
	}
	if len(req.NodeContexts) != len(req.NodeKeys) {
		return nil, monitor.NewValidationError("节点选择上下文不完整，请返回工作台重新选择")
	}

	indexed := make(map[string]MonitorNodeSelectionContext, len(req.NodeContexts))
	for _, snapshot := range req.NodeContexts {
		key := strings.TrimSpace(snapshot.NodeKey)
		if key == "" || strings.TrimSpace(snapshot.NodeIdentityKey) == "" || strings.TrimSpace(snapshot.ConfigRevisionKey) == "" {
			return nil, monitor.NewValidationError("节点选择上下文缺少稳定身份或配置 revision，请返回工作台重新选择")
		}
		if _, exists := indexed[key]; exists {
			return nil, monitor.NewValidationError("节点选择上下文不能重复")
		}
		snapshot.NodeKey = key
		indexed[key] = snapshot
	}
	for _, rawKey := range req.NodeKeys {
		key := strings.TrimSpace(rawKey)
		if _, exists := indexed[key]; !exists {
			return nil, monitor.NewValidationError("节点选择上下文与所选节点不一致，请返回工作台重新选择")
		}
	}
	return indexed, nil
}

// ListMonitorJobDTOs returns the safe status projection used by Web and Wails.
func (s *AppService) ListMonitorJobDTOs() ([]MonitorJobDTO, error) {
	if s.monitorLoadErr != nil {
		return nil, fmt.Errorf("load monitor job definitions: %w", s.monitorLoadErr)
	}
	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return nil, err
	}
	profileNames := make(map[string]string, len(store.Airports))
	for _, airport := range store.Airports {
		if airport != nil {
			profileNames[airport.ID] = airport.Name
		}
	}

	jobs := s.ListMonitorJobs()
	dtos := make([]MonitorJobDTO, 0, len(jobs))
	for _, job := range jobs {
		dtos = append(dtos, monitorJobDTO(job, profileNames[job.ProfileID]))
	}
	return dtos, nil
}

// loadPersistedMonitorJobs reconstructs scheduler objects from durable product
// definitions. It never starts a scheduler or executes a probe.
func (s *AppService) loadPersistedMonitorJobs() error {
	if s.historyStore == nil {
		return nil
	}
	definitions, err := s.historyStore.ListMonitorJobDefinitions(context.Background())
	if err != nil {
		return err
	}
	if len(definitions) == 0 {
		return nil
	}

	profileStore, profileErr := profiles.LoadStore(s.profilePaths.StoreFile())
	nodeCache := make(map[string][]monitor.MonitoredNode)
	nodeErrors := make(map[string]error)
	for _, definition := range definitions {
		if definition == nil {
			return fmt.Errorf("monitor job definition is nil")
		}
		job := monitor.MonitorJob{
			ID:        definition.ID,
			Name:      definition.Name,
			ProfileID: definition.ProfileID,
			NodeKeys:  make([]string, 0, len(definition.Nodes)),
			ProbeSet:  definition.ProbeSet,
			Interval:  definition.Interval,
			Timeout:   definition.Timeout,
			CreatedAt: definition.CreatedAt,
			UpdatedAt: definition.UpdatedAt,
		}
		job.Nodes = monitorNodesFromReferences(definition.Nodes)
		for _, node := range definition.Nodes {
			job.NodeKeys = append(job.NodeKeys, node.NodeKey)
		}

		blockedReason := ""
		switch {
		case definition.DefinitionVersion != monitor.MonitorJobDefinitionVersion:
			blockedReason = fmt.Sprintf("任务定义版本 %d 不受当前版本支持", definition.DefinitionVersion)
		case profileErr != nil:
			blockedReason = fmt.Sprintf("当前订阅配置不可用：%v", profileErr)
		case profileStore == nil || profileStore.Get(definition.ProfileID) == nil:
			blockedReason = "订阅不存在或已被移除"
		default:
			currentNodes, ok := nodeCache[definition.ProfileID]
			if !ok {
				currentNodes, nodeErrors[definition.ProfileID] = s.loadMonitorNodes(definition.ProfileID)
				nodeCache[definition.ProfileID] = currentNodes
			}
			if nodeErrors[definition.ProfileID] != nil {
				blockedReason = fmt.Sprintf("当前订阅节点不可用：%v", nodeErrors[definition.ProfileID])
			} else {
				resolved, reason := resolvePersistedMonitorNodes(definition.Nodes, currentNodes)
				if reason != "" {
					blockedReason = reason
				} else {
					job.Nodes = resolved
				}
			}
		}

		if blockedReason != "" {
			job.State = monitor.JobStateBlocked
			job.BlockedReason = blockedReason
		} else {
			job.State = monitor.JobStateStopped
			job.BlockedReason = ""
		}

		sched, err := monitor.NewScheduler(monitor.SchedulerConfig{
			Job:          &job,
			Runner:       s.monitorRunner,
			Store:        s.historyStore,
			StorageGuard: s.monitorStorageGuard,
		})
		if err != nil {
			return fmt.Errorf("restore monitor job %s: %w", definition.ID, err)
		}
		if _, exists := s.monitorSchedulers[definition.ID]; exists {
			return fmt.Errorf("duplicate persisted monitor job %s", definition.ID)
		}
		s.monitorSchedulers[definition.ID] = sched
	}
	return nil
}

func monitorNodesFromReferences(references []monitor.MonitorJobNodeReference) []monitor.MonitoredNode {
	nodes := make([]monitor.MonitoredNode, 0, len(references))
	for _, reference := range references {
		nodes = append(nodes, monitor.MonitoredNode{
			NodeKey:           reference.NodeKey,
			NodeIdentityKey:   reference.NodeIdentityKey,
			ConfigRevisionKey: reference.ConfigRevisionKey,
			DisplayName:       reference.DisplayName,
			Type:              reference.Type,
		})
	}
	return nodes
}

func monitorJobDefinitionFromJob(job monitor.MonitorJob) *monitor.MonitorJobDefinition {
	references := make([]monitor.MonitorJobNodeReference, 0, len(job.Nodes))
	for _, node := range job.Nodes {
		references = append(references, monitor.MonitorJobNodeReference{
			NodeKey:           node.NodeKey,
			NodeIdentityKey:   node.NodeIdentityKey,
			ConfigRevisionKey: node.ConfigRevisionKey,
			DisplayName:       node.DisplayName,
			Type:              node.Type,
		})
	}
	return &monitor.MonitorJobDefinition{
		ID:                job.ID,
		Name:              job.Name,
		ProfileID:         job.ProfileID,
		Nodes:             references,
		ProbeSet:          job.ProbeSet,
		Interval:          job.Interval,
		Timeout:           job.Timeout,
		CreatedAt:         job.CreatedAt,
		UpdatedAt:         job.UpdatedAt,
		DefinitionVersion: monitor.MonitorJobDefinitionVersion,
	}
}

func resolvePersistedMonitorNodes(references []monitor.MonitorJobNodeReference, current []monitor.MonitoredNode) ([]monitor.MonitoredNode, string) {
	resolved := make([]monitor.MonitoredNode, 0, len(references))
	for _, reference := range references {
		if reference.NodeKey == "" || reference.NodeIdentityKey == "" || reference.ConfigRevisionKey == "" {
			return nil, "任务包含不完整的稳定节点引用，需要重新配置"
		}
		exact := make([]monitor.MonitoredNode, 0, 1)
		byIdentity := make([]monitor.MonitoredNode, 0, 1)
		for _, candidate := range current {
			if candidate.NodeIdentityKey == reference.NodeIdentityKey {
				byIdentity = append(byIdentity, candidate)
			}
			if candidate.NodeKey == reference.NodeKey && candidate.NodeIdentityKey == reference.NodeIdentityKey {
				exact = append(exact, candidate)
			}
		}
		if len(exact) == 1 {
			if exact[0].ConfigRevisionKey != reference.ConfigRevisionKey {
				return nil, fmt.Sprintf("节点 %q 的配置 revision 已变化，需要重新确认", reference.DisplayName)
			}
			resolved = append(resolved, exact[0])
			continue
		}
		if len(byIdentity) == 1 && byIdentity[0].ConfigRevisionKey != reference.ConfigRevisionKey {
			return nil, fmt.Sprintf("节点 %q 的配置 revision 已变化，需要重新确认", reference.DisplayName)
		}
		if len(byIdentity) > 1 {
			return nil, fmt.Sprintf("节点 %q 的稳定身份不再唯一，无法安全恢复", reference.DisplayName)
		}
		return nil, fmt.Sprintf("节点 %q 已不存在或不可用", reference.DisplayName)
	}
	return resolved, ""
}

// GetMonitorJobDTO returns the safe status projection for one job.
func (s *AppService) GetMonitorJobDTO(jobID string) (*MonitorJobDTO, error) {
	job, err := s.GetMonitorJob(jobID)
	if err != nil {
		return nil, err
	}
	profileName := ""
	if store, loadErr := profiles.LoadStore(s.profilePaths.StoreFile()); loadErr == nil {
		if airport := store.Get(job.ProfileID); airport != nil {
			profileName = airport.Name
		}
	}
	dto := monitorJobDTO(*job, profileName)
	return &dto, nil
}

func (s *AppService) loadMonitorNodes(profileID string) ([]monitor.MonitoredNode, error) {
	if !s.profilePaths.HasCache(profileID) {
		return nil, fmt.Errorf("订阅节点缓存不存在，请先刷新订阅")
	}
	st, err := speedtester.New(&speedtester.Config{
		ConfigPaths: s.profilePaths.CacheFile(profileID),
		Mode:        speedtester.SpeedModeFast,
	})
	if err != nil {
		return nil, fmt.Errorf("初始化订阅配置失败: %w", err)
	}
	proxies, err := st.LoadProxies()
	if err != nil {
		return nil, fmt.Errorf("读取订阅配置失败: %w", err)
	}

	nodes := make([]monitor.MonitoredNode, 0, len(proxies))
	for name, proxy := range proxies {
		if proxy == nil || len(proxy.Config) == 0 {
			continue
		}
		node := monitor.MonitoredNode{
			DisplayName: name,
			Type:        proxy.Type().String(),
			Server:      configString(proxy.Config, "server"),
			Port:        configInt(proxy.Config, "port"),
			RawConfig:   proxy.Config,
		}
		monitor.PopulateNodeKeys(&node)
		if node.NodeKey == "" || node.NodeKey == "nk_unknown" || node.NodeIdentityKey == "" || len(node.RawConfig) == 0 {
			continue
		}
		nodes = append(nodes, node)
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("订阅中没有可运行的节点配置")
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].DisplayName < nodes[j].DisplayName })
	return nodes, nil
}

func monitorJobDTO(job monitor.MonitorJob, profileName string) MonitorJobDTO {
	nodes := make([]MonitorJobNodeDTO, 0, len(job.Nodes))
	for _, node := range job.Nodes {
		nodes = append(nodes, MonitorJobNodeDTO{
			NodeKey:           node.NodeKey,
			NodeIdentityKey:   node.NodeIdentityKey,
			ConfigRevisionKey: node.ConfigRevisionKey,
			DisplayName:       node.DisplayName,
			Type:              node.Type,
		})
	}
	nodeKeys := append([]string(nil), job.NodeKeys...)
	if len(nodeKeys) == 0 {
		for _, node := range nodes {
			nodeKeys = append(nodeKeys, node.NodeKey)
		}
	}
	return MonitorJobDTO{
		ID:               job.ID,
		Name:             job.Name,
		ProfileID:        job.ProfileID,
		ProfileName:      profileName,
		NodeKeys:         nodeKeys,
		Nodes:            nodes,
		ProbeSet:         job.ProbeSet,
		IntervalSeconds:  int64(job.Interval / time.Second),
		TimeoutSeconds:   int64(job.Timeout / time.Second),
		State:            job.State,
		BlockedReason:    job.BlockedReason,
		PersistenceState: job.PersistenceState,
		PersistenceError: job.PersistenceError,
		StorageState:     job.StorageState,
		StorageReason:    job.StorageReason,
		CreatedAt:        job.CreatedAt,
		UpdatedAt:        job.UpdatedAt,
	}
}

func validMonitorProbeSet(probeSet monitor.ProbeSetType) bool {
	switch probeSet {
	case monitor.ProbeSetLight, monitor.ProbeSetService, monitor.ProbeSetHeavy:
		return true
	default:
		return false
	}
}

func configString(config map[string]any, key string) string {
	value, _ := config[key].(string)
	return strings.TrimSpace(value)
}

func configInt(config map[string]any, key string) int {
	switch value := config[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(value))
		return parsed
	default:
		return 0
	}
}
