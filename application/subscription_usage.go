package application

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/faceair/clash-speedtest/core/profiles"
	"github.com/faceair/clash-speedtest/core/subscriptionusage"
)

func fillDefaultSubscriptionUsage(settings *AppSettings) {
	if settings.SubscriptionDailyUpdateEnabled == nil {
		enabled := true
		settings.SubscriptionDailyUpdateEnabled = &enabled
	}
}

// URLs are account identities, but never stored in usage history or exposed in
// its API. Identical source addresses share counters across duplicate imports.
func usageAccountKey(source string) string {
	return fmt.Sprintf("usage_%x", sha256.Sum256([]byte(strings.TrimSpace(source))))
}

func usageSnapshot(ap *profiles.Airport, sub *profiles.Subscription, status, source string, at time.Time) subscriptionusage.Snapshot {
	snap := subscriptionusage.Snapshot{AccountKey: usageAccountKey(sub.URL), AirportID: ap.ID, AirportName: ap.Name, SubscriptionID: sub.ID, SubscriptionName: sub.Name, CapturedAt: at.UTC(), Status: status, Source: source}
	if status == "ok" && sub.Usage != nil {
		up, down := sub.Usage.Upload, sub.Usage.Download
		snap.Upload = &up
		snap.Download = &down
		snap.Total = sub.Usage.Total
	}
	if status == "missing" {
		snap.Message = "机场未返回有效用量字段；不计为零用量"
	}
	if status == "refresh_failed" {
		snap.Message = "订阅更新失败；上次缓存保留，本次用量未知"
	}
	if status == "local_file" {
		snap.Message = "本地配置不提供机场账户用量"
	}
	return snap
}

func (s *AppService) seedSubscriptionUsageLocked(ctx context.Context, store *profiles.Store) error {
	if s.historyStore == nil {
		return nil
	}
	for _, ap := range store.Airports {
		if ap == nil {
			continue
		}
		for _, sub := range ap.Subscriptions {
			if sub == nil || sub.Usage == nil || sub.Usage.UpdatedAt.IsZero() || !profiles.IsHTTPURL(sub.URL) {
				continue
			}
			exists, err := s.historyStore.HasSubscriptionUsage(ctx, usageAccountKey(sub.URL))
			if err != nil {
				return err
			}
			if !exists {
				if err := s.historyStore.SaveSubscriptionUsage(ctx, usageSnapshot(ap, sub, "ok", "cached_baseline", sub.Usage.UpdatedAt)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *AppService) saveUsageObservations(ctx context.Context, snapshots []subscriptionusage.Snapshot) error {
	if s.subscriptionRefreshUsageSaveHook != nil {
		return s.subscriptionRefreshUsageSaveHook(ctx, snapshots)
	}
	if s.historyStore == nil {
		return nil
	}
	for _, snap := range snapshots {
		if err := s.historyStore.SaveSubscriptionUsage(ctx, snap); err != nil {
			return fmt.Errorf("订阅已更新，但用量快照保存失败，请重试：%w", err)
		}
	}
	return nil
}

// UpdateSubscriptionUsage refreshes subscriptions first; those refreshes save
// both node cache and counter snapshots. It does not trigger node probes.
func (s *AppService) UpdateSubscriptionUsage(ctx context.Context, userAgent string) (*subscriptionusage.RefreshState, error) {
	return s.collectSubscriptionUsage(ctx, time.Now(), userAgent, true)
}

func (s *AppService) collectSubscriptionUsage(ctx context.Context, now time.Time, userAgent string, force bool) (*subscriptionusage.RefreshState, error) {
	if !s.usageRefreshMu.TryLock() {
		return nil, fmt.Errorf("订阅用量更新正在执行，请稍后查看")
	}
	defer s.usageRefreshMu.Unlock()
	if s.historyStore == nil {
		return nil, fmt.Errorf("用量历史存储尚未初始化")
	}
	settings, err := s.GetSettings()
	if err != nil {
		return nil, err
	}
	if !force && !*settings.SubscriptionDailyUpdateEnabled {
		return s.historyStore.SubscriptionRefreshState(ctx)
	}
	claimed, err := s.historyStore.ClaimSubscriptionRefresh(ctx, now, subscriptionusage.NextDaily(now), force)
	if err != nil {
		return nil, err
	}
	if !claimed {
		return s.historyStore.SubscriptionRefreshState(ctx)
	}
	s.profileWriteMu.Lock()
	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	s.profileWriteMu.Unlock()
	if err != nil {
		_ = s.historyStore.FinishSubscriptionRefresh(ctx, "failed", "无法读取订阅来源，请手动重试")
		return nil, err
	}
	failed, processed := 0, 0
	for _, ap := range store.Airports {
		if ap == nil {
			continue
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		s.profileWriteMu.Lock()
		_, refreshErr := s.refreshAirportLocked(ctx, ap.ID, userAgent)
		s.profileWriteMu.Unlock()
		processed++
		if refreshErr != nil {
			failed++
		}
	}
	state, message := "completed", fmt.Sprintf("已更新 %d 个机场并记录用量；不提供用量的订阅保持未知", processed)
	if failed > 0 {
		state = "completed_with_issues"
		message = fmt.Sprintf("已尝试 %d 个机场，%d 个更新或保存异常；请查看观测记录并重试", processed, failed)
	}
	if err := s.historyStore.FinishSubscriptionRefresh(ctx, state, message); err != nil {
		return nil, err
	}
	return s.historyStore.SubscriptionRefreshState(ctx)
}

func (s *AppService) GetSubscriptionUsage(ctx context.Context, from, until, period, account string) (*subscriptionusage.Report, error) {
	a, b, err := subscriptionusage.ParseRange(from, until, period)
	if err != nil {
		return nil, err
	}
	if s.historyStore == nil {
		return nil, fmt.Errorf("用量历史存储尚未初始化")
	}
	// The first hour of the following day can close the last daily bucket.
	snapshots, err := s.historyStore.QuerySubscriptionUsage(ctx, a, b.Add(time.Hour))
	if err != nil {
		return nil, err
	}
	if account != "" {
		filtered := snapshots[:0]
		for _, snap := range snapshots {
			if snap.AccountKey == account {
				filtered = append(filtered, snap)
			}
		}
		snapshots = filtered
	}
	report := subscriptionusage.Build(snapshots, a, b, period)
	report.Refresh, err = s.historyStore.SubscriptionRefreshState(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := s.GetSettings()
	if err != nil {
		return nil, err
	}
	report.DailyEnabled = *settings.SubscriptionDailyUpdateEnabled
	return &report, nil
}
