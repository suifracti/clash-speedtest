package desktop

import "github.com/faceair/clash-speedtest/application"

func (a *App) ListSubscriptionRefreshJobs() ([]*application.SubscriptionRefreshJob, error) {
	return a.app.ListSubscriptionRefreshJobs()
}

func (a *App) StartSubscriptionRefreshJob(request application.SubscriptionRefreshRequest) (*application.SubscriptionRefreshJob, error) {
	return a.app.StartSubscriptionRefreshJob(request, a.userAgent)
}

func (a *App) GetSubscriptionRefreshJob(id string) (*application.SubscriptionRefreshJob, error) {
	return a.app.GetSubscriptionRefresh(id)
}

func (a *App) CancelSubscriptionRefreshJob(id string) (*application.SubscriptionRefreshJob, error) {
	return a.app.CancelSubscriptionRefresh(id)
}
