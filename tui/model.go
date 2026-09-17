package tui

import (
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/faceair/clash-speedtest/output"
	"github.com/faceair/clash-speedtest/speedtester"
)

// Messages for TUI updates
type resultMsg struct {
	result *speedtester.Result
}

type progressMsg struct {
	current int
	total   int
	name    string
}

type doneMsg struct{}

type timerTickMsg struct{}

type flushResultsMsg struct{}

// tuiModel represents the Bubble Tea model for the TUI
type tuiModel struct {
	mode           speedtester.SpeedMode
	totalProxies   int
	currentProxy   int
	results        []*speedtester.Result
	sequence       map[*speedtester.Result]int
	nextSequence   int
	baseHeaders    []string
	testing        bool
	quitting       bool
	progress       progress.Model
	table          table.Model
	help           helpState
	resultChannel  chan *speedtester.Result
	sortColumn     int
	sortAscending  bool
	detailVisible  bool
	detailResult   *speedtester.Result
	selectedIndex  int
	windowWidth    int
	windowHeight   int
	startTime      time.Time
	resultsDirty   bool
	flushScheduled bool
	detailHeight   int
	perf           *perfTracker
	duration       time.Duration
	rounds         int
	sampleCount    int
	resultIndex    map[string]int
	metrics        speedtester.MetricSet
}

const (
	tableHeaderPadding  = 1
	tableHeaderLines    = 2
	detailPanelMinWidth = 60
	defaultDetailWidth  = 80
	resultFlushInterval = 100 * time.Millisecond
)

var selectedRowStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("62")).
	Foreground(lipgloss.Color("230")).
	Bold(true)

// NewTUIModel creates a new TUI model
func NewTUIModel(mode speedtester.SpeedMode, totalProxies int, resultChannel chan *speedtester.Result) tuiModel {
	// Initialize progress bar
	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(40),
	)

	metrics := speedtester.MetricsFromMode(mode)
	headers := output.GetHeadersWithMetrics(mode, metrics)
	sortColumn, sortAscending := defaultSortState(mode, headers)
	columns := buildColumns(addSortIndicators(headers, sortColumn, sortAscending), 0, mode)

	t := table.New(
		table.WithColumns(columns),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	// Style the table
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	s.Selected = selectedRowStyle
	t.SetStyles(s)

	return tuiModel{
		mode:           mode,
		totalProxies:   totalProxies,
		currentProxy:   0,
		results:        make([]*speedtester.Result, 0),
		sequence:       make(map[*speedtester.Result]int),
		nextSequence:   0,
		baseHeaders:    headers,
		testing:        true,
		quitting:       false,
		progress:       p,
		table:          t,
		help:           newHelpState(t.KeyMap),
		resultChannel:  resultChannel,
		sortColumn:     sortColumn,
		sortAscending:  sortAscending,
		detailVisible:  false,
		detailResult:   nil,
		selectedIndex:  -1,
		windowWidth:    0,
		windowHeight:   0,
		startTime:      time.Now(),
		resultsDirty:   false,
		flushScheduled: false,
		detailHeight:   0,
		perf:           newPerfTracker(),
		resultIndex:    make(map[string]int),
		metrics:        metrics,
	}
}

func (m tuiModel) WithDuration(duration time.Duration) tuiModel {
	m.duration = duration
	return m
}

func (m tuiModel) WithRounds(rounds int) tuiModel {
	if rounds <= 0 {
		rounds = 1
	}
	m.rounds = rounds
	return m
}

func (m tuiModel) WithMetrics(metrics speedtester.MetricSet) tuiModel {
	if metrics.IsZero() {
		metrics = speedtester.MetricsFromMode(m.mode)
	}
	m.metrics = metrics
	m.baseHeaders = output.GetHeadersWithMetrics(m.mode, metrics)
	m.sortColumn, m.sortAscending = defaultSortState(m.mode, m.baseHeaders)
	m.table.SetColumns(buildColumns(addSortIndicators(m.baseHeaders, m.sortColumn, m.sortAscending), m.windowWidth, m.mode))
	return m
}

// Init initializes the TUI model
func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(
		timerTickCmd(),
		m.waitForResult(),
	)
}

// waitForResult waits for results from the channel
func (m tuiModel) waitForResult() tea.Cmd {
	return func() tea.Msg {
		result, ok := <-m.resultChannel
		if !ok {
			return doneMsg{}
		}
		return resultMsg{result: result}
	}
}

func scheduleFlushCmd() tea.Cmd {
	return tea.Tick(resultFlushInterval, func(time.Time) tea.Msg {
		return flushResultsMsg{}
	})
}

func (m *tuiModel) flushResultsIfDirty() {
	if !m.resultsDirty {
		return
	}
	m.sortResults()
	m.updateTableRows()
	m.resultsDirty = false
}

// Update handles messages and updates the model
func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.detailVisible {
				m.detailVisible = false
				m.help.setDetailVisible(false)
				m.detailHeight = 0
				m.updateTableLayout()
				return m, nil
			}
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}

		m.table, cmd = m.table.Update(msg)
		m.syncSelectionFromCursor()
		return m, cmd

	case tea.MouseMsg:
		if msg.Button == tea.MouseButtonWheelUp {
			m.table.MoveUp(1)
			m.syncSelectionFromCursor()
			return m, nil
		}
		if msg.Button == tea.MouseButtonWheelDown {
			m.table.MoveDown(1)
			m.syncSelectionFromCursor()
			return m, nil
		}
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionRelease {
			if m.isHeaderClick(msg.Y) {
				if columnIndex := m.columnAtX(msg.X); columnIndex >= 0 {
					if columnIndex == m.sortColumn {
						m.sortAscending = !m.sortAscending
					} else {
						m.sortColumn = columnIndex
						m.sortAscending = m.defaultSortAscending(columnIndex)
					}
					m.sortResults()
					m.updateTableHeaders()
					m.updateTableRows()
					m.resultsDirty = false
				}
				return m, nil
			}
			if rowIndex, ok := m.rowAtY(msg.Y); ok {
				m.toggleDetail(m.results[rowIndex])
				m.setSelection(rowIndex)
				return m, nil
			}
		}

	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
		m.windowHeight = msg.Height
		m.refreshDetailHeight()
		m.updateTableLayout()
		return m, nil

	case resultMsg:
		m.upsertResult(msg.result)
		m.resultsDirty = true
		progressCmd := m.progress.SetPercent(m.progressPercent())
		cmds := []tea.Cmd{progressCmd, m.waitForResult()}
		if !m.flushScheduled {
			m.flushScheduled = true
			cmds = append(cmds, scheduleFlushCmd())
		}
		return m, tea.Batch(cmds...)

	case doneMsg:
		m.testing = false
		m.flushScheduled = false
		m.flushResultsIfDirty()
		progressCmd := m.progress.SetPercent(1.0)
		return m, progressCmd

	case progressMsg:
		m.currentProxy = msg.current
		m.totalProxies = msg.total
		m.progress.SetPercent(float64(m.currentProxy) / float64(m.totalProxies))

		// Update progress bar
		progressModel, progressCmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		cmd = progressCmd

		return m, cmd

	case timerTickMsg:
		if m.duration > 0 && m.testing {
			return m, tea.Batch(timerTickCmd(), m.progress.SetPercent(m.progressPercent()))
		}
		return m, timerTickCmd()

	case flushResultsMsg:
		m.flushScheduled = false
		m.flushResultsIfDirty()
		return m, nil
	}

	// Default: update progress bar for other messages
	progressModel, progressCmd := m.progress.Update(msg)
	m.progress = progressModel.(progress.Model)
	cmd = progressCmd

	return m, cmd
}

func (m *tuiModel) upsertResult(result *speedtester.Result) {
	if result == nil {
		return
	}
	m.sampleCount++
	if idx, ok := m.resultIndex[result.ProxyName]; ok {
		old := m.results[idx]
		m.results[idx] = result
		if seq, exists := m.sequence[old]; exists {
			m.sequence[result] = seq
			delete(m.sequence, old)
		} else {
			m.recordSequence(result)
		}
		if m.detailVisible && m.detailResult != nil && m.detailResult.ProxyName == result.ProxyName {
			m.detailResult = result
		}
		return
	}
	m.currentProxy++
	m.resultIndex[result.ProxyName] = len(m.results)
	m.results = append(m.results, result)
	m.recordSequence(result)
}

func (m tuiModel) progressPercent() float64 {
	if m.duration > 0 {
		elapsed := time.Since(m.startTime)
		if elapsed <= 0 {
			return 0
		}
		percent := float64(elapsed) / float64(m.duration)
		if percent > 1 {
			return 1
		}
		return percent
	}
	if m.rounds > 1 {
		total := m.totalProxies * m.rounds
		if total <= 0 {
			return 0
		}
		percent := float64(m.sampleCount) / float64(total)
		if percent > 1 {
			return 1
		}
		return percent
	}
	if m.totalProxies <= 0 {
		return 0
	}
	return float64(m.currentProxy) / float64(m.totalProxies)
}

// View renders the TUI
func (m tuiModel) View() string {
	if m.quitting {
		return ""
	}

	// Layout: progress bar at top, table below
	tableView := m.table.View()
	detailView := m.detailPanelView()

	sections := []string{
		m.progressLine(),
		"",
		tableView,
	}
	if detailView != "" {
		sections = append(sections, "", detailView)
	}
	helpView := m.help.view()
	if helpView != "" {
		sections = append(sections, "", helpView)
	}
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}
