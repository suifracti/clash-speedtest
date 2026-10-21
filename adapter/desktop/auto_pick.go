package desktop

import (
	"fmt"

	"github.com/faceair/clash-speedtest/application"
	"github.com/faceair/clash-speedtest/core/autopick"
)

func (a *App) GetAutoPick() (*application.AutoPickStatus, error) {
	if a == nil || a.app == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	return a.app.GetAutoPickStatus()
}

func (a *App) SaveAutoPickConfig(cfg autopick.Config) (*application.AutoPickStatus, error) {
	if a == nil || a.app == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	return a.app.SaveAutoPickConfig(cfg)
}

func (a *App) RunAutoPickNow() (*application.AutoPickStatus, error) {
	if a == nil || a.app == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	return a.app.RunAutoPickNow()
}

func (a *App) ListAutoPickReports(limit int) ([]autopick.ReportMeta, error) {
	if a == nil || a.app == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	return a.app.ListAutoPickReports(limit)
}

func (a *App) GetAutoPickReport(reportID string) (*autopick.Report, error) {
	if a == nil || a.app == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	return a.app.GetAutoPickReport(reportID)
}
