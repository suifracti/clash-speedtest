package desktop

import (
	"fmt"
	"github.com/faceair/clash-speedtest/application"
)

func (a *App) RefreshSubscriptionUsage(airportID, subscriptionID string) (*application.SubscriptionUsageRefreshResult, error) {
	if a == nil || a.app == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	return a.app.RefreshAccountUsage(a.context(), airportID, subscriptionID, a.userAgent)
}
