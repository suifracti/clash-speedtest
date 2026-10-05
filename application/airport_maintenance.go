package application

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/faceair/clash-speedtest/core/profiles"
)

func (s *AppService) SaveAirportMaintenance(id string, next profiles.AirportMaintenance) error {
	if next.RefreshHours != 0 && (next.RefreshHours < 1 || next.RefreshHours > 720) {
		return fmt.Errorf("刷新间隔须为 1–720 小时，或关闭")
	}
	if len(next.Links) > 20 {
		return fmt.Errorf("每个机场最多保存 20 个入口")
	}
	for i := range next.Links {
		link := &next.Links[i]
		link.Label = strings.TrimSpace(link.Label)
		link.URL = strings.TrimSpace(link.URL)
		u, err := url.Parse(link.URL)
		if link.Label == "" || len(link.Label) > 100 || len(link.URL) > 2048 || err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
			return fmt.Errorf("入口需要名称和有效的 http/https 地址")
		}
		if link.MonthlyDay < 0 || link.MonthlyDay > 28 {
			return fmt.Errorf("每月提醒日须为 1–28，0 表示仅快捷入口")
		}
		if link.DoneMonth != "" {
			if _, err := time.Parse("2006-01", link.DoneMonth); err != nil {
				return fmt.Errorf("完成月份无效")
			}
		}
	}
	s.profileWriteMu.Lock()
	defer s.profileWriteMu.Unlock()
	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return err
	}
	ap := store.Get(id)
	if ap == nil {
		return fmt.Errorf("机场不存在")
	}
	old := ap.Maintenance
	next.NextRefresh, next.LastAttempt, next.LastResult = old.NextRefresh, old.LastAttempt, old.LastResult
	if next.RefreshHours != old.RefreshHours || next.NextRefresh.IsZero() {
		next.NextRefresh = time.Time{}
		if next.RefreshHours > 0 {
			next.NextRefresh = time.Now().Add(time.Duration(next.RefreshHours) * time.Hour)
		}
	}
	ap.Maintenance = next
	return profiles.SaveStore(s.profilePaths.StoreFile(), store)
}

// One worker, no catch-up queue. Claim is persisted before any network operation.
func (s *AppService) StartSubscriptionRefresh(userAgent string) {
	s.maintenanceOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		s.maintenanceCancel = cancel
		s.maintenanceWG.Add(1)
		go func() {
			defer s.maintenanceWG.Done()
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			if s.historyStore != nil {
				if _, err := s.collectSubscriptionUsage(ctx, time.Now(), userAgent, false); err != nil && ctx.Err() == nil {
					log.Print("每日订阅更新或用量记录无法完成，请在用量监测中重试")
				}
			}
			for {
				select {
				case <-ctx.Done():
					return
				case now := <-ticker.C:
					if s.historyStore != nil {
						if _, err := s.collectSubscriptionUsage(ctx, now, userAgent, false); err != nil && ctx.Err() == nil {
							log.Print("每日订阅更新或用量记录无法完成，请在用量监测中重试")
						}
					}
					if err := s.refreshDueAirport(ctx, now, userAgent); err != nil {
						log.Print("订阅定时刷新无法执行或保存，请检查机场存储")
					}
				}
			}
		}()
	})
}

func (s *AppService) refreshDueAirport(ctx context.Context, now time.Time, userAgent string) error {
	s.profileWriteMu.Lock()
	defer s.profileWriteMu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return err
	}
	var due *profiles.Airport
	for _, ap := range store.Airports {
		if ap == nil {
			continue
		}
		if ap.Maintenance.RefreshHours > 0 && !ap.Maintenance.NextRefresh.IsZero() && !now.Before(ap.Maintenance.NextRefresh) && (due == nil || ap.Maintenance.NextRefresh.Before(due.Maintenance.NextRefresh)) {
			due = ap
		}
	}
	if due == nil {
		return nil
	}
	due.Maintenance.NextRefresh = now.Add(time.Duration(due.Maintenance.RefreshHours) * time.Hour)
	due.Maintenance.LastAttempt = now
	due.Maintenance.LastResult = "刷新已开始；若程序中断，结果待确认"
	if err := profiles.SaveStore(s.profilePaths.StoreFile(), store); err != nil {
		return err
	}
	_, refreshErr := s.refreshAirportLocked(ctx, due.ID, userAgent)
	store, err = profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return err
	}
	ap := store.Get(due.ID)
	if ap == nil {
		return nil
	}
	ap.Maintenance.LastResult = "定时刷新完成"
	if refreshErr != nil {
		ap.Maintenance.LastResult = "刷新失败，请检查订阅后手动重试；下一周期仍会尝试"
	}
	return profiles.SaveStore(s.profilePaths.StoreFile(), store)
}
