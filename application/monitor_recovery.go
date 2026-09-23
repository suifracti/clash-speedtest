package application

import (
	"context"
	"fmt"

	"github.com/faceair/clash-speedtest/core/appdata"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/profiles"
)

// RecoverMonitorJobs is the application-launch entry point. It is deliberately
// explicit so construction, tests, and ordinary settings writes never start a
// scheduler. Calling it more than once in one process is harmless.
func (s *AppService) RecoverMonitorJobs(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.monitorRecoveryMu.Lock()
	defer s.monitorRecoveryMu.Unlock()
	if s.monitorRecoveryStarted {
		return nil
	}
	if s.historyStore == nil {
		return fmt.Errorf("monitor history store is not initialized")
	}
	info := s.appPaths.InspectMigration()
	if info.State != appdata.MigrationStateReady && info.State != appdata.MigrationStateIsolated {
		return fmt.Errorf("monitor recovery waits for a stable data root: %s", info.State)
	}
	s.monitorRecoveryStarted = true
	if err := s.historyStore.MarkRunningMonitorRunsInterrupted(ctx); err != nil {
		s.blockEligibleMonitorRecoveries("无法标记中断历史：" + err.Error())
		return fmt.Errorf("mark unfinished monitor runs interrupted before recovery: %w", err)
	}
	definitions, err := s.historyStore.ListMonitorJobDefinitions(ctx)
	if err != nil {
		s.blockEligibleMonitorRecoveries("无法读取持久化恢复意图：" + err.Error())
		return fmt.Errorf("read monitor launch intents: %w", err)
	}

	profileStore, profileErr := profiles.LoadStore(s.profilePaths.StoreFile())
	for _, definition := range definitions {
		if definition == nil || !definition.ResumeOnLaunch || normalizedDesiredState(definition.DesiredState) != monitor.JobStateRunning {
			continue
		}
		s.monitorMu.Lock()
		sched := s.monitorSchedulers[definition.ID]
		if sched == nil {
			s.monitorMu.Unlock()
			continue
		}
		current := sched.Job()
		// Runtime settings may have changed after the durable snapshot was read.
		if !current.ResumeOnLaunch || current.DesiredState != monitor.JobStateRunning {
			s.monitorMu.Unlock()
			continue
		}
		s.monitorMu.Unlock()

		resolved, blockedReason := s.resolveRecoveryNodes(definition, profileStore, profileErr)
		if blockedReason != "" {
			s.setRecoveryBlocked(sched, nil, blockedReason, true)
			continue
		}
		if reason := s.monitorStorageGuard(); reason != "" {
			s.setRecoveryBlocked(sched, resolved, "Monitor 容量保护阻止启动恢复："+reason, false)
			continue
		}
		if code, reason := s.monitorBudgetGuard(); code != "" {
			if reason == "" {
				reason = code
			}
			s.setRecoveryBlocked(sched, resolved, "Monitor 全局预算阻止启动恢复："+reason, false)
			continue
		}

		s.monitorMu.Lock()
		current = sched.Job()
		if !current.ResumeOnLaunch || current.DesiredState != monitor.JobStateRunning {
			s.monitorMu.Unlock()
			continue
		}
		if err := sched.UpdateRecoveryResolution(resolved, ""); err != nil {
			s.monitorMu.Unlock()
			return fmt.Errorf("refresh monitor job %s before recovery: %w", definition.ID, err)
		}
		sched.SetLaunchIntent(current.ResumeOnLaunch, current.DesiredState, monitor.RecoveryStateRestoring, "", "")
		if err := sched.StartRecovered(ctx); err != nil {
			reason := "应用启动恢复失败：" + err.Error()
			sched.SetLaunchIntent(current.ResumeOnLaunch, current.DesiredState, monitor.RecoveryStateBlocked, reason, "")
			s.monitorMu.Unlock()
			continue
		}
		s.monitorMu.Unlock()
	}
	return nil
}

func (s *AppService) blockEligibleMonitorRecoveries(reason string) {
	s.monitorMu.RLock()
	schedulers := make([]*monitor.Scheduler, 0, len(s.monitorSchedulers))
	for _, sched := range s.monitorSchedulers {
		schedulers = append(schedulers, sched)
	}
	s.monitorMu.RUnlock()
	for _, sched := range schedulers {
		s.monitorMu.Lock()
		job := sched.Job()
		if job.ResumeOnLaunch && job.DesiredState == monitor.JobStateRunning {
			sched.SetLaunchIntent(job.ResumeOnLaunch, job.DesiredState, monitor.RecoveryStateBlocked, reason, "")
		}
		s.monitorMu.Unlock()
	}
}

func (s *AppService) resolveRecoveryNodes(definition *monitor.MonitorJobDefinition, profileStore *profiles.Store, profileErr error) ([]monitor.MonitoredNode, string) {
	if definition.DefinitionVersion != monitor.MonitorJobDefinitionVersion {
		return nil, fmt.Sprintf("任务定义版本 %d 不受当前版本支持", definition.DefinitionVersion)
	}
	if profileErr != nil {
		return nil, fmt.Sprintf("当前订阅配置不可用：%v", profileErr)
	}
	if profileStore == nil || profileStore.Get(definition.ProfileID) == nil {
		return nil, "订阅不存在或已被移除"
	}
	current, err := s.loadMonitorNodes(definition.ProfileID)
	if err != nil {
		return nil, fmt.Sprintf("当前订阅节点不可用：%v", err)
	}
	resolved, reason := resolvePersistedMonitorNodes(definition.Nodes, current)
	if reason != "" {
		return nil, reason
	}
	return resolved, ""
}

func (s *AppService) setRecoveryBlocked(sched *monitor.Scheduler, nodes []monitor.MonitoredNode, reason string, configBlocked bool) {
	s.monitorMu.Lock()
	defer s.monitorMu.Unlock()
	job := sched.Job()
	if !job.ResumeOnLaunch || job.DesiredState != monitor.JobStateRunning {
		return
	}
	resolutionReason := ""
	if configBlocked {
		resolutionReason = reason
	}
	if err := sched.UpdateRecoveryResolution(nodes, resolutionReason); err != nil {
		return
	}
	sched.SetLaunchIntent(job.ResumeOnLaunch, job.DesiredState, monitor.RecoveryStateBlocked, reason, "")
}

func (s *AppService) refreshBlockedMonitorJob(sched *monitor.Scheduler) error {
	job := sched.Job()
	if job.State != monitor.JobStateBlocked {
		return nil
	}
	profileStore, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return fmt.Errorf("当前订阅配置不可用：%w", err)
	}
	if profileStore == nil || profileStore.Get(job.ProfileID) == nil {
		return fmt.Errorf("订阅不存在或已被移除")
	}
	current, err := s.loadMonitorNodes(job.ProfileID)
	if err != nil {
		return fmt.Errorf("当前订阅节点不可用：%w", err)
	}
	definition := monitorJobDefinitionFromJob(job)
	resolved, reason := resolvePersistedMonitorNodes(definition.Nodes, current)
	if reason != "" {
		return fmt.Errorf("%s", reason)
	}
	return sched.UpdateRecoveryResolution(resolved, "")
}
