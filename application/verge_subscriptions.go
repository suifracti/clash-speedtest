package application

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/faceair/clash-speedtest/core/profiles"
)

// The selection contains local IDs only. Source URLs stay in the backend.
type ClashVergeImportRequest struct {
	UID            string `json:"uid"`
	AirportID      string `json:"airport_id,omitempty"`
	SubscriptionID string `json:"subscription_id,omitempty"`
}

type ClashVergeSubscriptionDTO struct {
	UID            string    `json:"uid"`
	Name           string    `json:"name"`
	UpdatedAt      time.Time `json:"updated_at"`
	AirportID      string    `json:"airport_id,omitempty"`
	SubscriptionID string    `json:"subscription_id,omitempty"`
	Linked         bool      `json:"linked"`
	URLChanged     bool      `json:"url_changed"`
}

func (s *AppService) ListClashVergeSubscriptions() ([]ClashVergeSubscriptionDTO, error) {
	if s == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	file, err := profiles.DefaultVergeProfilesFile()
	if err != nil {
		return nil, err
	}
	sources, err := profiles.LoadVergeProfiles(file)
	if err != nil {
		return nil, err
	}
	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return nil, err
	}
	result := make([]ClashVergeSubscriptionDTO, 0, len(sources))
	for _, source := range sources {
		dto := ClashVergeSubscriptionDTO{UID: source.UID, Name: source.Name, UpdatedAt: source.UpdatedAt}
		matches := 0
		for _, airport := range store.Airports {
			if airport == nil {
				continue
			}
			for _, sub := range airport.Subscriptions {
				if sub == nil {
					continue
				}
				if sub.VergeUID == source.UID {
					dto.AirportID, dto.SubscriptionID, dto.Linked = airport.ID, sub.ID, true
					dto.URLChanged = sub.URL != source.URL
				} else if sub.VergeUID == "" && sub.URL == source.URL && !dto.Linked {
					matches++
					dto.AirportID, dto.SubscriptionID = airport.ID, sub.ID
				}
			}
		}
		if !dto.Linked && matches != 1 {
			dto.AirportID, dto.SubscriptionID = "", ""
		}
		result = append(result, dto)
	}
	return result, nil
}

func (s *AppService) ImportClashVergeSubscription(ctx context.Context, request ClashVergeImportRequest) (*SubscriptionDTO, error) {
	if s == nil {
		return nil, fmt.Errorf("应用服务未初始化")
	}
	file, err := profiles.DefaultVergeProfilesFile()
	if err != nil {
		return nil, err
	}
	sources, err := profiles.LoadVergeProfiles(file)
	if err != nil {
		return nil, err
	}
	var source *profiles.VergeProfile
	for i := range sources {
		if sources[i].UID == request.UID {
			source = &sources[i]
			break
		}
	}
	if source == nil {
		return nil, fmt.Errorf("Clash Verge 中没有这个订阅，请重新读取列表")
	}
	body, err := profiles.ReadVergeProfileCache(file, *source)
	if err != nil {
		return nil, err
	}
	s.profileWriteMu.Lock()
	defer s.profileWriteMu.Unlock()
	store, err := profiles.LoadStore(s.profilePaths.StoreFile())
	if err != nil {
		return nil, err
	}
	var airport *profiles.Airport
	var sub *profiles.Subscription
	for _, candidate := range store.Airports {
		if candidate == nil {
			continue
		}
		for _, account := range candidate.Subscriptions {
			if account != nil && account.VergeUID == source.UID {
				airport, sub = candidate, account
			}
		}
	}
	if sub != nil {
		if request.SubscriptionID != "" && (request.AirportID != airport.ID || request.SubscriptionID != sub.ID) {
			return nil, fmt.Errorf("这个 Clash Verge 订阅已关联其他账户")
		}
	} else if request.AirportID != "" || request.SubscriptionID != "" {
		airport = store.Get(request.AirportID)
		if airport != nil {
			sub = airport.GetSubscription(request.SubscriptionID)
		}
		if sub == nil {
			return nil, fmt.Errorf("请选择有效的已有订阅账户")
		}
		if sub.VergeUID != "" {
			return nil, fmt.Errorf("这个账户已关联其他 Clash Verge 订阅")
		}
	}
	if err := s.seedSubscriptionUsageLocked(ctx, store); err != nil {
		return nil, err
	}
	previousStore, err := os.ReadFile(s.profilePaths.StoreFile())
	if err != nil {
		return nil, err
	}
	created := sub == nil
	if created {
		id := fmt.Sprintf("%d", time.Now().UnixNano())
		name := source.Name
		if name == "" {
			name = "Clash Verge 订阅"
		}
		sub = &profiles.Subscription{ID: id, Name: name}
		airport = &profiles.Airport{ID: id, Name: name, Subscriptions: []*profiles.Subscription{sub}}
		store.Add(airport)
	}
	if sub.URL != source.URL {
		sub.Usage = nil
	}
	sub.URL, sub.VergeUID = source.URL, source.UID
	if created || len(airport.Subscriptions) == 1 {
		airport.URL = source.URL
	}
	if err := profiles.SaveStore(s.profilePaths.StoreFile(), store); err != nil {
		return nil, err
	}
	fetched := &subscriptionFetchResult{path: "verge", body: body, usage: source.Usage, updatedAt: sub.UpdatedAt, userAgent: sub.UserAgent}
	result, err := s.refreshSubscriptionWithFetchLocked(ctx, airport.ID, sub.ID, "", "", nil, fetched)
	if err != nil {
		if restoreErr := s.restoreRefreshStoreBytes(previousStore); restoreErr != nil {
			return nil, fmt.Errorf("导入失败且原订阅恢复失败：%w", restoreErr)
		}
		if created {
			_ = os.Remove(s.profilePaths.CacheFile(sub.ID))
		}
		return nil, err
	}
	return result, nil
}
