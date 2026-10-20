package desktop

import (
	"fmt"
	"github.com/faceair/clash-speedtest/application"
)

func (a *App) SetSubscriptionUserAgent(airportID, subscriptionID, userAgent string) (*application.SubscriptionDTO, error) {
	if a == nil || a.app == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	return a.app.SetSubscriptionUserAgent(airportID, subscriptionID, userAgent)
}
