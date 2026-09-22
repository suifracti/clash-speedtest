package application

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/faceair/clash-speedtest/core/monitor"
)

const (
	defaultMonitorStorageWarningBytes int64 = 1 << 30
	defaultMonitorStorageHardBytes    int64 = 2 << 30
	maxMonitorStorageThresholdBytes   int64 = 1 << 50
)

func fillDefaultMonitorStorageThresholds(settings *AppSettings) {
	if settings.MonitorStorageWarningBytes == nil {
		value := defaultMonitorStorageWarningBytes
		settings.MonitorStorageWarningBytes = &value
	}
	if settings.MonitorStorageHardBytes == nil {
		value := defaultMonitorStorageHardBytes
		settings.MonitorStorageHardBytes = &value
	}
}

func validateMonitorStorageThresholds(settings *AppSettings) error {
	fillDefaultMonitorStorageThresholds(settings)
	warning, hard := *settings.MonitorStorageWarningBytes, *settings.MonitorStorageHardBytes
	if warning <= 0 || hard <= 0 || warning >= hard || hard > maxMonitorStorageThresholdBytes {
		return monitor.NewValidationError("Monitor 容量阈值无效：告警和保护阈值须为正数，告警阈值须小于保护阈值，保护阈值不得超过 1 PiB")
	}
	return nil
}

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
	settings, err := s.GetSettings()
	if err != nil {
		return nil, fmt.Errorf("read Monitor storage thresholds: %w", err)
	}
	usage, err := s.historyStore.StorageUsage(*settings.MonitorStorageWarningBytes, *settings.MonitorStorageHardBytes)
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
		return "Monitor SQLite 文件占用达到产品保护阈值；后台采集已暂停。删除 raw 可能只释放数据库内部可复用空间，可在容量设置中明确调整阈值"
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
