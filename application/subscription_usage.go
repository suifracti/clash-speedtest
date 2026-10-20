package application

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/faceair/clash-speedtest/core/profiles"
	"github.com/faceair/clash-speedtest/core/speedtester"
	"github.com/faceair/clash-speedtest/core/subscriptionusage"
)

var noticeRemainingValue = regexp.MustCompile(`(?i)(?:距离下次重置剩余|剩余(?:可用)?(?:流量)?|可用流量)[：:\s]*([0-9]+(?:\.[0-9]+)?)\s*(PiB|PB|TiB|TB|GiB|GB|MiB|MB|KiB|KB|B)\b`)

var errUsageRefreshPanic = errors.New("订阅用量更新出现内部异常；请查看账户状态并重试")

func fillDefaultSubscriptionUsage(settings *AppSettings) {
	if settings == nil {
		return
	}
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
	var airportID, airportName, subID, subName, subURL string
	if ap != nil {
		airportID = ap.ID
		airportName = ap.Name
	}
	if sub != nil {
		subID = sub.ID
		subName = sub.Name
		subURL = sub.URL
	}
	snap := subscriptionusage.Snapshot{
		AccountKey:       usageAccountKey(subURL),
		AirportID:        airportID,
		AirportName:      airportName,
		SubscriptionID:   subID,
		SubscriptionName: subName,
		CapturedAt:       at.UTC(),
		Status:           status,
		Source:           source,
	}
	if status == "ok" && sub != nil && sub.Usage != nil {
		snap.Upload = sub.Usage.Upload
		snap.Download = sub.Usage.Download
		snap.Total = sub.Usage.Total
		snap.CounterKind = "subscription_header"
		if _, known := sub.Usage.UsedBytes(); !known {
			snap.Status = "partial"
			snap.Message = "机场仅返回部分用量字段；缺失的计数未知，不计为零用量"
			if sub.Usage.Upload == nil && sub.Usage.Download == nil && sub.Usage.Total == nil && sub.Usage.Expire == nil {
				snap.Status = "missing"
			}
		}
	}
	if snap.Status == "missing" {
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

func noticeRemainingBytes(body []byte) (int64, bool) {
	names, err := speedtester.SubscriptionNoticeNames(body)
	if err != nil {
		return 0, false
	}
	for _, name := range names {
		match := noticeRemainingValue.FindStringSubmatch(name)
		if len(match) != 3 {
			continue
		}
		value, err := strconv.ParseFloat(match[1], 64)
		if err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		unit := strings.ToUpper(match[2])
		power := 0
		switch unit[:1] {
		case "K":
			power = 1
		case "M":
			power = 2
		case "G":
			power = 3
		case "T":
			power = 4
		case "P":
			power = 5
		}
		bytes := math.Round(value * math.Pow(1024, float64(power)))
		if bytes <= 9007199254740991 {
			return int64(bytes), true
		}
	}
	return 0, false
}

func (s *AppService) seedSubscriptionUsageLocked(ctx context.Context, store *profiles.Store) error {
	if s == nil || s.historyStore == nil || store == nil {
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
	if s == nil {
		return fmt.Errorf("app service is not initialized")
	}
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
	if s == nil {
		return nil, fmt.Errorf("app service is not initialized")
	}
	return s.collectSubscriptionUsage(ctx, time.Now(), userAgent, true)
}

func (s *AppService) collectSubscriptionUsage(ctx context.Context, now time.Time, userAgent string, force bool) (result *subscriptionusage.RefreshState, retErr error) {
	if s == nil {
		return nil, fmt.Errorf("app service is not initialized")
	}
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
	dailyEnabled := true
	if settings != nil && settings.SubscriptionDailyUpdateEnabled != nil {
		dailyEnabled = *settings.SubscriptionDailyUpdateEnabled
	}
	if !force && !dailyEnabled {
		return s.historyStore.SubscriptionRefreshState(ctx)
	}
	claimed, err := s.historyStore.ClaimSubscriptionRefresh(ctx, now, subscriptionusage.NextDaily(now), force)
	if err != nil {
		return nil, err
	}
	if !claimed {
		return s.historyStore.SubscriptionRefreshState(ctx)
	}
	state, message := "failed", "用量更新未完成；请手动重试"
	defer func() {
		if err := ctx.Err(); err != nil {
			state, message = "cancelled", "用量更新已取消；已保存的观测保留"
			if !errors.Is(retErr, err) {
				retErr = errors.Join(retErr, err)
			}
		}
		// A cancelled request must still persist a terminal claim state.
		finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := s.historyStore.FinishSubscriptionRefresh(finishCtx, state, message); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("保存用量更新终态失败：%w", err))
			return
		}
		var err error
		result, err = s.historyStore.SubscriptionRefreshState(finishCtx)
		retErr = errors.Join(retErr, err)
	}()
	s.profileWriteMu.Lock()
	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	s.profileWriteMu.Unlock()
	if err != nil {
		message = "无法读取订阅来源，请手动重试"
		return nil, err
	}
	// Serialize each origin (including its 429 cooldown), while allowing up to
	// four independent providers to fetch concurrently. Only IDs/hashes queue.
	groups := map[string][]usageRefreshSource{}
	var origins []string
	for _, ap := range store.Airports {
		if ap == nil {
			continue
		}
		for _, sub := range ap.Subscriptions {
			if sub == nil {
				continue
			}
			origin := profiles.SourceOrigin(sub.URL)
			if _, exists := groups[origin]; !exists {
				origins = append(origins, origin)
			}
			groups[origin] = append(groups[origin], usageRefreshSource{RefreshSelection: RefreshSelection{AirportID: ap.ID, SubscriptionID: sub.ID}, fingerprint: refreshSourceFingerprint(sub.URL)})
		}
	}
	jobs := make(chan []usageRefreshSource, len(origins))
	for _, origin := range origins {
		jobs <- groups[origin]
	}
	close(jobs)
	var processed, failed atomic.Int64
	var panicked atomic.Bool
	var workers sync.WaitGroup
	for i := 0; i < min(4, len(origins)); i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for group := range jobs {
				for _, source := range group {
					if ctx.Err() != nil {
						return
					}
					err := s.refreshUsageSource(ctx, source, userAgent)
					processed.Add(1)
					if err != nil {
						failed.Add(1)
					}
					if errors.Is(err, errUsageRefreshPanic) {
						panicked.Store(true)
					}
				}
			}
		}()
	}
	workers.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if panicked.Load() {
		message = errUsageRefreshPanic.Error()
		return nil, errUsageRefreshPanic
	}
	state, message = "completed", fmt.Sprintf("已更新 %d 个订阅并记录用量；不提供用量的订阅保持未知", processed.Load())
	if failed.Load() > 0 {
		state = "completed_with_issues"
		message = fmt.Sprintf("已尝试 %d 个订阅，%d 个更新或保存异常；请查看观测记录并重试", processed.Load(), failed.Load())
	}
	return nil, nil
}

type usageRefreshSource struct {
	RefreshSelection
	fingerprint string
}

func (s *AppService) refreshUsageSource(ctx context.Context, source usageRefreshSource, userAgent string) (retErr error) {
	defer func() {
		if recover() != nil {
			retErr = errUsageRefreshPanic
		}
	}()
	var rawURL string
	fetched := &subscriptionFetchResult{}
	err := func() error {
		s.profileWriteMu.Lock()
		defer s.profileWriteMu.Unlock()
		store, err := profiles.LoadStore(s.profilePaths.StoreFile())
		if err != nil {
			return err
		}
		ap := store.Get(source.AirportID)
		if ap == nil {
			return &refreshError{Code: "configuration_changed", Message: "机场配置已变化；未发出请求"}
		}
		sub := ap.GetSubscription(source.SubscriptionID)
		if sub == nil || strings.TrimSpace(sub.URL) == "" || refreshSourceFingerprint(sub.URL) != source.fingerprint {
			return &refreshError{Code: "configuration_changed", Message: "订阅配置已变化；未发出请求"}
		}
		if err := refreshSourceCooldown(store, sub.URL, time.Now()); err != nil {
			return err
		}
		if err := s.seedSubscriptionUsageLocked(ctx, store); err != nil {
			return err
		}
		rawURL, fetched.updatedAt, fetched.userAgent = sub.URL, sub.UpdatedAt, sub.UserAgent
		userAgent = sub.EffectiveUserAgent(userAgent)
		return nil
	}()
	if err != nil {
		return err
	}
	fetched.body, fetched.usage, fetched.err = fetchSubscriptionSource(ctx, rawURL, userAgent)
	if err := ctx.Err(); err != nil {
		return err
	}
	s.profileWriteMu.Lock()
	defer s.profileWriteMu.Unlock()
	_, err = s.refreshSubscriptionWithFetchLocked(ctx, source.AirportID, source.SubscriptionID, userAgent, source.fingerprint, nil, fetched)
	return err
}

func (s *AppService) GetSubscriptionUsage(ctx context.Context, from, until, period, account string) (*subscriptionusage.Report, error) {
	if s == nil {
		return nil, fmt.Errorf("app service is not initialized")
	}
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
	dailyEnabled := true
	if settings != nil && settings.SubscriptionDailyUpdateEnabled != nil {
		dailyEnabled = *settings.SubscriptionDailyUpdateEnabled
	}
	report.DailyEnabled = dailyEnabled
	return &report, nil
}
