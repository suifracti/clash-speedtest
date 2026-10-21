package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/faceair/clash-speedtest/core/autopick"
	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/publicservice"
)

// Auto-pick rounds are frozen in the same measurement_rounds tables as manual
// runs, so 节点历史区 can list them tagged 自动精选.

const autoPickRoundTrigger = "auto_pick"

// autoPickRoundContextKey marks the goroutine that may execute an auto-pick plan.
type autoPickRoundContextKey struct{}

type autoPickServicePlan struct {
	entry    *autopick.Node
	node     autoPickScopedNode
	purpose  string
	services []string
}

func autoPickServiceRequestID(reportID string, node autoPickScopedNode, serviceID string) string {
	sum := sha256.Sum256([]byte(node.ProfileID + "\x00" + node.Node.NodeKey + "\x00" + node.Node.NodeIdentityKey + "\x00" + node.Node.ConfigRevisionKey + "\x00" + serviceID))
	return fmt.Sprintf("auto-pick:%s:%x", reportID, sum[:8])
}

func autoPickServiceRoundID(reportID string) string {
	sum := sha256.Sum256([]byte(reportID))
	return fmt.Sprintf("auto-pick-svc-%x", sum[:16])
}

func autoPickServiceAttemptID(requestID string) string {
	sum := sha256.Sum256([]byte(requestID))
	return "auto-pick-svc-" + hex.EncodeToString(sum[:12])
}

func (s *AppService) autoPickHistoryDB() *history.DB {
	if s == nil || s.historyStore == nil {
		return nil
	}
	return s.historyStore.DB()
}

// autoPickStartServiceRound freezes every required (node, service) pair before
// the first request. A nil return means history is unavailable: the round still
// runs, it just leaves no history rows.
func (s *AppService) autoPickStartServiceRound(report *autopick.Report, plans []autoPickServicePlan) *history.MeasurementRound {
	db := s.autoPickHistoryDB()
	if db == nil || len(plans) == 0 {
		return nil
	}
	round := &history.MeasurementRound{
		RoundID:     autoPickServiceRoundID(report.ReportID),
		TriggerType: autoPickRoundTrigger,
		StartedAt:   report.CreatedAt,
	}
	for _, plan := range plans {
		for _, serviceID := range plan.services {
			round.Items = append(round.Items, history.MeasurementRoundItem{
				RequestID:         autoPickServiceRequestID(report.ReportID, plan.node, serviceID),
				Project:           "service",
				ServiceID:         serviceID,
				ProfileID:         plan.node.ProfileID,
				NodeKey:           plan.node.Node.NodeKey,
				NodeIdentityKey:   plan.node.Node.NodeIdentityKey,
				ConfigRevisionKey: plan.node.Node.ConfigRevisionKey,
				DisplayName:       plan.node.Node.DisplayName,
			})
		}
	}
	if len(round.Items) == 0 {
		return nil
	}
	if err := db.CreateMeasurementRound(context.Background(), *round); err != nil {
		report.Notes = append(report.Notes, "解锁结果未能写入历史："+err.Error())
		return nil
	}
	return round
}

func autoPickExecutionState(result publicservice.Result) string {
	state := "failed"
	if result.RequestCount > 0 {
		state = "completed"
	}
	if result.Details["execution_status"] == "not_executed" {
		state = "not_executed"
	}
	switch result.Outcome {
	case "matched", "profiled", "reachable", "challenge", "unlocked", "originals_only", "banned", "region_limited", "service_rejected", "region_blocked", "unknown", "auth_failed", "permission_denied", "setup_required", "rate_limited", "flapping":
		state = "completed"
	case "cancelled":
		state = "cancelled"
	}
	return state
}

// autoPickSaveServiceAttempt writes the conclusion the same way the workbench
// does: attempt → running → staged result → committed fact.
func (s *AppService) autoPickSaveServiceAttempt(ctx context.Context, report *autopick.Report, round *history.MeasurementRound, node autoPickScopedNode, serviceID string, rule publicservice.Rule, result publicservice.Result) {
	db := s.autoPickHistoryDB()
	if db == nil || round == nil {
		return
	}
	requestID := autoPickServiceRequestID(report.ReportID, node, serviceID)
	attempt := &history.PublicServiceAttempt{
		AttemptID:         autoPickServiceAttemptID(requestID),
		RequestID:         requestID,
		ProfileID:         node.ProfileID,
		NodeKey:           node.Node.NodeKey,
		NodeIdentityKey:   node.Node.NodeIdentityKey,
		ConfigRevisionKey: node.Node.ConfigRevisionKey,
		DisplayName:       node.Node.DisplayName,
		NodeType:          node.Node.Type,
		Source:            autoPickRoundTrigger,
		ServiceID:         serviceID,
		RequestedAt:       report.CreatedAt,
	}
	timeoutSeconds := int64(rule.TimeoutSeconds)
	if timeoutSeconds <= 0 {
		timeoutSeconds = int64(publicservice.DefaultTimeout.Seconds())
	}
	attempt.Rule = publicServiceRuleSnapshot(rule, timeoutSeconds)
	if err := db.CreatePublicServiceAttempt(ctx, attempt); err != nil {
		report.Notes = append(report.Notes, "解锁结果未能写入历史："+err.Error())
		return
	}
	startedAt := result.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	if err := db.BeginPublicServiceAttempt(ctx, attempt.AttemptID, startedAt); err != nil {
		report.Notes = append(report.Notes, "解锁结果未能写入历史："+err.Error())
		return
	}
	measurement := history.PublicServiceMeasurement{
		Model:        result.Model,
		RequestCount: result.RequestCount,
		Summary:      result.Summary,
		Details:      result.Details,
		Outcome:      result.Outcome,
		HTTPStatus:   result.HTTPStatus,
		BytesRead:    result.BytesRead,
		StartedAt:    result.StartedAt,
		FinishedAt:   result.FinishedAt,
		DurationMs:   result.DurationMs,
		FailurePhase: result.FailurePhase,
		ErrorMessage: result.ErrorMessage,
	}
	if err := db.StagePublicServiceResult(ctx, attempt.AttemptID, autoPickExecutionState(result), measurement); err != nil {
		report.Notes = append(report.Notes, "解锁结果未能写入历史："+err.Error())
		return
	}
	if err := db.CommitPublicServiceResult(ctx, attempt.AttemptID); err != nil {
		report.Notes = append(report.Notes, "解锁结果未能写入历史："+err.Error())
	}
}

func (s *AppService) autoPickFinishServiceRound(round *history.MeasurementRound) {
	db := s.autoPickHistoryDB()
	if db == nil || round == nil {
		return
	}
	_ = db.FinishMeasurementRound(context.Background(), round.RoundID, "finished")
}
