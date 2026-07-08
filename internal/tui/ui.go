package tui

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"slices"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/go-logr/logr"
	"github.com/raffis/rageta/internal/processor"
)

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
	KeyFilter = "/"
	KeyEscape = "esc"
	KeyTab    = "tab"
	KeyEnter  = "enter"
	KeyQuit   = "ctrl+c"
	KeyQ      = "q"
)

type UI struct {
	list         list.Model
	loader       spinner.Model
	status       TaskStatus
	scanInput    textinput.Model
	width        int
	height       int
	mu           *sync.Mutex
	logger       logr.Logger
	exitErr      error
	activePanel  Panel
	lastSelected list.Item
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

	// Render the progress bar (via TaskMsg.Description) in the row that
	// would otherwise be the blank gap between items, instead of adding an
	// extra line: reporting Spacing=0 alongside the 2-line item height keeps
	// the total rows per item (2) identical to the previous single-line +
	// gap layout. The description row has no content of its own selection
	// state, so strip the left border the default styles inherit from the
	// title and keep only the matching left padding for alignment.
	delegate.ShowDescription = true
	delegate.SetSpacing(0)
	noBorderDescPadding := lipgloss.NewStyle().Padding(0, 0, 0, 2)
	delegate.Styles.SelectedDesc = noBorderDescPadding
	delegate.Styles.NormalDesc = noBorderDescPadding
	delegate.Styles.DimmedDesc = noBorderDescPadding

	ui := UI{
		status:      TaskStatusWaiting,
		list:        list.New(nil, delegate, 0, 0),
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

// sortList sorts the list items by labels and start time
func (m *UI) sortList() {
	items := m.list.Items()
	sort.Slice(items, func(i, j int) bool {
		iLabels := m.formatLabelsForSorting(items[i].(TaskMsg).Labels)
		jLabels := m.formatLabelsForSorting(items[j].(TaskMsg).Labels)

		iLabelsKey := strings.Join(iLabels, "-")
		jLabelsKey := strings.Join(jLabels, "-")

		if iLabelsKey == jLabelsKey {
			return items[i].(TaskMsg).started.Before(items[j].(TaskMsg).started)
		}

		return iLabelsKey < jLabelsKey
	})

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

// getTaskMsg retrieves a task message by name
func (m *UI) getTaskMsg(name string) (TaskMsg, error) {
	for _, task := range m.list.Items() {
		if v, ok := task.(TaskMsg); ok && v.Name == name {
			return v, nil
		}
	}
	return TaskMsg{}, fmt.Errorf("no such task: %s", name)
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
	var cmds []tea.Cmd

	// Update loader
	loader, cmd := m.loader.Update(msg)
	m.loader = loader
	cmds = append(cmds, cmd)

	m.logger.V(7).Info("tui update msg", "msg", msg)

	switch msg := msg.(type) {
	case PipelineDoneMsg:
		cmds = append(cmds, m.handlePipelineDone(msg)...)
	case TaskMsg:
		cmds = append(cmds, m.handleTaskMessage(msg)...)
	case ResourceStatsMsg:
		m.handleResourceStats(msg)
	case PullProgressMsg:
		m.handlePullProgress(msg)
	case tea.MouseMsg:
		cmds = append(cmds, m.handleMouseMessage(msg))
	case tea.KeyPressMsg:
		return m.handleKeyMessage(msg)
	case tea.WindowSizeMsg:
		cmds = append(cmds, m.handleWindowResize(msg)...)
	case TickMsg:
		cmds = append(cmds, m.handleTick(msg)...)
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
		items := slices.Clone(m.list.Items())
		for i, listItem := range items {
			if item, ok := listItem.(TaskMsg); ok && item.Status == TaskStatusRunning {
				items[i] = item.WithStatus(TaskStatusFailed)
			}
		}
		m.list.SetItems(items)
	}

	return nil
}

// handleTaskMessage handles task status updates
func (m *UI) handleTaskMessage(msg TaskMsg) []tea.Cmd {
	m.mu.Lock()
	defer m.mu.Unlock()

	var cmds []tea.Cmd
	items := slices.Clone(m.list.Items())

	_, err := m.getTaskMsg(msg.Name)
	if err != nil {
		// New task
		msg.ready = true
		msg.listWidth = m.list.Width()
		msg.listHeight = m.list.Height()
		msg.pullImageProgress.SetWidth(pullImageProgressWidth(msg.listWidth))

		// Initialize viewport dimensions
		if msg.viewport != nil {
			m.updateViewportDimensions(&msg)
		}

		cmds = append(cmds, func() tea.Msg { return msg.loader.Tick() })

		m.list.InsertItem(-1, msg.WithStatus(msg.Status))
		m.sortList()
	} else {
		// Update existing task
		for i, listItem := range items {
			if item, ok := listItem.(TaskMsg); ok && item.Name == msg.Name {
				if msg.Status != TaskStatusRunning {
					item.Stats = ResourceStats{}
					item.Pull = PullProgress{}
				}

				items[i] = item.WithStatus(msg.Status)
			}
		}
		m.list.SetItems(items)
	}

	if msg.Status == TaskStatusRunning {
		m.status = TaskStatusRunning
	}

	return cmds
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
	}

	if m.activePanel == PanelList {
		return m.handleListPanelKeys(msg)
	}
	return m, m.updateSelectedViewport(msg)
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
	items := slices.Clone(m.list.Items())
	for i, listItem := range items {
		if m.lastSelected != nil && listItem.(TaskMsg).Name == m.lastSelected.(TaskMsg).Name {
			viewport, cmd := m.lastSelected.(TaskMsg).viewport.Update(msg)
			last := m.lastSelected.(TaskMsg)
			last.viewport = &viewport
			items[i] = last

			m.list.SetItems(items)
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

	items := slices.Clone(m.list.Items())
	for i, listItem := range items {
		if item, ok := listItem.(TaskMsg); ok {
			item.listWidth = m.list.Width()
			item.listHeight = m.list.Height()
			item.pullImageProgress.SetWidth(pullImageProgressWidth(item.listWidth))
			items[i] = item
		}
	}
	m.list.SetItems(items)

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

	items := slices.Clone(m.list.Items())
	for i, listItem := range items {
		if item, ok := listItem.(TaskMsg); ok && item.Name == msg.Name {
			item.Pull = PullProgress{Current: msg.Current, Total: msg.Total}
			items[i] = item
			break
		}
	}
	m.list.SetItems(items)
}

// handleResourceStats updates the Stats field of a running task
func (m *UI) handleResourceStats(msg ResourceStatsMsg) {
	m.mu.Lock()
	defer m.mu.Unlock()

	items := slices.Clone(m.list.Items())
	for i, listItem := range items {
		if item, ok := listItem.(TaskMsg); ok && item.Name == msg.Name {
			item.Stats = msg.Stats
			items[i] = item
			break
		}
	}
	m.list.SetItems(items)
}

// handleTick handles tick messages for animations
func (m *UI) handleTick(msg TickMsg) []tea.Cmd {
	items := slices.Clone(m.list.Items())
	for i, listItem := range items {
		if item, ok := listItem.(TaskMsg); ok {
			loader, _ := item.loader.Update(item.loader.Tick())
			item.loader = loader
			items[i] = item
		}
	}
	m.list.SetItems(items)
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
	headerPanel := m.renderHeaderPanel()
	listPanel := m.renderListPanel()
	pagerPanel := m.renderPagerPanel()
	bottomPanel := m.renderBottomPanel()

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

	helpWidth := m.width - lipgloss.Width(status) - lipgloss.Width(scrollPercentage)

	return lipgloss.JoinHorizontal(
		lipgloss.Bottom,
		status,
		lipgloss.NewStyle().Width(helpWidth).Render(m.list.Help.View(m.list)),
		scrollPercentage,
	)
}
