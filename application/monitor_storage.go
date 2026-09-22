package application

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/faceair/clash-speedtest/core/monitor"
)

func (s *AppService) canonicalMonitorHistory() error {
	if s.historyStore == nil {
		return fmt.Errorf("history store is not initialized")
	}
	if !s.appPaths.Explicit && s.appPaths.HistoryDir != "" && filepath.Clean(s.historyStore.Dir()) != filepath.Clean(s.appPaths.HistoryDir) {
		return fmt.Errorf("Monitor history is still using the legacy migration source; finish canonical data migration before retention")
	}
	return nil
}

func (s *AppService) GetMonitorStorageUsage() (*monitor.StorageUsage, error) {
	if err := s.canonicalMonitorHistory(); err != nil {
		return nil, err
	}
	usage, err := s.historyStore.StorageUsage(s.monitorStorageWarningBytes, s.monitorStorageHardBytes)
	if err != nil {
		return nil, err
	}
	return &usage, nil
}

func (s *AppService) monitorStorageGuard() string {
	usage, err := s.GetMonitorStorageUsage()
	if err != nil {
		return fmt.Sprintf("无法核对 Monitor 存储空间：%v", err)
	}
	if usage.Protected {
		return "Monitor 历史数据库达到产品保护阈值；后台采集已暂停，请清理历史或释放空间"
	}
	return ""
}

func (s *AppService) PreviewMonitorRetention(ctx context.Context, req monitor.RetentionRequest) (*monitor.RetentionPreview, error) {
	if err := s.canonicalMonitorHistory(); err != nil {
		return nil, err
	}
	preview, err := s.historyStore.PreviewRetention(ctx, req)
	if err != nil {
		return nil, err
	}
	usage, err := s.GetMonitorStorageUsage()
	if err != nil {
		return nil, err
	}
	preview.Storage = *usage
	return preview, nil
}
