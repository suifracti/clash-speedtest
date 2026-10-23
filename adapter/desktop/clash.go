package desktop

import (
	"github.com/faceair/clash-speedtest/application"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) GetClashNodeStatus(scope application.WorkbenchLatencyBatchSelection) (application.ClashNodeStatus, error) {
	return a.app.GetClashNodeStatus(a.context(), scope)
}
func (a *App) CompareClashNode(scope application.WorkbenchLatencyBatchSelection) (*application.ClashComparison, error) {
	return a.app.CompareClashNode(a.context(), scope)
}

func (a *App) GetClashUsage() []application.ClashUsage {
	if wailsruntime.WindowIsMinimised(a.context()) {
		return []application.ClashUsage{}
	}
	return a.app.GetClashUsage(a.context())
}
func (a *App) GetClashNodeGroups(scope application.WorkbenchLatencyBatchSelection) ([]application.ClashGroupOption, error) {
	return a.app.GetClashNodeGroups(a.context(), scope)
}
func (a *App) UseClashNode(req application.ClashSelectRequest) (*application.ClashSelectResult, error) {
	return a.app.UseClashNode(a.context(), req)
}
