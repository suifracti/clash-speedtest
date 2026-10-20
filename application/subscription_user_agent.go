package application

import (
	"fmt"
	"strings"

	"github.com/faceair/clash-speedtest/core/profiles"
)

// SetSubscriptionUserAgent changes only the account request preference. The
// next refresh uses it; setting it neither downloads nodes nor invents an update.
func (s *AppService) SetSubscriptionUserAgent(airportID, subID, userAgent string) (*SubscriptionDTO, error) {
	if s == nil {
		return nil, fmt.Errorf("app service is not initialized")
	}
	userAgent = strings.TrimSpace(userAgent)
	if len(userAgent) > 512 {
		return nil, fmt.Errorf("User-Agent 不能超过 512 字节")
	}
	for _, c := range userAgent {
		if c < 32 || c == 127 {
			return nil, fmt.Errorf("User-Agent 不能包含控制字符")
		}
	}
	s.profileWriteMu.Lock()
	defer s.profileWriteMu.Unlock()
	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return nil, err
	}
	airport := store.Get(airportID)
	if airport == nil {
		return nil, fmt.Errorf("未找到指定机场")
	}
	sub := airport.GetSubscription(subID)
	if sub == nil {
		return nil, fmt.Errorf("未找到指定订阅")
	}
	sub.UserAgent = userAgent
	if err = profiles.SaveStore(s.profilePaths.StoreFile(), store); err != nil {
		return nil, err
	}
	dto := subscriptionDTO(airport.ID, sub, s.CountCachedNodes(sub.ID), s.profilePaths.HasCache(sub.ID))
	return &dto, nil
}
