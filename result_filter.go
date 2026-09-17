package main

import (
	"fmt"
	"time"

	"github.com/faceair/clash-speedtest/core/speedtester"
)

type resultFilter struct {
	mode             speedtester.SpeedMode
	metrics          speedtester.MetricSet
	maxLatency       time.Duration
	maxPacketLoss    float64
	minDownloadSpeed float64
	minUploadSpeed   float64
	downloadSize     int
}

func newResultFilter(mode speedtester.SpeedMode, metrics speedtester.MetricSet) resultFilter {
	if metrics.IsZero() {
		metrics = speedtester.MetricsFromMode(mode)
	}
	return resultFilter{
		mode:             mode,
		metrics:          metrics,
		maxLatency:       *maxLatency,
		maxPacketLoss:    *maxPacketLoss,
		minDownloadSpeed: *minDownloadSpeed * 1024 * 1024,
		minUploadSpeed:   *minUploadSpeed * 1024 * 1024,
		downloadSize:     *downloadSize,
	}
}

func (f resultFilter) activeMetrics() speedtester.MetricSet {
	if f.metrics.IsZero() {
		return speedtester.MetricsFromMode(f.mode)
	}
	return f.metrics
}

func (f resultFilter) Match(result *speedtester.Result) bool {
	if result == nil {
		return false
	}
	metrics := f.activeMetrics()
	if metrics.Latency && f.maxLatency > 0 && result.Latency > f.maxLatency {
		return false
	}
	if metrics.Latency && f.maxPacketLoss >= 0 && result.PacketLoss > f.maxPacketLoss {
		return false
	}
	if metrics.Download && f.downloadSize > 0 && f.minDownloadSpeed > 0 && result.DownloadSpeed < f.minDownloadSpeed {
		return false
	}
	if metrics.Upload && f.minUploadSpeed > 0 && result.UploadSpeed < f.minUploadSpeed {
		return false
	}
	if result.ProxyConfig == nil || result.ProxyConfig["name"] == nil || result.ProxyConfig["server"] == nil {
		return false
	}
	return true
}

type earlyStopper struct {
	limit  int
	filter resultFilter
	count  int
}

func newEarlyStopper(limit int, filter resultFilter) (*earlyStopper, error) {
	if limit < 0 {
		return nil, fmt.Errorf("early-stop must be greater than or equal to 0, got %d", limit)
	}
	return &earlyStopper{
		limit:  limit,
		filter: filter,
	}, nil
}

func (s *earlyStopper) ShouldContinue(result *speedtester.Result) bool {
	if s.limit == 0 {
		return true
	}
	if s.filter.Match(result) {
		s.count++
	}
	return s.count < s.limit
}
