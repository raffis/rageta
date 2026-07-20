package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/stopwatch"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/go-logr/logr"
	"github.com/raffis/rageta/internal/processor"
)

// TEMPORARY: diagnostic logging for the "UI is slow/laggy with many tasks"
// investigation. Writes one line per Update() call to /tmp/ui.log with the
// message type, current list size, and how long the call took, so we can
// see which message types dominate and whether cost scales with list size.
// Remove once the investigation is done.
var (
	debugLogOnce sync.Once
	debugLogFile *os.File
)

func debugLog(format string, args ...any) {
	debugLogOnce.Do(func() {
		f, err := os.OpenFile("/tmp/ui.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			debugLogFile = f
		}
	})
	if debugLogFile != nil {
		fmt.Fprintf(debugLogFile, "%s "+format+"\n", append([]any{time.Now().Format(time.RFC3339Nano)}, args...)...)
	}
}

type Panel int8

const (
	PanelList Panel = iota
	PanelDetails
)

const (
	ListWidthPercentage       = 35.0
	ListHeightPercentage      = 40.0
	LayoutAreaHeight          = 4
	LayoutAreaHeightNarrow    = 6
	FilterInputHeightOffset   = 1
	LabelsHeightOffset        = 1
	AlignHorizontalBreakpoint = 250
)

const (
	KeyFilter     = "/"
	KeyEscape     = "esc"
	KeyTab        = "tab"
	KeyEnter      = "enter"
	KeyQuit       = "ctrl+c"
	KeyQ          = "q"
	KeyDebugShell = "s"
	KeyShowAll    = "a"
)

// uiKeyMap is the help.KeyMap rendered in the bottom help bar. It's a
// hand-picked list of bindings rather than a delegate to list.KeyMap so we
// can fully control what's shown (no "?" full-help toggle, arrow keys only).
type uiKeyMap struct{}

func (uiKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "up")),
		key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "down")),
		key.NewBinding(key.WithKeys(KeyFilter), key.WithHelp(KeyFilter, "filter")),
		key.NewBinding(key.WithKeys(KeyTab), key.WithHelp("⇅", "switch panel")),
		key.NewBinding(key.WithKeys(KeyShowAll), key.WithHelp(KeyShowAll, "show/hide all")),
		key.NewBinding(key.WithKeys(KeyDebugShell), key.WithHelp(KeyDebugShell, "shell")),
		key.NewBinding(key.WithKeys(KeyQ), key.WithHelp(KeyQ, "quit")),
	}
}

func (k uiKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.ShortHelp()}
}

// DebugShellFactory builds the tea.ExecCommand used to run a debug shell for the given
// task context. It is provided by the run package, which owns the buildkit client and
// container runtime needed to export and start the task's state as a container.
type DebugShellFactory func(taskCtx processor.TaskContext) (tea.ExecCommand, error)

// DebugShellDoneMsg is delivered once a debug shell spawned via tea.Exec exits.
type DebugShellDoneMsg struct {
	Name string
	Err  error
}

type UI struct {
	list         list.Model
	loader       spinner.Model
	help         help.Model
	status       TaskStatus
	scanInput    textinput.Model
	width        int
	height       int
	mu           *sync.Mutex
	logger       logr.Logger
	exitErr      error
	activePanel  Panel
	lastSelected list.Item
	debugShell   DebugShellFactory

	// tasks is the source of truth for every task the pipeline has reported,
	// in display order. m.list only ever holds the currently visible subset
	// (see refreshList), so every mutation must go through m.tasks first.
	tasks   []TaskMsg
	showAll bool
}

// SetDebugShell registers the factory used to spawn a debug shell for the selected task.
func (m *UI) SetDebugShell(fn DebugShellFactory) {
	m.debugShell = fn
}

type TickMsg time.Time

type PipelineDoneMsg struct {
	Status TaskStatus
	Error  error
}

func NewUI(logger logr.Logger) UI {
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		BorderForeground(activePanelColor).
		Border(lipgloss.BlockBorder(), false, false, false, true)

	delegate.ShowDescription = true
	delegate.SetSpacing(0)
	noBorderDescPadding := lipgloss.NewStyle().Padding(0, 0, 0, 2)
	delegate.Styles.SelectedDesc = noBorderDescPadding
	delegate.Styles.NormalDesc = noBorderDescPadding
	delegate.Styles.DimmedDesc = noBorderDescPadding

	ui := UI{
		status:      TaskStatusWaiting,
		list:        list.New(nil, delegate, 0, 0),
		help:        help.New(),
		mu:          &sync.Mutex{},
		activePanel: PanelList,
		logger:      logger,
	}

	ui.initializeList()
	ui.initializeScanInput()
	ui.initializeLoader()

	return ui
}

// initializeList sets up the list component
func (m *UI) initializeList() {
	m.list.SetShowTitle(false)
	m.list.SetShowStatusBar(false)
	m.list.SetShowHelp(false)
	m.list.SetShowFilter(false)
	m.list.SetFilteringEnabled(true)
	m.list.Styles.PaginationStyle = listPaginatorStyle
	m.list.KeyMap.CursorUp = key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "up"))
	m.list.KeyMap.CursorDown = key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "down"))
}

// initializeScanInput sets up the scan input component
func (m *UI) initializeScanInput() {
	scanInput := textinput.New()
	scanInput.Prompt = "Filter: "
	scanInput.CharLimit = 64
	scanInput.Focus()
	m.scanInput = scanInput
}

// initializeLoader sets up the loading spinner
func (m *UI) initializeLoader() {
	m.loader = spinner.New()
	m.loader.Spinner = spinner.Dot
	m.loader.Style = lipgloss.NewStyle().Foreground(activePanelColor)
}

// sortList sorts m.tasks by labels and refreshes the visible list.
func (m *UI) sortList() {
	sort.Slice(m.tasks, func(i, j int) bool {
		iLabelsKey := strings.Join(m.formatLabelsForSorting(m.tasks[i].Labels), "-")
		jLabelsKey := strings.Join(m.formatLabelsForSorting(m.tasks[j].Labels), "-")

		if iLabelsKey == jLabelsKey {
			return false
		}

		return iLabelsKey < jLabelsKey
	})

	m.refreshList()
}

// isHiddenStatus reports whether a task in this status is hidden from the
// list by default (successful/skipped tasks are noise once they're done).
func isHiddenStatus(status TaskStatus) bool {
	switch status {
	case TaskStatusDone, TaskStatusCached, TaskStatusSkipped:
		return true
	default:
		return false
	}
}

// refreshList rebuilds the visible list from m.tasks, applying the
// show-all/hide-finished filter and preserving the current selection.
func (m *UI) refreshList() {
	items := make([]list.Item, 0, len(m.tasks))
	for _, t := range m.tasks {
		if m.showAll || !isHiddenStatus(t.Status) {
			items = append(items, t)
		}
	}

	current := m.findCurrentSelection(items)
	m.list.SetItems(items)
	m.list.Select(current)
}

// formatLabelsForSorting formats labels for sorting purposes
func (m *UI) formatLabelsForSorting(labels []processor.Label) []string {
	var formattedLabels []string
	for _, label := range labels {
		formattedLabels = append(formattedLabels, fmt.Sprintf("%s:%s", label.Key, label.Value))
	}
	return formattedLabels
}

// findCurrentSelection finds the index of the currently selected item
func (m *UI) findCurrentSelection(items []list.Item) int {
	if m.lastSelected == nil {
		return 0
	}

	for i, item := range items {
		if item.(TaskMsg).Name == m.lastSelected.(TaskMsg).Name {
			return i
		}
	}
	return 0
}

// renderStatus renders the current pipeline status
func (m *UI) renderStatus() string {
	switch m.status {
	case TaskStatusCached:
		return pipelineCachedStyle.Render("CACHED")
	case TaskStatusDone:
		return pipelineOkStyle.Render("SUCCESS")
	case TaskStatusFailed:
		return pipelineFailedStyle.Render("FAILED")
	case TaskStatusWaiting:
		return pipelineWaitingStyle.Render("INITIALIZING")
	case TaskStatusRunning:
		return pipelineRunningStyle.Render("RUNNING")
	}
	return ""
}

// Init initializes the UI model
func (m UI) Init() tea.Cmd {
	return nil
}

// Update handles all UI updates and events
func (m UI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	start := time.Now()
	itemCount := len(m.list.Items())
	defer func() {
		debugLog("update type=%T items=%d elapsed=%s", msg, itemCount, time.Since(start))
	}()

	var cmds []tea.Cmd

	// The top-level loader is only ever visible on the initial "waiting"
	// screen, before any task has been selected (see View). Updating it on
	// every message regardless wastes work once the list is populated and
	// receiving frequent stats/progress messages.
	if m.lastSelected == nil {
		loader, cmd := m.loader.Update(msg)
		m.loader = loader
		cmds = append(cmds, cmd)
	}

	m.logger.V(7).Info("tui update msg", "msg", msg)

	switch msg := msg.(type) {
	case tea.MouseMsg:
		cmds = append(cmds, m.handleMouseMessage(msg))
	case tea.KeyPressMsg:
		return m.handleKeyMessage(msg)
	case PipelineDoneMsg:
		cmds = append(cmds, m.handlePipelineDone(msg)...)
	case TaskMsg:
		cmds = append(cmds, m.handleTaskMessage(msg)...)
	case TickMsg:
		cmds = append(cmds, m.handleTick(msg)...)
	case stopwatch.StartStopMsg, stopwatch.ResetMsg, stopwatch.TickMsg:
		cmds = append(cmds, m.handleStopwatchControl(msg)...)
	case ResourceStatsMsg:
		m.handleResourceStats(msg)
	case PullProgressMsg:
		m.handlePullProgress(msg)
	case tea.WindowSizeMsg:
		cmds = append(cmds, m.handleWindowResize(msg)...)
	case DebugShellDoneMsg:
		if msg.Err != nil {
			m.logger.Error(msg.Err, "debug shell exited with an error", "task", msg.Name)
		}
	}

	m.updateLastSelected()
	return m, tea.Batch(cmds...)
}

// handlePipelineDone handles pipeline completion
func (m *UI) handlePipelineDone(msg PipelineDoneMsg) []tea.Cmd {
	if m.status == TaskStatusWaiting {
		return []tea.Cmd{tea.Quit}
	}

	m.status = msg.Status
	m.exitErr = msg.Error

	if msg.Status == TaskStatusFailed {
		for i, t := range m.tasks {
			if t.Status == TaskStatusRunning {
				m.tasks[i] = t.WithStatus(TaskStatusFailed)
			}
		}
		m.refreshList()
	}

	return nil
}

// handleTaskMessage handles task status updates
func (m *UI) handleTaskMessage(msg TaskMsg) []tea.Cmd {
	m.mu.Lock()
	defer m.mu.Unlock()

	var cmds []tea.Cmd

	existing, idx, err := m.getTaskMsg(msg.Name)
	// New task
	if err != nil {
		msg.ready = true
		msg.listWidth = m.list.Width()
		msg.listHeight = m.list.Height()
		msg.pullImageProgress.SetWidth(pullImageProgressWidth(msg.listWidth))

		// Initialize viewport dimensions
		if msg.viewport != nil {
			m.updateViewportDimensions(&msg)
		}

		cmds = append(cmds,
			msg.timer.Init(),
			msg.loader.Tick,
		)

		m.tasks = append(m.tasks, msg.WithStatus(msg.Status))
		m.sortList()
	} else {
		// Update existing task
		item := existing
		if msg.Status != TaskStatusRunning {
			item.Stats = ResourceStats{}
			item.Pull = PullProgress{}
			item.Context = msg.Context
			cmds = append(cmds, item.timer.Stop())
		}

		if item.Status == TaskStatusRunning && msg.Status != TaskStatusRunning && m.debugShell != nil {
			m.writeDebugShellHint(&item)
		}

		m.tasks[idx] = item.WithStatus(msg.Status)
		m.refreshList()
	}

	if msg.Status == TaskStatusRunning {
		m.status = TaskStatusRunning
	}

	return cmds
}

// getTaskMsg retrieves a task message by name along with its index in m.tasks
func (m *UI) getTaskMsg(name string) (TaskMsg, int, error) {
	for i, task := range m.tasks {
		if task.Name == name {
			return task, i, nil
		}
	}
	return TaskMsg{}, -1, fmt.Errorf("no such task: %s", name)
}

// writeDebugShellHint writes a hint into the task's viewport telling the user
// they can press the debug shell key now that the task has finished. Tasks
// can transition out of "running" more than once (e.g. across retries), so
// this only fires once per task to avoid spamming the viewport.
func (m *UI) writeDebugShellHint(item *TaskMsg) {
	if item.shellHintShown {
		return
	}
	item.shellHintShown = true

	fmt.Fprintf(item, "\n%s\n", durationStyle.Render(fmt.Sprintf("Press '%s' to start a shell", KeyDebugShell)))
	item.Flush()
}

// handleMouseMessage handles mouse interactions
func (m *UI) handleMouseMessage(msg tea.MouseMsg) tea.Cmd {
	var cmd tea.Cmd

	switch mmsg := msg.(type) {
	case tea.MouseWheelMsg:
		if m.activePanel == PanelList {
			switch mmsg.Button {
			case tea.MouseWheelUp:
				m.list.CursorUp()
			case tea.MouseWheelDown:
				m.list.CursorDown()
			}
		} else {
			cmd = m.updateSelectedViewport(msg)
		}
	default:
		if m.activePanel != PanelList {
			cmd = m.updateSelectedViewport(msg)
		}
	}

	return cmd
}

// handleKeyMessage handles keyboard input
func (m UI) handleKeyMessage(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case KeyQuit:
	case KeyQ:
		return m, tea.Quit
	case KeyTab:
		m.toggleActivePanel()
		return m, nil
	case KeyShowAll:
		m.showAll = !m.showAll
		m.refreshList()
		return m, nil
	case KeyDebugShell:
		if m.activePanel == PanelDetails {
			if cmd := m.openDebugShell(); cmd != nil {
				return m, cmd
			}
		}
	}

	if m.activePanel == PanelList {
		return m.handleListPanelKeys(msg)
	}
	return m, m.updateSelectedViewport(msg)
}

// openDebugShell spawns a debug shell for the currently selected task, if the task has
// finished (so its final buildkit state is available) and a debug shell factory has been
// registered. It uses tea.Exec so bubbletea releases the terminal for the duration of the
// interactive shell and restores it to the TUI once the shell exits.
func (m *UI) openDebugShell() tea.Cmd {
	if m.debugShell == nil || m.lastSelected == nil {
		return nil
	}

	task, ok := m.lastSelected.(TaskMsg)
	if !ok || task.Status == TaskStatusRunning || task.Status == TaskStatusWaiting {
		return nil
	}

	execCmd, err := m.debugShell(task.Context)
	if err != nil {
		m.logger.Error(err, "failed to prepare debug shell", "task", task.Name)
		return nil
	}

	return tea.Exec(execCmd, func(err error) tea.Msg {
		return DebugShellDoneMsg{Name: task.Name, Err: err}
	})
}

// toggleActivePanel switches between the list and details panels
func (m *UI) toggleActivePanel() {
	if m.activePanel == PanelList {
		m.activePanel = PanelDetails
	} else {
		m.activePanel = PanelList
	}
}

// handleListPanelKeys handles keyboard input for the list panel
func (m UI) handleListPanelKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case KeyFilter:
		m.list.SetFilterState(list.Filtering)
		m.list.FilterInput.Focus()
	case KeyEscape:
		m.list.FilterInput.Reset()
		m.list.SetFilterState(list.Unfiltered)
	default:
		if m.list.FilterState() > 0 {
			m.list.FilterInput, cmd = m.list.FilterInput.Update(msg)
			filterText := m.list.FilterInput.Value()
			m.list.SetFilterText(filterText)
			m.list.SetFilterState(list.Filtering)
		} else {
			m.list, cmd = m.list.Update(msg)
		}
	}

	return m, cmd
}

// updateSelectedViewport updates the viewport for the selected item
func (m *UI) updateSelectedViewport(msg tea.Msg) tea.Cmd {
	if m.lastSelected == nil {
		return nil
	}

	name := m.lastSelected.(TaskMsg).Name
	for i, t := range m.tasks {
		if t.Name == name {
			viewport, cmd := t.viewport.Update(msg)
			t.viewport = &viewport
			m.tasks[i] = t

			m.refreshList()
			return cmd
		}
	}

	return nil
}

// handleWindowResize handles window resize events
func (m *UI) handleWindowResize(msg tea.WindowSizeMsg) []tea.Cmd {
	m.height = msg.Height
	m.width = msg.Width

	if m.width < AlignHorizontalBreakpoint {
		listHeight := float64(m.height) * ListHeightPercentage / 100
		m.list.SetSize(m.width, int(listHeight)-LayoutAreaHeightNarrow)
	} else {
		listWidth := float64(m.width) * ListWidthPercentage / 100
		m.list.SetSize(int(listWidth), m.height-LayoutAreaHeight)
	}

	for i, t := range m.tasks {
		t.listWidth = m.list.Width()
		t.listHeight = m.list.Height()
		t.pullImageProgress.SetWidth(pullImageProgressWidth(t.listWidth))
		m.tasks[i] = t
	}
	m.refreshList()

	return nil
}

// pullImageProgressWidth derives a sensible progress bar width from the list width.
func pullImageProgressWidth(listWidth int) int {
	return max(listWidth-StatusColumnWidth-2, 10)
}

// handlePullProgress updates the pull progress of a running task
func (m *UI) handlePullProgress(msg PullProgressMsg) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, t := range m.tasks {
		if t.Name == msg.Name {
			t.Pull = PullProgress{Current: msg.Current, Total: msg.Total}
			m.tasks[i] = t
			break
		}
	}
	m.refreshList()
}

// handleResourceStats updates the Stats field of a running task
func (m *UI) handleResourceStats(msg ResourceStatsMsg) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, t := range m.tasks {
		if t.Name == msg.Name {
			t.Stats = msg.Stats
			m.tasks[i] = t
			break
		}
	}
	m.refreshList()
}

// handleStopwatchControl routes stopwatch start/stop/reset messages to every
// task's timer. stopwatch.Model.Update ignores messages whose ID doesn't
// match its own, so broadcasting to all items is safe. The returned command
// must be propagated (it's what re-schedules the next tick for the timer
// that actually owns this message) or every stopwatch freezes after its
// first tick.
func (m *UI) handleStopwatchControl(msg tea.Msg) []tea.Cmd {
	var cmds []tea.Cmd
	for i, t := range m.tasks {
		timer, cmd := t.timer.Update(msg)
		t.timer = timer
		m.tasks[i] = t
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	m.refreshList()
	return cmds
}

// handleTick handles tick messages for animations
func (m *UI) handleTick(msg TickMsg) []tea.Cmd {
	for i, t := range m.tasks {
		loader, _ := t.loader.Update(t.loader.Tick())
		t.loader = loader
		m.tasks[i] = t
	}
	m.refreshList()
	return nil
}

// updateLastSelected updates the last selected item
func (m *UI) updateLastSelected() {
	if selectedItem := m.list.SelectedItem(); selectedItem != nil {
		task := selectedItem.(TaskMsg)
		if task.viewport != nil {
			// Initialize viewport dimensions if not set
			if task.viewport.Width == 0 || task.viewport.Height == 0 {
				m.updateViewportDimensions(&task)
			}
			m.lastSelected = selectedItem
		}
	}
}

// View renders the UI
func (m UI) View() tea.View {
	start := time.Now()
	itemCount := len(m.list.Items())
	defer func() {
		debugLog("view items=%d elapsed=%s", itemCount, time.Since(start))
	}()

	m.logger.Info("tui view", "height", m.height, "width", m.width, "last", m.lastSelected)

	var content string
	if m.lastSelected == nil || m.height == 0 || m.width == 0 {
		content = m.loader.View()
	} else {
		content = m.renderMainLayout()
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// renderMainLayout renders the main UI layout
func (m UI) renderMainLayout() string {
	t0 := time.Now()
	headerPanel := m.renderHeaderPanel()
	t1 := time.Now()
	listPanel := m.renderListPanel()
	t2 := time.Now()
	pagerPanel := m.renderPagerPanel()
	t3 := time.Now()
	bottomPanel := m.renderBottomPanel()
	t4 := time.Now()
	debugLog("renderMainLayout items=%d header=%s list=%s pager=%s bottom=%s",
		len(m.list.Items()), t1.Sub(t0), t2.Sub(t1), t3.Sub(t2), t4.Sub(t3))

	// Stack panels vertically if the terminal is too narrow
	if m.width < AlignHorizontalBreakpoint {
		return lipgloss.JoinVertical(
			lipgloss.Top,
			listPanel,
			headerPanel,
			pagerPanel,
			bottomPanel,
		)
	}

	// Otherwise use horizontal layout
	return lipgloss.JoinVertical(
		lipgloss.Top,
		headerPanel,
		lipgloss.JoinHorizontal(lipgloss.Top, listPanel, pagerPanel),
		bottomPanel,
	)
}

// renderHeaderPanel renders the combined header panel
func (m UI) renderHeaderPanel() string {
	var listStyle lipgloss.Style
	var pagerStyle lipgloss.Style
	var activeStyle = lipgloss.NewStyle().Foreground(activePanelColor)
	var pagerHeader string

	if m.activePanel == PanelList {
		listStyle = listStyle.Foreground(activePanelColor)
		pagerStyle = listStyle.Foreground(inactivePanelColor)
	} else {
		listStyle = listStyle.Foreground(inactivePanelColor)
		pagerStyle = listStyle.Foreground(activePanelColor)
	}

	if m.width < AlignHorizontalBreakpoint {
		tab := activeStyle.Render(" ⇅ ")
		line := strings.Repeat("─", max(0, (m.width-4)/2))

		pagerHeader = fmt.Sprintf("%s%s%s%s",
			pagerStyle.Render("┌"),
			pagerStyle.Render(line),
			pagerStyle.Render(tab),
			pagerStyle.Render(line),
		)

		return lipgloss.JoinVertical(lipgloss.Top, pagerHeader)
	}

	listHeader := listStyle.
		Render(strings.Repeat("─", max(0, m.list.Width()-2)))

	if m.lastSelected != nil {
		task := m.lastSelected.(TaskMsg)
		headerWidth := max(0, task.viewport.Width-10-lipgloss.Width(task.GetName()))
		pagerHeader = fmt.Sprintf("%s %s %s %s",
			pagerStyle.Render("─── ·"),
			topTitleStyle.Render(task.GetName()),
			pagerStyle.Render("·"),
			pagerStyle.Render(strings.Repeat("─", headerWidth)),
		)
	}

	tab := activeStyle.Render(" ⇆ ")
	return lipgloss.JoinHorizontal(lipgloss.Top, listHeader, tab, pagerHeader)
}

// renderListPanel renders the left list panel
func (m UI) renderListPanel() string {
	listPanelContent := []string{m.list.View()}

	if m.list.FilterState() > 0 {
		listPanelContent = append(listPanelContent, m.list.FilterInput.View())
		m.list.SetHeight(m.list.Height() - FilterInputHeightOffset)
	}

	var style lipgloss.Style
	if m.activePanel == PanelList {
		style = listStyle.BorderForeground(activePanelColor)
	} else {
		style = listStyle.BorderForeground(inactivePanelColor)
	}

	if m.width < AlignHorizontalBreakpoint {
		// In vertical layout the list panel isn't flanked by the pager, so
		// mirror the right border on the left for a symmetric box.
		style = style.Border(lipgloss.NormalBorder(), false, true, true, true)
	}

	header := m.renderListHeader()
	list := style.
		Height(m.list.Height()).
		Width(m.list.Width()).
		Render(lipgloss.JoinVertical(lipgloss.Top, listPanelContent...))

	return lipgloss.JoinVertical(lipgloss.Top, header, list)
}

func (m UI) renderListHeader() string {
	listWidth := m.list.Width() - StatusColumnWidth - 2 // Account for status and padding
	nameWidth := int(float64(listWidth) * NameColumnPercent / 100)
	labelsWidth := int(float64(listWidth) * LabelsColumnPercent / 100)
	cpuWidth := int(float64(listWidth) * CPUColumnPercent / 100)
	memWidth := int(float64(listWidth) * MemColumnPercent / 100)
	netWidth := int(float64(listWidth) * NetColumnPercent / 100)
	durationWidth := int(float64(listWidth) * DurationColumnPercent / 100)

	headerStyle := listHeaderStyle
	if m.width < AlignHorizontalBreakpoint {
		// Close off the top of the box and mirror the list panel's left
		// border added in vertical layout. The extra top border line adds
		// a second line to the block, so bump the height cap to match.
		headerStyle = headerStyle.Border(lipgloss.NormalBorder(), true, true, false, true).MaxHeight(2)
	}

	return headerStyle.Width(m.list.Width()).Render(fmt.Sprintf("  %s %s %s %s %s %s",
		listColumnStyle.Width(nameWidth).Render(ellipsis("TASK", nameWidth)),
		listColumnStyle.Width(labelsWidth).Render("LABELS"),
		listColumnStyle.Width(cpuWidth).Align(lipgloss.Right).Render("CPU"),
		listColumnStyle.Width(memWidth).Align(lipgloss.Right).Render("MEM"),
		listColumnStyle.Width(netWidth).Align(lipgloss.Right).Render("NET"),
		listColumnStyle.Width(durationWidth).Align(lipgloss.Right).Render("DUR"),
	))
}

func (m UI) renderPagerPanel() string {
	task := m.lastSelected.(TaskMsg)

	m.updatePanelStyles()
	m.updateViewportDimensions(&task)

	detailsContent := m.buildPagerContent(task)

	style := viewportStyle
	if m.width < AlignHorizontalBreakpoint {
		// In vertical layout the pager isn't flanked by anything on the
		// right, so add a right border to close off the box.
		style = style.Border(lipgloss.NormalBorder(), false, true, true, true)
	}

	return lipgloss.JoinVertical(
		lipgloss.Top,
		style.Render(lipgloss.JoinVertical(lipgloss.Top, detailsContent...)),
	)
}

// updatePanelStyles updates the styles based on the active panel
func (m *UI) updatePanelStyles() {
	if m.activePanel == PanelList {
		topStyle = topStyle.Foreground(inactivePanelColor)
		topTitleStyle = topTitleStyle.Foreground(inactivePanelColor)
		viewportStyle = viewportStyle.BorderForeground(inactivePanelColor)
		m.lastSelected.(TaskMsg).viewport.Styles.LineNumber = lineNumberInactiveStyle
		return
	}

	topStyle = topStyle.Foreground(activePanelColor)
	topTitleStyle = newStyle()
	viewportStyle = viewportStyle.BorderForeground(activePanelColor)
	m.lastSelected.(TaskMsg).viewport.Styles.LineNumber = lineNumberActiveStyle
}

// updateViewportDimensions updates the viewport dimensions
func (m *UI) updateViewportDimensions(task *TaskMsg) {
	// Set viewport width based on layout
	if m.width < AlignHorizontalBreakpoint {
		// In vertical layout, viewport takes full width, minus the left
		// and right borders drawn around the pager panel.
		task.viewport.Width = m.width - 2
		// Height is reduced by list height and bottom panel
		task.viewport.Height = m.height - m.list.Height() - LayoutAreaHeightNarrow
	} else {
		// In horizontal layout, viewport takes remaining width
		task.viewport.Width = m.width - m.list.Width()
		task.viewport.Height = m.height - LayoutAreaHeight + 1
	}

	if task.LabelsAsString() != "" {
		task.viewport.Height -= LabelsHeightOffset
	}
}

// buildPagerContent builds the content for the details panel
func (m UI) buildPagerContent(task TaskMsg) []string {
	var content []string

	// Add labels if present
	if labels := task.LabelsAsString(); labels != "" {
		content = append(content, lipgloss.NewStyle().
			Width(task.viewport.Width).
			Render(labels))
	}

	// Add viewport content
	content = append(content, task.viewport.View())

	return content
}

// renderBottomPanel renders the bottom status panel
func (m UI) renderBottomPanel() string {
	status := m.renderStatus()
	scrollPercentage := scrollPercentageStyle.Render(
		fmt.Sprintf("%3.f%%", m.lastSelected.(TaskMsg).viewport.ScrollPercent()*100))
	delimiter := helpDelimiterStyle.Render("│")

	helpWidth := max(0, m.width-lipgloss.Width(status)-lipgloss.Width(scrollPercentage)-lipgloss.Width(delimiter))
	m.help.SetWidth(helpWidth)

	return lipgloss.JoinHorizontal(
		lipgloss.Bottom,
		status,
		delimiter,
		lipgloss.NewStyle().Width(helpWidth).Render(m.help.View(uiKeyMap{})),
		scrollPercentage,
	)
}
