package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/faceair/clash-speedtest/core/speedtester"
)

func TestTUIModelHeaderClickSort(t *testing.T) {
	resultChannel := make(chan *speedtester.Result, 10)
	model := NewTUIModel(speedtester.SpeedModeDownload, 3, resultChannel)

	result1 := &speedtester.Result{
		ProxyName:     "Proxy 1",
		ProxyType:     "SS",
		Latency:       300 * time.Millisecond,
		DownloadSpeed: 5 * 1024 * 1024,
		ProxyConfig:   map[string]any{},
	}
	result2 := &speedtester.Result{
		ProxyName:     "Proxy 2",
		ProxyType:     "Trojan",
		Latency:       100 * time.Millisecond,
		DownloadSpeed: 10 * 1024 * 1024,
		ProxyConfig:   map[string]any{},
	}
	result3 := &speedtester.Result{
		ProxyName:     "Proxy 3",
		ProxyType:     "Vmess",
		Latency:       200 * time.Millisecond,
		DownloadSpeed: 8 * 1024 * 1024,
		ProxyConfig:   map[string]any{},
	}

	updatedModel, _ := model.Update(resultMsg{result: result1})
	updatedModel, _ = updatedModel.(tuiModel).Update(resultMsg{result: result2})
	updatedModel, _ = updatedModel.(tuiModel).Update(resultMsg{result: result3})

	clickMsg := tea.MouseMsg{
		X:      headerClickX(updatedModel.(tuiModel), 3),
		Y:      updatedModel.(tuiModel).tableHeaderY(),
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
	}
	updatedModel, _ = updatedModel.(tuiModel).Update(clickMsg)

	if updatedModel.(tuiModel).results[0] != result2 {
		t.Error("Expected first result to be the lowest latency after header click")
	}
	if updatedModel.(tuiModel).results[2] != result1 {
		t.Error("Expected last result to be the highest latency after header click")
	}

	updatedModel, _ = updatedModel.(tuiModel).Update(clickMsg)
	if updatedModel.(tuiModel).results[0] != result1 {
		t.Error("Expected first result to be the highest latency after second header click")
	}
}

func headerClickX(model tuiModel, column int) int {
	x := 0
	for i, col := range model.table.Columns() {
		width := col.Width + tableHeaderPadding*2
		if i == column {
			return x + width/2
		}
		x += width
	}
	return -1
}

func TestSortResultsLatencyNA(t *testing.T) {
	resultChannel := make(chan *speedtester.Result, 10)
	model := NewTUIModel(speedtester.SpeedModeFast, 2, resultChannel)

	naResult := &speedtester.Result{
		ProxyName:   "NA",
		ProxyType:   "SS",
		Latency:     0,
		ProxyConfig: map[string]any{},
	}
	fastResult := &speedtester.Result{
		ProxyName:   "Fast",
		ProxyType:   "SS",
		Latency:     120 * time.Millisecond,
		ProxyConfig: map[string]any{},
	}

	model.results = []*speedtester.Result{naResult, fastResult}
	model.sortColumn = 3
	model.sortAscending = true
	model.sortResults()

	if model.results[0] != fastResult {
		t.Error("Expected non-N/A latency to sort before N/A")
	}
	if model.results[1] != naResult {
		t.Error("Expected N/A latency to sort last when ascending")
	}
}

func TestDefaultSortStateAndAscending(t *testing.T) {
	t.Run("upload only headers", func(t *testing.T) {
		headers := []string{"序号", "节点名称", "类型", "上传速度"}
		col, asc := defaultSortState(speedtester.SpeedModeFull, headers)
		if col != 3 || asc != false {
			t.Errorf("expected col 3, asc false, got col %d, asc %v", col, asc)
		}
	})

	t.Run("antigravity only headers", func(t *testing.T) {
		headers := []string{"序号", "节点名称", "类型", "Antigravity", "出口"}
		col, asc := defaultSortState(speedtester.SpeedModeFast, headers)
		if col != 3 || asc != true {
			t.Errorf("expected col 3, asc true, got col %d, asc %v", col, asc)
		}
	})

	t.Run("latency only headers", func(t *testing.T) {
		headers := []string{"序号", "节点名称", "类型", "延迟"}
		col, asc := defaultSortState(speedtester.SpeedModeFast, headers)
		if col != 3 || asc != true {
			t.Errorf("expected col 3, asc true, got col %d, asc %v", col, asc)
		}
	})
}

func TestSortResultsByMetrics(t *testing.T) {
	resultChannel := make(chan *speedtester.Result, 10)
	model := NewTUIModel(speedtester.SpeedModeFull, 2, resultChannel).WithMetrics(speedtester.MetricSet{Upload: true})

	r1 := &speedtester.Result{ProxyName: "P1", UploadSpeed: 10 * 1024 * 1024}
	r2 := &speedtester.Result{ProxyName: "P2", UploadSpeed: 50 * 1024 * 1024}
	model.results = []*speedtester.Result{r1, r2}

	// Should default to sorting by 上传速度 (column 3) descending
	if model.sortColumn != 3 || model.sortAscending != false {
		t.Fatalf("expected sortColumn 3, ascending false, got %d, %v", model.sortColumn, model.sortAscending)
	}

	model.sortResults()
	if model.results[0] != r2 {
		t.Errorf("expected r2 first for upload sort descending, got %s", model.results[0].ProxyName)
	}
}
