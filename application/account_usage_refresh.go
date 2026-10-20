package application

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/faceair/clash-speedtest/core/profiles"
	"github.com/faceair/clash-speedtest/core/subscriptionusage"
)

type SubscriptionUsageRefreshResult struct {
	Subscription SubscriptionDTO            `json:"subscription"`
	Observation  subscriptionusage.Snapshot `json:"observation"`
}

func sameAccountUsageVersion(before, after *profiles.Subscription) bool {
	if before == nil || after == nil || before.URL != after.URL || before.UserAgent != after.UserAgent || !before.UpdatedAt.Equal(after.UpdatedAt) || (before.Usage == nil) != (after.Usage == nil) {
		return false
	}
	return before.Usage == nil || before.Usage.UpdatedAt.Equal(after.Usage.UpdatedAt)
}

// RefreshAccountUsage records one account without changing node content or its
// update time. The usage collector lock prevents duplicate manual/daily work;
// network IO is outside the profile lock so configuration edits remain usable.
func (s *AppService) RefreshAccountUsage(ctx context.Context, airportID, subID, userAgent string) (*SubscriptionUsageRefreshResult, error) {
	if s == nil {
		return nil, fmt.Errorf("app service is not initialized")
	}
	if s.historyStore == nil {
		return nil, fmt.Errorf("用量历史存储尚未初始化")
	}
	if !s.usageRefreshMu.TryLock() {
		return nil, &refreshError{Code: "busy", Message: "订阅用量更新正在执行，请稍后重试"}
	}
	defer s.usageRefreshMu.Unlock()
	var before profiles.Subscription
	if err := func() error {
		s.profileWriteMu.Lock()
		defer s.profileWriteMu.Unlock()
		if err := ctx.Err(); err != nil {
			return err
		}
		store, err := profiles.LoadStore(s.profilePaths.StoreFile())
		if err != nil {
			return err
		}
		airport := store.Get(airportID)
		if airport == nil {
			return fmt.Errorf("未找到指定机场")
		}
		sub := airport.GetSubscription(subID)
		if sub == nil {
			return fmt.Errorf("未找到指定订阅")
		}
		if !profiles.IsHTTPURL(sub.URL) {
			return fmt.Errorf("本地配置不提供机场账户用量")
		}
		if err = refreshSourceCooldown(store, sub.URL, time.Now()); err != nil {
			return err
		}
		if err = s.seedAccountUsageLocked(ctx, airport, sub); err != nil {
			return err
		}
		before = *sub
		userAgent = sub.EffectiveUserAgent(userAgent)
		return nil
	}(); err != nil {
		return nil, err
	}
	body, usage, fetchErr := profiles.FetchSubscriptionForUsageContext(ctx, before.URL, userAgent)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.profileWriteMu.Lock()
	defer s.profileWriteMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return nil, err
	}
	airport := store.Get(airportID)
	if airport == nil || !sameAccountUsageVersion(&before, airport.GetSubscription(subID)) {
		return nil, &refreshError{Code: "configuration_changed", Message: "抓取期间订阅配置或用量已有更新；旧响应未写入，请重新刷新用量"}
	}
	sub := airport.GetSubscription(subID)
	original, err := os.ReadFile(s.profilePaths.StoreFile())
	if err != nil {
		return nil, fmt.Errorf("读取订阅存储失败")
	}
	var observation subscriptionusage.Snapshot
	changed := false
	if fetchErr != nil {
		observation = usageSnapshot(airport, sub, "refresh_failed", "account_usage_refresh", time.Now())
		var httpErr *profiles.FetchHTTPError
		if errors.As(fetchErr, &httpErr) && httpErr.Status == http.StatusTooManyRequests {
			failure := refreshFailure(fetchErr)
			sub.LastFailureAt = time.Now()
			sub.LastFailureCode = failure.Code
			sub.LastFailureMessage = failure.Message
			sub.LastFailureRetryAt = httpErr.RetryAfterUntil
			changed = true
		}
	} else {
		sub.Usage = usage
		changed = true
		status := "ok"
		if usage == nil {
			status = "missing"
		}
		observation = usageSnapshotWithNotice(airport, sub, status, "account_usage_refresh", time.Now(), body)
	}
	rollback := func(message string) error {
		if !changed {
			return &refreshError{Code: "persist", Message: message}
		}
		return refreshRollbackError(message, s.restoreRefreshStoreBytes(original))
	}
	if changed {
		if err = s.saveSubscriptionRefreshStore(store); err != nil {
			return nil, rollback("用量保存失败；订阅状态已恢复")
		}
	}
	// Once profile state commits, finish or roll back the paired history write
	// even if the requesting view is closed at this boundary.
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err = s.saveUsageObservations(saveCtx, []subscriptionusage.Snapshot{observation}); err != nil {
		return nil, rollback("用量历史保存失败；订阅状态已恢复")
	}
	if fetchErr != nil {
		return nil, refreshFailure(fetchErr)
	}
	hasCache := s.profilePaths.HasCache(sub.ID)
	nodeCount := 0
	if hasCache {
		nodeCount = s.CountCachedNodes(sub.ID)
	}
	return &SubscriptionUsageRefreshResult{Subscription: subscriptionDTO(airport.ID, sub, nodeCount, hasCache), Observation: observation}, nil
}
