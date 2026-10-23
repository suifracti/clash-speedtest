package desktop

import "github.com/faceair/clash-speedtest/application"

func (a *App) GetClashNodeStatus(scope application.WorkbenchLatencyBatchSelection) (application.ClashNodeStatus, error) {
	return a.app.GetClashNodeStatus(a.context(), scope)
}
func (a *App) CompareClashNode(scope application.WorkbenchLatencyBatchSelection) (*application.ClashComparison, error) {
	return a.app.CompareClashNode(a.context(), scope)
}
