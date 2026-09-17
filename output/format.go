package output

import (
	"fmt"
	"sort"

	"github.com/faceair/clash-speedtest/speedtester"
)

// GetHeaders returns table headers based on speed mode.
// fast: ID, Name, Type, Latency
// download: ID, Name, Type, Latency, Jitter, Packet Loss, Download Speed
// full: ID, Name, Type, Latency, Jitter, Packet Loss, Download Speed, Upload Speed
func GetHeaders(mode speedtester.SpeedMode) []string {
	return GetHeadersWithMetrics(mode, speedtester.MetricSet{})
}

func GetHeadersWithMetrics(mode speedtester.SpeedMode, metrics speedtester.MetricSet) []string {
	if metrics.IsZero() {
		metrics = speedtester.MetricsFromMode(mode)
	}
	headers := []string{
		"序号",
		"节点名称",
		"类型",
	}
	if metrics.Latency {
		headers = append(headers, "延迟")
		if !mode.IsFast() {
			headers = append(headers, "抖动", "丢包率")
		}
	}
	if metrics.Download {
		headers = append(headers, "下载速度")
	}
	if metrics.Upload {
		headers = append(headers, "上传速度")
	}
	if metrics.Antigravity {
		headers = append(headers, "Antigravity", "出口")
	}
	return headers
}

// FormatRow formats a single result row without ANSI colors.
// Returns plain text strings using speedtester.Result's Format* methods.
func FormatRow(result *speedtester.Result, mode speedtester.SpeedMode, index int) []string {
	return FormatRowWithMetrics(result, mode, index, speedtester.MetricSet{})
}

func FormatRowWithMetrics(result *speedtester.Result, mode speedtester.SpeedMode, index int, metrics speedtester.MetricSet) []string {
	if metrics.IsZero() {
		metrics = speedtester.MetricsFromMode(mode)
	}
	idStr := fmt.Sprintf("%d.", index+1)
	row := []string{
		idStr,
		result.ProxyName,
		result.ProxyType,
	}
	if metrics.Latency {
		row = append(row, result.FormatLatency())
		if !mode.IsFast() {
			row = append(row, result.FormatJitter(), result.FormatPacketLoss())
		}
	}
	if metrics.Download {
		row = append(row, result.FormatDownloadSpeed())
	}
	if metrics.Upload {
		row = append(row, result.FormatUploadSpeed())
	}
	if metrics.Antigravity {
		row = append(row, result.FormatAntigravity(), result.FormatExitCountry())
	}
	return row
}

// SortResults sorts results based on speed mode.
// fast: latency ascending (lower is better)
// download/full: download speed descending (higher is better)
func SortResults(results []*speedtester.Result, mode speedtester.SpeedMode) []*speedtester.Result {
	return SortResultsWithMetrics(results, mode, speedtester.MetricSet{})
}

func SortResultsWithMetrics(results []*speedtester.Result, mode speedtester.SpeedMode, metrics speedtester.MetricSet) []*speedtester.Result {
	if metrics.IsZero() {
		metrics = speedtester.MetricsFromMode(mode)
	}
	if metrics.Download {
		sort.Slice(results, func(i, j int) bool {
			return results[i].DownloadSpeed > results[j].DownloadSpeed
		})
		return results
	}
	if metrics.Upload {
		sort.Slice(results, func(i, j int) bool {
			return results[i].UploadSpeed > results[j].UploadSpeed
		})
		return results
	}
	if metrics.Antigravity {
		sort.Slice(results, func(i, j int) bool {
			return speedtester.AntigravityRank(results[i].AntigravityStatus) < speedtester.AntigravityRank(results[j].AntigravityStatus)
		})
		return results
	}
	if metrics.Latency {
		sort.Slice(results, func(i, j int) bool {
			return results[i].Latency < results[j].Latency
		})
		return results
	}
	return results
}
