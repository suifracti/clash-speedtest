package application

import (
	"context"
	"fmt"

	"github.com/faceair/clash-speedtest/core/monitor"
)

const (
	defaultBudgetConcurrent          = 4
	defaultBudgetResponseBytes int64 = 256 << 10
)

func fillDefaultMonitorBudget(settings *AppSettings) {
	if settings.MonitorBudgetMaxConcurrent == nil {
		value := defaultBudgetConcurrent
		settings.MonitorBudgetMaxConcurrent = &value
	}
	if settings.MonitorBudgetResponseBytes == nil {
		value := defaultBudgetResponseBytes
		settings.MonitorBudgetResponseBytes = &value
	}
}

func validateMonitorBudget(settings *AppSettings) error {
	fillDefaultMonitorBudget(settings)
	concurrent, responseBytes := *settings.MonitorBudgetMaxConcurrent, *settings.MonitorBudgetResponseBytes
	if concurrent < 1 || concurrent > 64 || responseBytes < 1 || responseBytes > 16<<20 {
		return monitor.NewValidationError("Monitor 请求设置无效：并发须为 1–64、单响应 1 B–16 MiB")
	}
	return nil
}

func (s *AppService) monitorBudgetLimits() (monitor.BudgetLimits, error) {
	settings, err := s.GetSettings()
	if err != nil {
		return monitor.BudgetLimits{}, err
	}
	return monitor.BudgetLimits{
		MaxConcurrent: *settings.MonitorBudgetMaxConcurrent,
		ResponseBytes: *settings.MonitorBudgetResponseBytes,
	}, nil
}

func (s *AppService) GetMonitorBudgetStatus(ctx context.Context) (*monitor.BudgetStatus, error) {
	if err := s.canonicalMonitorHistory(); err != nil {
		return nil, err
	}
	if s.monitorBudget == nil {
		return nil, fmt.Errorf("Monitor budget is not initialized")
	}
	status, err := s.monitorBudget.Status(ctx)
	if err != nil {
		return nil, err
	}
	return &status, nil
}

func (s *AppService) monitorBudgetGuard() (string, string) {
	status, err := s.GetMonitorBudgetStatus(context.Background())
	if err != nil {
		if block, ok := monitor.AsBudgetBlock(err); ok {
			return block.Code, block.Reason
		}
		return "budget_unavailable", fmt.Sprintf("无法核对 Monitor 预算：%v", err)
	}
	return status.BlockedCode, status.BlockedReason
}
