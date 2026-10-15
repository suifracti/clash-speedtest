package desktop

import (
	"context"
	"fmt"
	"github.com/faceair/clash-speedtest/core/history"
)

func (a *App) ListMeasurementRoundNodes(profile, node, nid, rev string, limit int) ([]history.MeasurementRound, error) {
	if a == nil || a.app == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	return a.app.ListMeasurementRoundNodes(context.Background(), profile, node, nid, rev, limit)
}
func (a *App) CreateManualMeasurementRound(r history.MeasurementRound) error {
	if a == nil || a.app == nil {
		return fmt.Errorf("应用服务未初始化")
	}
	return a.app.CreateManualMeasurementRound(context.Background(), r)
}
func (a *App) FinishManualMeasurementRound(id, state string) error {
	if a == nil || a.app == nil {
		return fmt.Errorf("应用服务未初始化")
	}
	return a.app.FinishManualMeasurementRound(context.Background(), id, state)
}
