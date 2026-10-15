package desktop

import (
	"context"
	"fmt"
	"github.com/faceair/clash-speedtest/application"
)

func (a *App) GetPeriodicSampling() (*application.PeriodicSamplingStatus, error) {
	if a == nil || a.app == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	return a.app.GetPeriodicSampling()
}
func (a *App) ConfigurePeriodicSampling(cfg application.PeriodicSamplingConfig) (*application.PeriodicSamplingStatus, error) {
	if a == nil || a.app == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	return a.app.ConfigurePeriodicSampling(cfg)
}

func (a *App) GetPeriodicServiceChecks(profile, node string) (application.PeriodicChecks, error) {
	if a == nil || a.app == nil {
		return application.PeriodicChecks{}, fmt.Errorf("应用服务未初始化")
	}
	return a.app.GetPeriodicServiceChecks(context.Background(), profile, node)
}
