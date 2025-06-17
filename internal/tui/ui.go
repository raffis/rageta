package tui

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"slices"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-logr/logr"
	"github.com/raffis/rageta/internal/processor"
)

// Panel represents which panel is currently active
type Panel int8

const (
	// PanelList represents the left list panel
	PanelList Panel = iota
	// PanelDetails represents the right details panel
	PanelDetails
)

// UI dimensions and layout constants
const (
	ListWidthPercentage     = 30.0
	MinBottomPanelHeight    = 3
	FilterInputHeightOffset = 1
	TagsHeightOffset        = 1
	AlignHorizontal         = 130
)

// Keyboard shortcuts
const (
	KeyFilter = "/"
	KeyEscape = "esc"
	KeyTab    = "tab"
	KeyEnter  = "enter"
	KeyQuit   = "ctrl+c"
)

// UI represents the main Terminal User Interface model
type UI struct {
	list         list.Model
	loader       spinner.Model
	status       StepStatus
	scanInput    textinput.Model
	scanState    list.FilterState
	width        int
	height       int
	mu           *sync.Mutex
	logger       logr.Logger
	exitErr      error
	activePanel  Panel
	lastSelected list.Item
}

// TickMsg represents a tick message for animations
type TickMsg time.Time

// PipelineDoneMsg represents a message when the pipeline is complete
type PipelineDoneMsg struct {
	Status StepStatus
	Error  error
}

// NewUI creates a new UI instance with the given logger
func NewUI(logger logr.Logger) UI {
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		BorderForeground(activePanelColor).
		Border(lipgloss.BlockBorder(), false, false, false, true)

	delegate.ShowDescription = false

	ui := UI{
		status:      StepStatusWaiting,
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

// sortList sorts the list items by tags and start time
func (m *UI) sortList() {
	items := m.list.Items()
	sort.Slice(items, func(i, j int) bool {
		iTags := m.formatTagsForSorting(items[i].(StepMsg).Tags)
		jTags := m.formatTagsForSorting(items[j].(StepMsg).Tags)

		iTagsKey := strings.Join(iTags, "-")
		jTagsKey := strings.Join(jTags, "-")

		if iTagsKey == jTagsKey {
			return items[i].(StepMsg).started.Before(items[j].(StepMsg).started)
		}

		return iTagsKey < jTagsKey
	})

	current := m.findCurrentSelection(items)
	m.list.SetItems(items)
	m.list.Select(current)
}

// formatTagsForSorting formats tags for sorting purposes
func (m *UI) formatTagsForSorting(tags []processor.Tag) []string {
	var formattedTags []string
	for _, tag := range tags {
		formattedTags = append(formattedTags, fmt.Sprintf("%s:%s", tag.Key, tag.Value))
	}
	return formattedTags
}

// findCurrentSelection finds the index of the currently selected item
func (m *UI) findCurrentSelection(items []list.Item) int {
	if m.lastSelected == nil {
		return 0
	}

	for i, item := range items {
		if item.(StepMsg).Name == m.lastSelected.(StepMsg).Name {
			return i
		}
	}
	return 0
}

// getStepMsg retrieves a step message by name
func (m *UI) getStepMsg(name string) (StepMsg, error) {
	for _, step := range m.list.Items() {
		if v, ok := step.(StepMsg); ok && v.Name == name {
			return v, nil
		}
	}
	return StepMsg{}, fmt.Errorf("no such step: %s", name)
}

// renderStatus renders the current pipeline status
func (m *UI) renderStatus() string {
	switch m.status {
	case StepStatusDone:
		return pipelineOkStyle.Render("SUCCESS")
	case StepStatusFailed:
		return pipelineFailedStyle.Render("FAILED")
	case StepStatusWaiting:
		return pipelineWaitingStyle.Render("INITIALIZING")
	case StepStatusRunning:
		return pipelineWaitingStyle.Render("RUNNING")
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

	m.logger.V(1).Info("tui update msg", "msg", msg)

	switch msg := msg.(type) {
	case PipelineDoneMsg:
		cmds = append(cmds, m.handlePipelineDone(msg)...)
	case StepMsg:
		cmds = append(cmds, m.handleStepMessage(msg)...)
	case tea.MouseMsg:
		cmds = append(cmds, m.handleMouseMessage(msg))
	case tea.KeyMsg:
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
	m.status = msg.Status
	m.exitErr = msg.Error

	if msg.Status == StepStatusFailed {
		items := slices.Clone(m.list.Items())
		for i, listItem := range items {
			if item, ok := listItem.(StepMsg); ok && item.Status == StepStatusRunning {
				items[i] = item.WithStatus(StepStatusFailed)
			}
		}
		m.list.SetItems(items)
	}

	return nil
}

// handleStepMessage handles step status updates
func (m *UI) handleStepMessage(msg StepMsg) []tea.Cmd {
	m.mu.Lock()
	defer m.mu.Unlock()

	var cmds []tea.Cmd
	items := slices.Clone(m.list.Items())

	_, err := m.getStepMsg(msg.Name)
	if err != nil {
		// New step
		msg.ready = true
		msg.listWidth = m.list.Width()
		msg.listHeight = m.list.Height()

		// Initialize viewport dimensions
		if msg.viewport != nil {
			m.updateViewportDimensions(&msg)
		}

		cmds = append(cmds, msg.loader.Tick)

		m.list.InsertItem(-1, msg.WithStatus(msg.Status))
		m.sortList()
	} else {
		// Update existing step
		for i, listItem := range items {
			if item, ok := listItem.(StepMsg); ok && item.Name == msg.Name {
				items[i] = item.WithStatus(msg.Status)
			}
		}
		m.list.SetItems(items)
	}

	if msg.Status == StepStatusRunning {
		m.status = StepStatusRunning
	}

	return cmds
}

// handleMouseMessage handles mouse interactions
func (m *UI) handleMouseMessage(msg tea.MouseMsg) tea.Cmd {
	var cmd tea.Cmd

	if m.activePanel == PanelList {
		switch msg.Type {
		case tea.MouseWheelUp:
			m.list.CursorUp()
		case tea.MouseWheelDown:
			m.list.CursorDown()
		}
	} else {
		cmd = m.updateSelectedViewport(msg)
	}

	return cmd
}

// handleKeyMessage handles keyboard input
func (m UI) handleKeyMessage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case KeyQuit:
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
func (m UI) handleListPanelKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		if m.lastSelected != nil && listItem.(StepMsg).Name == m.lastSelected.(StepMsg).Name {
			viewport, cmd := m.lastSelected.(StepMsg).viewport.Update(msg)
			last := m.lastSelected.(StepMsg)
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

	if m.width < AlignHorizontal {
		listHeight := float64(m.height) * 50 / 100
		m.list.SetSize(m.width, int(listHeight)-MinBottomPanelHeight)
	} else {
		listWidth := float64(m.width) * ListWidthPercentage / 100
		m.list.SetSize(int(listWidth), m.height-MinBottomPanelHeight)
	}

	items := slices.Clone(m.list.Items())
	for i, listItem := range items {
		if item, ok := listItem.(StepMsg); ok {
			item.listWidth = m.list.Width()
			item.listHeight = m.list.Height()
			items[i] = item
		}
	}
	m.list.SetItems(items)

	return nil
}

// handleTick handles tick messages for animations
func (m *UI) handleTick(msg TickMsg) []tea.Cmd {
	items := slices.Clone(m.list.Items())
	for i, listItem := range items {
		if item, ok := listItem.(StepMsg); ok {
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
		step := selectedItem.(StepMsg)
		if step.viewport != nil {
			// Initialize viewport dimensions if not set
			if step.viewport.Width == 0 || step.viewport.Height == 0 {
				m.updateViewportDimensions(&step)
			}
			m.lastSelected = selectedItem
		}
	}
}

// View renders the UI
func (m UI) View() string {
	m.logger.Info("tUI view", "height", m.height, "width", m.width, "last", m.lastSelected)

	if m.lastSelected == nil || m.height == 0 || m.width == 0 {
		return m.loader.View()
	}

	return m.renderMainLayout()
}

// renderMainLayout renders the main UI layout
func (m UI) renderMainLayout() string {
	listPanel := m.renderListPanel()
	pagerPanel := m.renderPagerPanel()
	bottomPanel := m.renderBottomPanel()

	// Stack panels vertically if the terminal is too narrow
	if m.width < AlignHorizontal {
		return lipgloss.JoinVertical(
			lipgloss.Top,
			listPanel,
			pagerPanel,
			bottomPanel,
		)
	}

	// Otherwise use horizontal layout
	return lipgloss.JoinVertical(
		lipgloss.Top,
		lipgloss.JoinHorizontal(lipgloss.Top, listPanel, pagerPanel),
		bottomPanel,
	)
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

	return style.
		Height(m.list.Height()).
		Width(m.list.Width()).
		Render(lipgloss.JoinVertical(lipgloss.Top, listPanelContent...))
}

// renderPagerPanel renders the right details panel
func (m UI) renderPagerPanel() string {
	step := m.lastSelected.(StepMsg)

	m.updatePanelStyles()
	m.updateViewportDimensions(&step)

	detailsContent := m.buildPagerContent(step)
	detailsHeader := m.buildPagerHeader(step)

	return lipgloss.JoinVertical(
		lipgloss.Top,
		detailsHeader,
		viewportStyle.Render(lipgloss.JoinVertical(lipgloss.Top, detailsContent...)),
	)
}

// updatePanelStyles updates the styles based on the active panel
func (m *UI) updatePanelStyles() {
	if m.activePanel == PanelList {
		topStyle = topStyle.Foreground(inactivePanelColor)
		topTitleStyle = topTitleStyle.Foreground(inactivePanelColor)
		viewportStyle = viewportStyle.BorderForeground(inactivePanelColor)
		m.lastSelected.(StepMsg).viewport.Styles.LineNumber = lineNumberInactiveStyle
		return
	}

	topStyle = topStyle.Foreground(activePanelColor)
	topTitleStyle = newStyle()
	viewportStyle = viewportStyle.BorderForeground(activePanelColor)
	m.lastSelected.(StepMsg).viewport.Styles.LineNumber = lineNumberActiveStyle
}

// updateViewportDimensions updates the viewport dimensions
func (m *UI) updateViewportDimensions(step *StepMsg) {
	// Set viewport width based on layout
	if m.width < AlignHorizontal {
		// In vertical layout, viewport takes full width
		step.viewport.Width = m.width
		// Height is reduced by list height and bottom panel
		step.viewport.Height = m.height - m.list.Height() - MinBottomPanelHeight
	} else {
		// In horizontal layout, viewport takes remaining width
		step.viewport.Width = m.width - m.list.Width()
		step.viewport.Height = m.height - MinBottomPanelHeight
	}

	if step.TagsAsString() != "" {
		step.viewport.Height -= TagsHeightOffset
	}
}

// buildPagerContent builds the content for the details panel
func (m UI) buildPagerContent(step StepMsg) []string {
	var content []string

	// Add tags if present
	if tags := step.TagsAsString(); tags != "" {
		content = append(content, lipgloss.NewStyle().
			Width(step.viewport.Width).
			Render(tags))
	}

	// Add viewport content
	content = append(content, step.viewport.View())

	return content
}

// buildPagerHeader builds the header for the details panel
func (m UI) buildPagerHeader(step StepMsg) string {
	headerWidth := step.viewport.Width - 9 - lipgloss.Width(step.GetName())
	return lipgloss.NewStyle().
		Width(step.viewport.Width).
		Render(fmt.Sprintf("%s %s %s %s",
			topStyle.Render("┌─ ·"),
			topTitleStyle.Render(step.GetName()),
			topStyle.Render("· "),
			topStyle.Render(strings.Repeat("─", headerWidth))))
}

// renderBottomPanel renders the bottom status panel
func (m UI) renderBottomPanel() string {
	status := m.renderStatus()
	scrollPercentage := scrollPercentageStyle.Render(
		fmt.Sprintf("%3.f%%", m.lastSelected.(StepMsg).viewport.ScrollPercent()*100))

	helpWidth := m.width - lipgloss.Width(status) - lipgloss.Width(scrollPercentage)

	return lipgloss.JoinHorizontal(
		lipgloss.Bottom,
		status,
		lipgloss.NewStyle().Width(helpWidth).Render(m.list.Help.View(m.list)),
		scrollPercentage,
	)
}
