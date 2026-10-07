package desktop

import (
	"context"
	"github.com/faceair/clash-speedtest/application"
)

func (a *App) GetPeriodicSampling() (*application.PeriodicSamplingStatus, error) {
	return a.app.GetPeriodicSampling()
}
func (a *App) ConfigurePeriodicSampling(cfg application.PeriodicSamplingConfig) (*application.PeriodicSamplingStatus, error) {
	return a.app.ConfigurePeriodicSampling(cfg)
}

func (a *App) GetPeriodicServiceChecks(profile, node string) (application.PeriodicChecks, error) {
	return a.app.GetPeriodicServiceChecks(context.Background(), profile, node)
}
