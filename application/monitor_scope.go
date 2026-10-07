package application

import (
	"fmt"
	"github.com/faceair/clash-speedtest/core/monitor"
)

type MonitorNodeResolutionIssue struct {
	monitor.MonitorJobNodeReference
	Reason string `json:"reason"`
}

func monitorResolutionIssues(refs []monitor.MonitorJobNodeReference, current []monitor.MonitoredNode) []MonitorNodeResolutionIssue {
	issues := []MonitorNodeResolutionIssue{}
	for _, ref := range refs {
		if _, reason := resolvePersistedMonitorNodes([]monitor.MonitorJobNodeReference{ref}, current); reason != "" {
			issues = append(issues, MonitorNodeResolutionIssue{ref, reason})
		}
	}
	return issues
}
func (s *AppService) enrichMonitorScope(dto *MonitorJobDTO, job monitor.MonitorJob) {
	dto.ScopeKind = "frozen_task"
	if s.periodic != nil {
		s.periodic.mu.Lock()
		for _, entry := range s.periodic.jobs {
			if entry.ID == job.ID {
				dto.ScopeKind = "current_patrol"
				break
			}
		}
		s.periodic.mu.Unlock()
	}
	if job.State != monitor.JobStateBlocked {
		return
	}
	refs := make([]monitor.MonitorJobNodeReference, 0, len(job.Nodes))
	for _, n := range job.Nodes {
		refs = append(refs, monitor.MonitorJobNodeReference{NodeKey: n.NodeKey, NodeIdentityKey: n.NodeIdentityKey, ConfigRevisionKey: n.ConfigRevisionKey, DisplayName: n.DisplayName, Type: n.Type})
	}
	current, err := s.loadMonitorNodes(job.ProfileID)
	if err != nil {
		for _, ref := range refs {
			dto.UnresolvedNodes = append(dto.UnresolvedNodes, MonitorNodeResolutionIssue{ref, "当前订阅缓存无法读取；不能判断该节点是否已移除"})
		}
	} else {
		dto.UnresolvedNodes = monitorResolutionIssues(refs, current)
	}
	dto.UnresolvedNodeCount = len(dto.UnresolvedNodes)
	if len(dto.UnresolvedNodes) > 0 {
		dto.BlockedReason = fmt.Sprintf("整项任务未启动，冻结范围中共 %d 个节点缺失或配置不一致；其余节点也未执行。", len(dto.UnresolvedNodes))
	} else {
		dto.BlockedReason = "整项任务未启动：" + dto.BlockedReason
	}
}
