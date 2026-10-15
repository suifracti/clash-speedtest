package desktop

import (
	"fmt"

	"github.com/faceair/clash-speedtest/application"
)

func (a *App) ListSubscriptionRefreshJobs() ([]*application.SubscriptionRefreshJob, error) {
	if a == nil || a.app == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	return a.app.ListSubscriptionRefreshJobs()
}

func (a *App) StartSubscriptionRefreshJob(request application.SubscriptionRefreshRequest) (*application.SubscriptionRefreshJob, error) {
	if a == nil || a.app == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	return a.app.StartSubscriptionRefreshJob(request, a.userAgent)
}

func (a *App) GetSubscriptionRefreshJob(id string) (*application.SubscriptionRefreshJob, error) {
	if a == nil || a.app == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	return a.app.GetSubscriptionRefresh(id)
}

func (a *App) CancelSubscriptionRefreshJob(id string) (*application.SubscriptionRefreshJob, error) {
	if a == nil || a.app == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	return a.app.CancelSubscriptionRefresh(id)
}
