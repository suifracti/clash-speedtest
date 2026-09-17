package tui

import (
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/faceair/clash-speedtest/output"
	"github.com/faceair/clash-speedtest/speedtester"
)

// updateTableRows updates the table rows with current results
func (m *tuiModel) updateTableRows() {
	start := time.Now()
	defer m.perf.record(perfEventRows, len(m.results), start)
	rows := make([]table.Row, len(m.results))
	for i, result := range m.results {
		rows[i] = output.FormatRowWithMetrics(result, m.mode, i, m.metrics)
	}
	m.table.SetRows(rows)
	m.syncSelection()
}

func (m *tuiModel) updateTableHeaders() {
	if len(m.baseHeaders) == 0 {
		return
	}
	columns := buildColumns(addSortIndicators(m.baseHeaders, m.sortColumn, m.sortAscending), m.windowWidth, m.mode)
	m.table.SetColumns(columns)
}

func buildColumns(headers []string, width int, mode speedtester.SpeedMode) []table.Column {
	columns := make([]table.Column, len(headers))
	widths := calculateHeaderColumnWidths(headers, width)
	for i, h := range headers {
		columnWidth := 10
		if i < len(widths) {
			columnWidth = widths[i]
		}
		columns[i] = table.Column{Title: h, Width: columnWidth}
	}
	return columns
}

func calculateHeaderColumnWidths(headers []string, width int) []int {
	columnPadding := 2
	columnCount := len(headers)
	windowWidth := width
	availableWidth := width
	if width > 0 {
		availableWidth = max(width-columnCount*columnPadding, 0)
	}

	widths := make([]int, len(headers))
	minWidths := make([]int, len(headers))
	type colPriority struct {
		index    int
		priority int
	}
	priorities := make([]colPriority, len(headers))

	fixedWidth := 0
	nameIndex := -1

	for i, h := range headers {
		clean := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(h, " ⇅"), " ↑"), " ↓"))
		switch clean {
		case "序号":
			widths[i] = 6
			minWidths[i] = 4
			priorities[i] = colPriority{index: i, priority: 9}
			fixedWidth += widths[i]
		case "节点名称":
			nameIndex = i
			minWidths[i] = 4
			priorities[i] = colPriority{index: i, priority: 0}
		case "类型":
			widths[i] = 12
			minWidths[i] = 6
			priorities[i] = colPriority{index: i, priority: 8}
			fixedWidth += widths[i]
		case "延迟":
			widths[i] = 10
			minWidths[i] = 6
			priorities[i] = colPriority{index: i, priority: 6}
			fixedWidth += widths[i]
		case "抖动":
			widths[i] = 10
			minWidths[i] = 6
			priorities[i] = colPriority{index: i, priority: 5}
			fixedWidth += widths[i]
		case "丢包率":
			widths[i] = 10
			minWidths[i] = 6
			priorities[i] = colPriority{index: i, priority: 5}
			fixedWidth += widths[i]
		case "下载速度":
			widths[i] = 16
			minWidths[i] = 6
			priorities[i] = colPriority{index: i, priority: 2}
			fixedWidth += widths[i]
		case "上传速度":
			widths[i] = 16
			minWidths[i] = 6
			priorities[i] = colPriority{index: i, priority: 3}
			fixedWidth += widths[i]
		case "Antigravity":
			widths[i] = 14
			minWidths[i] = 8
			priorities[i] = colPriority{index: i, priority: 4}
			fixedWidth += widths[i]
		case "出口":
			widths[i] = 16
			minWidths[i] = 6
			priorities[i] = colPriority{index: i, priority: 4}
			fixedWidth += widths[i]
		default:
			widths[i] = 12
			minWidths[i] = 6
			priorities[i] = colPriority{index: i, priority: 7}
			fixedWidth += widths[i]
		}
	}

	if nameIndex >= 0 {
		if windowWidth <= 0 {
			widths[nameIndex] = 30
		} else {
			widths[nameIndex] = max(minWidths[nameIndex], availableWidth-fixedWidth)
		}
	}

	if windowWidth <= 0 {
		return widths
	}

	sort.SliceStable(priorities, func(i, j int) bool {
		return priorities[i].priority < priorities[j].priority
	})
	shrinkOrder := make([]int, len(priorities))
	for i, p := range priorities {
		shrinkOrder[i] = p.index
	}

	return shrinkWidthsToFit(windowWidth, columnPadding, widths, minWidths, shrinkOrder)
}

func calculateColumnWidths(width int, mode speedtester.SpeedMode) []int {
	return calculateHeaderColumnWidths(output.GetHeaders(mode), width)
}

func shrinkWidthsToFit(windowWidth int, columnPadding int, widths []int, minWidths []int, shrinkOrder []int) []int {
	if windowWidth <= 0 {
		return widths
	}
	padding := columnPadding * len(widths)
	maxTotal := max(windowWidth-padding, 0)
	total := 0
	for _, value := range widths {
		total += value
	}
	overflow := total - maxTotal
	for overflow > 0 {
		shrunk := false
		for _, idx := range shrinkOrder {
			if idx < 0 || idx >= len(widths) || idx >= len(minWidths) {
				continue
			}
			if widths[idx] > minWidths[idx] {
				widths[idx]--
				overflow--
				shrunk = true
				if overflow == 0 {
					break
				}
			}
		}
		if !shrunk {
			break
		}
	}
	return widths
}

func addSortIndicators(headers []string, sortColumn int, sortAscending bool) []string {
	withIndicators := make([]string, len(headers))
	for i, header := range headers {
		withIndicators[i] = header + " ⇅"
	}
	if sortColumn >= 0 && sortColumn < len(withIndicators) {
		direction := "↓"
		if sortAscending {
			direction = "↑"
		}
		withIndicators[sortColumn] = headers[sortColumn] + " " + direction
	}
	return withIndicators
}

func (m tuiModel) columnAtX(x int) int {
	if x < 0 {
		return -1
	}
	columns := m.table.Columns()
	currentX := 0
	for i, col := range columns {
		width := col.Width + tableHeaderPadding*2
		if x >= currentX && x < currentX+width {
			return i
		}
		currentX += width
	}
	return -1
}

func (m tuiModel) rowAtY(y int) (int, bool) {
	startY := m.tableHeaderY() + tableHeaderLines
	if y < startY {
		return 0, false
	}
	rowIndex := y - startY
	if rowIndex < 0 || rowIndex >= m.table.Height() {
		return 0, false
	}
	start := tableStartIndex(m.table.Cursor(), m.table.Height())
	absoluteIndex := start + rowIndex
	if absoluteIndex < 0 || absoluteIndex >= len(m.results) {
		return 0, false
	}
	return absoluteIndex, true
}

func (m tuiModel) tableHeaderY() int {
	return lipgloss.Height(lipgloss.JoinVertical(
		lipgloss.Left,
		m.progressLine(),
		"",
	))
}

func (m tuiModel) isHeaderClick(y int) bool {
	startY := m.tableHeaderY()
	endY := startY + tableHeaderLines
	return y >= startY && y < endY
}

func (m *tuiModel) setSelection(index int) {
	if index < 0 || index >= len(m.results) {
		m.selectedIndex = -1
		return
	}
	if m.detailResult == nil || m.detailResult != m.results[index] {
		m.detailResult = m.results[index]
	}
	m.selectedIndex = index
	m.table.SetCursor(index)
	m.table.Focus()
}

func (m *tuiModel) syncSelection() {
	if m.selectedIndex < 0 {
		return
	}
	if m.detailResult != nil {
		for i, result := range m.results {
			if result == m.detailResult {
				m.selectedIndex = i
				break
			}
		}
	}
	if m.selectedIndex < 0 || m.selectedIndex >= len(m.results) {
		m.selectedIndex = -1
		m.table.Blur()
		return
	}
	m.table.SetCursor(m.selectedIndex)
	m.table.Focus()
}

func (m *tuiModel) syncSelectionFromCursor() {
	cursor := m.table.Cursor()
	if cursor < 0 || cursor >= len(m.results) {
		return
	}
	m.selectedIndex = cursor
	if m.detailVisible {
		if m.detailResult != m.results[cursor] {
			previousHeight := m.detailHeight
			m.detailResult = m.results[cursor]
			m.refreshDetailHeight()
			if m.detailHeight != previousHeight {
				// Keep layout in sync when detail content height changes on scroll.
				m.updateTableLayout()
			}
		}
	}
	m.table.Focus()
}

func tableStartIndex(cursor int, height int) int {
	if cursor < 0 {
		return 0
	}
	return clampInt(cursor-height, 0, cursor)
}

func clampInt(value int, low int, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

// colorizeRow applies color thresholds to a row
func (m *tuiModel) colorizeRow(row []string, result *speedtester.Result) table.Row {
	// Color thresholds matching ANSI colors in main.go
	// Latency: <800ms green, <1500ms yellow, >=1500ms red
	latencyStr := row[3]
	if result.Latency > 0 {
		if result.Latency < 800*time.Millisecond {
			latencyStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render(latencyStr) // green
		} else if result.Latency < 1500*time.Millisecond {
			latencyStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")).Render(latencyStr) // yellow
		} else {
			latencyStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render(latencyStr) // red
		}
	} else {
		latencyStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render(latencyStr) // red
	}

	if m.mode.IsFast() {
		return table.Row{row[0], row[1], row[2], latencyStr}
	}

	// Jitter: <800ms green, <1500ms yellow, >=1500ms red
	jitterStr := row[4]
	if result.Jitter > 0 {
		if result.Jitter < 800*time.Millisecond {
			jitterStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render(jitterStr) // green
		} else if result.Jitter < 1500*time.Millisecond {
			jitterStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")).Render(jitterStr) // yellow
		} else {
			jitterStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render(jitterStr) // red
		}
	} else {
		jitterStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render(jitterStr) // red
	}

	// Packet loss: <10% green, <20% yellow, >=20% red
	packetLossStr := row[5]
	if result.PacketLoss < 10 {
		packetLossStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render(packetLossStr) // green
	} else if result.PacketLoss < 20 {
		packetLossStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")).Render(packetLossStr) // yellow
	} else {
		packetLossStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render(packetLossStr) // red
	}

	// Download speed: >=10MB/s green, >=5MB/s yellow, <5MB/s red
	downloadSpeed := result.DownloadSpeed / (1024 * 1024)
	downloadSpeedStr := row[6]
	if downloadSpeed >= 10 {
		downloadSpeedStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render(downloadSpeedStr) // green
	} else if downloadSpeed >= 5 {
		downloadSpeedStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")).Render(downloadSpeedStr) // yellow
	} else {
		downloadSpeedStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render(downloadSpeedStr) // red
	}

	if !m.mode.UploadEnabled() {
		return table.Row{
			row[0],
			row[1],
			row[2],
			latencyStr,
			jitterStr,
			packetLossStr,
			downloadSpeedStr,
		}
	}

	// Upload speed: >=5MB/s green, >=2MB/s yellow, <2MB/s red
	uploadSpeed := result.UploadSpeed / (1024 * 1024)
	uploadSpeedStr := row[7]
	if uploadSpeed >= 5 {
		uploadSpeedStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render(uploadSpeedStr) // green
	} else if uploadSpeed >= 2 {
		uploadSpeedStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")).Render(uploadSpeedStr) // yellow
	} else {
		uploadSpeedStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render(uploadSpeedStr) // red
	}

	return table.Row{
		row[0],
		row[1],
		row[2],
		latencyStr,
		jitterStr,
		packetLossStr,
		downloadSpeedStr,
		uploadSpeedStr,
	}
}
