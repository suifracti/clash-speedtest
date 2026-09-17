package tui

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/faceair/clash-speedtest/core/speedtester"
)

func (m *tuiModel) recordSequence(result *speedtester.Result) {
	if _, ok := m.sequence[result]; ok {
		return
	}
	m.nextSequence++
	m.sequence[result] = m.nextSequence
}

func defaultSortState(mode speedtester.SpeedMode, headers []string) (int, bool) {
	priority := []string{"下载速度", "上传速度", "Antigravity", "延迟"}
	for _, target := range priority {
		for i, header := range headers {
			if header == target {
				return i, defaultSortAscendingForHeader(target)
			}
		}
	}
	return 0, true
}

func defaultSortAscendingForHeader(header string) bool {
	switch header {
	case "下载速度", "上传速度":
		return false
	default:
		return true
	}
}

func (m *tuiModel) defaultSortAscending(column int) bool {
	if column >= 0 && column < len(m.baseHeaders) {
		return defaultSortAscendingForHeader(m.baseHeaders[column])
	}
	return true
}

func defaultSortAscending(column int) bool {
	switch column {
	case 6, 7:
		return false
	default:
		return true
	}
}

func (m *tuiModel) sortResults() {
	start := time.Now()
	defer m.perf.record(perfEventSort, len(m.results), start)
	sort.SliceStable(m.results, func(i, j int) bool {
		comparison := m.compareResults(m.results[i], m.results[j])
		if m.sortAscending {
			return comparison < 0
		}
		return comparison > 0
	})
}

func (m *tuiModel) compareResults(a, b *speedtester.Result) int {
	header := ""
	if m.sortColumn >= 0 && m.sortColumn < len(m.baseHeaders) {
		header = m.baseHeaders[m.sortColumn]
	}
	switch header {
	case "序号":
		return compareInt(m.sequence[a], m.sequence[b])
	case "节点名称":
		return strings.Compare(a.ProxyName, b.ProxyName)
	case "类型":
		return strings.Compare(a.ProxyType, b.ProxyType)
	case "延迟":
		return compareDuration(a.Latency, b.Latency)
	case "抖动":
		return compareDuration(a.Jitter, b.Jitter)
	case "丢包率":
		return compareFloat(a.PacketLoss, b.PacketLoss)
	case "下载速度":
		return compareFloat(a.DownloadSpeed, b.DownloadSpeed)
	case "上传速度":
		return compareFloat(a.UploadSpeed, b.UploadSpeed)
	case "Antigravity":
		return compareInt(speedtester.AntigravityRank(a.AntigravityStatus), speedtester.AntigravityRank(b.AntigravityStatus))
	case "出口":
		return strings.Compare(a.FormatExitCountry(), b.FormatExitCountry())
	default:
		return 0
	}
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func compareFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func compareDuration(a, b time.Duration) int {
	aValue := durationSortValue(a)
	bValue := durationSortValue(b)
	switch {
	case aValue < bValue:
		return -1
	case aValue > bValue:
		return 1
	default:
		return 0
	}
}

func durationSortValue(value time.Duration) time.Duration {
	if value == 0 {
		return time.Duration(math.MaxInt64)
	}
	return value
}
