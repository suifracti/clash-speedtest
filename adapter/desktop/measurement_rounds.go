package desktop

import (
	"context"
	"github.com/faceair/clash-speedtest/core/history"
)

func (a *App) ListMeasurementRoundNodes(profile, node, nid, rev string, limit int) ([]history.MeasurementRound, error) {
	return a.app.ListMeasurementRoundNodes(context.Background(), profile, node, nid, rev, limit)
}
func (a *App) CreateManualMeasurementRound(r history.MeasurementRound) error {
	return a.app.CreateManualMeasurementRound(context.Background(), r)
}
func (a *App) FinishManualMeasurementRound(id, state string) error {
	return a.app.FinishManualMeasurementRound(context.Background(), id, state)
}
