package application

import (
	"context"
	"fmt"
	"time"

	"github.com/faceair/clash-speedtest/core/profiles"
	"github.com/faceair/clash-speedtest/core/subscriptionusage"
)

type SubscriptionUsageRefreshResult struct {
	Subscription SubscriptionDTO            `json:"subscription"`
	Observation  subscriptionusage.Snapshot `json:"observation"`
}

// RefreshAccountUsage reuses subscription acquisition and records one account.
// It leaves node cache and node/airport update times unchanged.
func (s *AppService) RefreshAccountUsage(ctx context.Context, airportID, subID, userAgent string) (*SubscriptionUsageRefreshResult, error) {
	if s == nil {
		return nil, fmt.Errorf("app service is not initialized")
	}
	load := func() (*profiles.Store, *profiles.Airport, *profiles.Subscription, error) {
		store, err := profiles.LoadStore(s.profilePaths.StoreFile())
		if err != nil {
			return nil, nil, nil, err
		}
		airport := store.Get(airportID)
		if airport == nil {
			return nil, nil, nil, fmt.Errorf("未找到指定机场")
		}
		sub := airport.GetSubscription(subID)
		if sub == nil {
			return nil, nil, nil, fmt.Errorf("未找到指定订阅")
		}
		return store, airport, sub, nil
	}
	s.profileWriteMu.Lock()
	_, airport, sub, err := load()
	if err == nil && !profiles.IsHTTPURL(sub.URL) {
		err = fmt.Errorf("本地配置不提供机场账户用量")
	}
	if err == nil {
		err = s.seedAccountUsageLocked(ctx, airport, sub)
	}
	var source string
	if err == nil {
		source = sub.URL
		userAgent = sub.EffectiveUserAgent(userAgent)
	}
	s.profileWriteMu.Unlock()
	if err != nil {
		return nil, err
	}
	body, usage, err := profiles.FetchSubscriptionWithUsageContext(ctx, source, userAgent)
	if err != nil {
		return nil, refreshFailure(err)
	}
	s.profileWriteMu.Lock()
	defer s.profileWriteMu.Unlock()
	store, airport, sub, err := load()
	if err != nil {
		return nil, err
	}
	sub.Usage = usage
	status := "ok"
	if usage == nil {
		status = "missing"
	}
	observation := usageSnapshotWithNotice(airport, sub, status, "account_usage_refresh", time.Now(), body)
	if err = s.saveUsageObservations(ctx, []subscriptionusage.Snapshot{observation}); err != nil {
		return nil, err
	}
	if err = profiles.SaveStore(s.profilePaths.StoreFile(), store); err != nil {
		return nil, err
	}
	hasCache := s.profilePaths.HasCache(sub.ID)
	nodeCount := 0
	if hasCache {
		nodeCount = s.CountCachedNodes(sub.ID)
	}
	return &SubscriptionUsageRefreshResult{Subscription: subscriptionDTO(airport.ID, sub, nodeCount, hasCache), Observation: observation}, nil
}
