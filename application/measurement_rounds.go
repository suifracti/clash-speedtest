package application

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
	"github.com/faceair/clash-speedtest/core/publicservice"
)

type scheduledRoundContextKey struct{}
type periodicRoundIDContextKey struct{}

func periodicRoundRequestID(round string, n MonitorNodeOptionDTO, service string) string {
	sum := sha256.Sum256([]byte(n.ProfileID + "\x00" + n.NodeKey + "\x00" + n.NodeIdentityKey + "\x00" + n.ConfigRevisionKey + "\x00" + service))
	return round + ":" + fmt.Sprintf("%x", sum[:16])
}
func (s *AppService) CreateManualMeasurementRound(ctx context.Context, r history.MeasurementRound) error {
	if s.historyStore == nil {
		return fmt.Errorf("history store is not initialized")
	}
	// Origin and creation time are assigned here, never accepted from Web JSON.
	r.TriggerType = "manual"
	r.StartedAt = time.Now().UTC()
	r.State = "running"
	cache := map[string][]monitor.MonitoredNode{}
	for _, i := range r.Items {
		nodes, ok := cache[i.ProfileID]
		if !ok {
			var err error
			nodes, err = s.loadMonitorNodes(i.ProfileID)
			if err != nil {
				return err
			}
			cache[i.ProfileID] = nodes
		}
		var node monitor.MonitoredNode
		found := false
		for _, n := range nodes {
			if n.NodeKey == i.NodeKey && n.NodeIdentityKey == i.NodeIdentityKey {
				node = n
				found = true
				break
			}
		}
		if !found {
			return monitor.NewValidationError("计划节点已不在当前订阅中")
		}
		if node.NodeIdentityKey != i.NodeIdentityKey || node.ConfigRevisionKey != i.ConfigRevisionKey {
			return monitor.NewValidationError("计划节点配置已变化，需要重新确认")
		}
		if i.Project == "service" {
			if _, ok := publicservice.RuleFor(i.ServiceID); !ok {
				return monitor.NewValidationError("未知服务规则")
			}
		}
	}
	return s.historyStore.DB().CreateMeasurementRound(ctx, r)
}
func (s *AppService) validateMeasurementRequest(ctx context.Context, requestID, profile, node, nid, rev, project, service string) error {
	trigger, err := s.historyStore.DB().MeasurementRequestTrigger(ctx, requestID, profile, nid, rev, project, service)
	if err != nil {
		return err
	}
	if trigger == "scheduled" || trigger == "diagnostic" {
		if ctx.Value(scheduledRoundContextKey{}) != trigger {
			return monitor.NewValidationError("定时/诊断计划项只能由对应后台任务执行")
		}
	}
	if trigger == "" && project != "latency" {
		// Compatibility callers get an explicit individual manual round; old DB
		// rows are left unlinked. No timestamp/name based backfill is performed.
		r := history.MeasurementRound{RoundID: "manual-" + requestID, TriggerType: "manual", StartedAt: time.Now().UTC(), Items: []history.MeasurementRoundItem{{RequestID: requestID, Project: project, ServiceID: service, ProfileID: profile, NodeKey: node, NodeIdentityKey: nid, ConfigRevisionKey: rev}}}
		if len(r.RoundID) > 96 {
			sum := sha256.Sum256([]byte(requestID))
			r.RoundID = fmt.Sprintf("manual-%x", sum[:16])
		}
		if err := s.historyStore.DB().CreateMeasurementRound(ctx, r); err != nil {
			return err
		}
	}
	return nil
}
func (s *AppService) ListMeasurementRoundNodes(ctx context.Context, profile, node, nid, rev string, limit int) ([]history.MeasurementRound, error) {
	if s.historyStore == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	return s.historyStore.DB().QueryMeasurementRoundNodes(ctx, profile, node, nid, rev, limit)
}
func (s *AppService) FinishManualMeasurementRound(ctx context.Context, id, state string) error {
	if s.historyStore == nil {
		return fmt.Errorf("history store is not initialized")
	}
	return s.historyStore.DB().FinishManualMeasurementRound(ctx, id, state)
}

func (s *AppService) ensureManualLatencyRound(ctx context.Context, req WorkbenchLatencyBatchRequest, selections []WorkbenchLatencyBatchSelection) error {
	trigger, err := s.historyStore.DB().MeasurementRequestTrigger(ctx, req.RequestID, selections[0].ProfileID, selections[0].NodeIdentityKey, selections[0].ConfigRevisionKey, "latency", "")
	if err != nil {
		return err
	}
	if trigger != "" {
		for _, n := range selections {
			t, e := s.historyStore.DB().MeasurementRequestTrigger(ctx, req.RequestID, n.ProfileID, n.NodeIdentityKey, n.ConfigRevisionKey, "latency", "")
			if e != nil {
				return e
			}
			if t != trigger {
				return fmt.Errorf("batch selection differs from frozen round plan")
			}
		}
		return nil
	}
	sum := sha256.Sum256([]byte(req.RequestID))
	r := history.MeasurementRound{RoundID: fmt.Sprintf("manual-%x", sum[:16]), TriggerType: "manual", StartedAt: time.Now().UTC()}
	for _, n := range selections {
		r.Items = append(r.Items, history.MeasurementRoundItem{RequestID: req.RequestID, Project: "latency", ProfileID: n.ProfileID, NodeKey: n.NodeKey, NodeIdentityKey: n.NodeIdentityKey, ConfigRevisionKey: n.ConfigRevisionKey, DisplayName: n.DisplayName})
	}
	return s.historyStore.DB().CreateMeasurementRound(ctx, r)
}
