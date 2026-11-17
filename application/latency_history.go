package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/faceair/clash-speedtest/core/history"
	"github.com/faceair/clash-speedtest/core/monitor"
)

type LatencyHistoryScopesQuery struct {
	ProfileID   string     `json:"profile_id"`
	DisplayName string     `json:"display_name"`
	Since       *time.Time `json:"since"`
	Until       *time.Time `json:"until"`
}

func (s *AppService) ListLatencyHistoryScopes(ctx context.Context, query LatencyHistoryScopesQuery) ([]history.LatencyHistoryScope, error) {
	if s == nil || s.historyStore == nil {
		return nil, fmt.Errorf("history store is not initialized")
	}
	query.ProfileID = strings.TrimSpace(query.ProfileID)
	if query.ProfileID == "" || strings.TrimSpace(query.DisplayName) == "" {
		return nil, monitor.NewValidationError("延迟历史查询必须提供订阅 profile_id 和节点显示名")
	}
	if query.Since == nil || query.Until == nil || query.Since.IsZero() || query.Until.IsZero() || !query.Since.Before(*query.Until) {
		return nil, monitor.NewValidationError("延迟历史查询必须提供有效的起止时间")
	}
	return s.historyStore.DB().ListLatencyHistoryScopes(ctx, query.ProfileID, query.DisplayName, *query.Since, *query.Until)
}
