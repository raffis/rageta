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
)

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
	activePanel  int8
	lastSelected list.Item
}

func (m *UI) sortList() {
	items := m.list.Items()
	sort.Slice(items, func(i, j int) bool {
		var iTags, jTags []string
		for _, tag := range items[i].(StepMsg).Tags {
			iTags = append(iTags, fmt.Sprintf("%s:%s", tag.Key, tag.Value))
		}
		for _, tag := range items[j].(StepMsg).Tags {
			jTags = append(jTags, fmt.Sprintf("%s:%s", tag.Key, tag.Value))
		}

		iTagsKey := strings.Join(iTags, "-")
		jTagsKey := strings.Join(jTags, "-")

		if iTagsKey == jTagsKey {
			return items[i].(StepMsg).started.Before(items[j].(StepMsg).started)
		}

		return iTagsKey < jTagsKey
	})

	var current int
	for i, item := range items {
		if m.lastSelected != nil && item.(StepMsg).Name == m.lastSelected.(StepMsg).Name {
			current = i
			break
		}
	}

	m.list.SetItems(items)
	m.list.Select(current)
}

func (m *UI) getStepMsg(name string) (StepMsg, error) {
	for _, step := range m.list.Items() {
		if v, ok := step.(StepMsg); ok && v.Name == name {
			return step.(StepMsg), nil
		}
	}

	return StepMsg{}, fmt.Errorf("no such step: %s", name)
}

func NewUI(logger logr.Logger) UI {
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.BorderForeground(activePanelColor).Border(lipgloss.BlockBorder(), false, false, false, true)

	delegate.ShowDescription = false
	m := UI{
		status: StepStatusWaiting,
		list:   list.New(nil, delegate, 0, 0),
		mu:     &sync.Mutex{},
	}

	m.list.SetShowTitle(false)
	m.list.SetShowStatusBar(false)
	m.list.SetShowHelp(false)
	m.list.SetShowFilter(false)
	m.list.SetFilteringEnabled(false)
	m.list.Styles.PaginationStyle = listPaginatorStyle

	scanInput := textinput.New()
	scanInput.Prompt = "Filter: "
	scanInput.CharLimit = 64
	scanInput.Focus()
	m.scanInput = scanInput
	m.logger = logger
	m.loader = spinner.New()
	m.loader.Spinner = spinner.Dot
	m.loader.Style = lipgloss.NewStyle().Foreground(activePanelColor)

	return m
}

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

type TickMsg time.Time

type PipelineDoneMsg struct {
	Status StepStatus
	Error  error
}

func (m UI) Init() tea.Cmd {
	return nil
}

func (m UI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	l, c := m.loader.Update(msg)
	m.loader = l
	cmds = append(cmds, c)

	m.logger.V(1).Info("tui update msg", "msg", msg)

	newItems := slices.Clone(m.list.Items())

	switch msg := msg.(type) {
	case PipelineDoneMsg:
		m.status = msg.Status
		if msg.Status == StepStatusFailed {
			for i, listItem := range m.list.Items() {
				if item, ok := listItem.(StepMsg); ok {
					if item.Status == StepStatusRunning {
						newItems[i] = item.WithStatus(StepStatusFailed)
					}
				}
			}
		}

		m.exitErr = msg.Error
	case StepMsg:
		m.mu.Lock()
		defer m.mu.Unlock()

		_, err := m.getStepMsg(msg.Name)
		if err != nil {
			msg.ready = true
			msg.listWidth = m.list.Width()
			msg.listHeight = m.list.Height()
			cmds = append(cmds, msg.loader.Tick)

			m.list.InsertItem(-1, msg.WithStatus(msg.Status))
			m.sortList()
			newItems = slices.Clone(m.list.Items())
		} else {
			for i, listItem := range m.list.Items() {
				if item, ok := listItem.(StepMsg); ok {
					if item.Name == msg.Name {
						newItems[i] = item.WithStatus(msg.Status)
					}
				}
			}
		}

		if msg.Status == StepStatusRunning {
			m.status = StepStatusRunning
		}
	case tea.MouseMsg:
		if m.activePanel == 0 {
			if msg.Type == tea.MouseWheelUp {
				m.list.CursorUp()
			}

			if msg.Type == tea.MouseWheelDown {
				m.list.CursorDown()
			}
		} else {
			for i, listItem := range m.list.Items() {
				if listItem.(StepMsg).Name == m.lastSelected.(StepMsg).Name {
					viewport, cmd := m.lastSelected.(StepMsg).viewport.Update(msg)
					cmds = append(cmds, cmd)
					last := m.lastSelected.(StepMsg)
					last.viewport = &viewport
					newItems[i] = last
				}
			}
		}
	case tea.KeyMsg:
		switch {
		case msg.String() == "ctrl+c":
			return m, tea.Quit
		case msg.String() == "tab":
			if m.activePanel == 0 {
				m.activePanel = 1
			} else {
				m.activePanel = 0
			}
		case m.activePanel == 0:
			switch msg.String() {
			case "/":
				m.list.SetFilterState(list.Filtering)
				m.list.FilterInput.Focus()
			case "esc":
				m.list.FilterInput.Reset()
				m.list.SetFilterState(list.Unfiltered)
			default:
				if m.list.FilterState() > 0 {
					m.list.FilterInput, cmd = m.list.FilterInput.Update(msg)
					m.list.SetFilterText(m.list.FilterInput.Value())
				} else {
					m.list, cmd = m.list.Update(msg)
				}
			}
		case m.activePanel == 1:
			switch msg.String() {
			case "/":
				m.scanState = list.Filtering
				m.scanInput.Focus()
			case "esc":
				m.scanInput.Reset()
				m.scanState = list.Unfiltered
				m.list.SelectedItem().(StepMsg).viewport.ScanAfter("")
			case "enter":
				m.list.SelectedItem().(StepMsg).viewport.ScanAfter(m.scanInput.Value())
			default:
				if m.scanState > 0 {
					m.scanInput, cmd = m.scanInput.Update(msg)
				} else {
					for i, listItem := range m.list.Items() {
						if listItem.(StepMsg).Name == m.lastSelected.(StepMsg).Name {
							viewport, cmd := m.lastSelected.(StepMsg).viewport.Update(msg)
							cmds = append(cmds, cmd)
							last := m.lastSelected.(StepMsg)
							last.viewport = &viewport
							newItems[i] = last
						}
					}
				}
			}
		}
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
		listWidth := float64(m.width) / 100 * 30
		m.list.SetSize(int(listWidth), m.height-3)

		for i, listItem := range m.list.Items() {
			if item, ok := listItem.(StepMsg); ok {
				item.listWidth = m.list.Width()
				item.listHeight = m.list.Height()
				newItems[i] = item
			}
		}
	case TickMsg:
		for i, listItem := range m.list.Items() {
			if item, ok := listItem.(StepMsg); ok {
				loader, _ := item.loader.Update(item.loader.Tick())
				item.loader = loader
				newItems[i] = item
			}
		}
	}

	m.list.SetItems(newItems)
	if i := m.list.SelectedItem(); i != nil && i.(StepMsg).viewport != nil {
		m.lastSelected = m.list.SelectedItem()
	}

	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m UI) View() string {
	m.logger.Info("tUI view", "height", m.height, "width", m.width, "last", m.lastSelected)
	if m.lastSelected == nil || m.height == 0 || m.width == 0 {
		return m.loader.View()
	}

	listPanel := []string{
		m.list.View(),
	}

	if m.list.FilterState() > 0 {
		listPanel = append(listPanel, m.list.FilterInput.View())
		m.list.SetHeight(m.list.Height() + -1)
	}

	step := m.lastSelected.(StepMsg)

	if m.activePanel == 0 {
		listStyle = listStyle.BorderForeground(activePanelColor)
		topStyle = topStyle.Foreground(inactivePanelColor)
		topTitleStyle = topTitleStyle.Foreground(inactivePanelColor)
		viewportStyle = viewportStyle.BorderForeground(inactivePanelColor)
		step.viewport.Styles.LineNumber = lineNumberInactiveStyle
	} else {
		listStyle = listStyle.BorderForeground(inactivePanelColor)
		topStyle = topStyle.Foreground(activePanelColor)
		topTitleStyle = newStyle()
		viewportStyle = viewportStyle.BorderForeground(activePanelColor)
		step.viewport.Styles.LineNumber = lineNumberActiveStyle
	}

	list := listStyle.Height(m.list.Height()).Width(m.list.Width()).Render(lipgloss.JoinVertical(lipgloss.Top, listPanel...))
	tags := step.TagsAsString()
	var pagerPanelContent []string
	var pagerPanel []string

	pagerPanelContent = append(pagerPanelContent, step.viewport.View())

	if tags != "" {
		pagerPanelContent = append([]string{lipgloss.NewStyle().Width(step.viewport.Width - lipgloss.Width(list)).Render(tags)}, pagerPanelContent...)
	}

	step.viewport.Width = m.width
	step.viewport.Height = m.height - 3

	if m.scanState > 0 {
		pagerPanelContent = append(pagerPanelContent, m.scanInput.View())
		step.viewport.Height = step.viewport.Height + -1
	}

	if step.TagsAsString() != "" {
		step.viewport.Height = step.viewport.Height + -1
	}

	pagerPanel = []string{
		lipgloss.NewStyle().Width(step.viewport.Width - lipgloss.Width(list)).Render(
			fmt.Sprintf("%s %s %s %s",
				topStyle.Render("┌─ ·"),
				topTitleStyle.Render(step.GetName()),
				topStyle.Render("· "),
				topStyle.Render(strings.Repeat("─", m.width-lipgloss.Width(list)-lipgloss.Width(step.GetName())-9))),
		),
		viewportStyle.Render(lipgloss.JoinVertical(lipgloss.Top, pagerPanelContent...)),
	}

	return lipgloss.JoinVertical(
		lipgloss.Top,
		lipgloss.JoinHorizontal(
			lipgloss.Top,
			list,
			lipgloss.JoinVertical(
				lipgloss.Top,
				pagerPanel...,
			),
		),

		m.bottomPanel(),
	)
}

func (m UI) bottomPanel() string {
	status := m.renderStatus()
	scrollPercentage := scrollPercentageStyle.Render(fmt.Sprintf("%3.f%%", m.lastSelected.(StepMsg).viewport.ScrollPercent()*100))
	return lipgloss.JoinHorizontal(lipgloss.Bottom,
		status,
		lipgloss.NewStyle().Width(m.width-lipgloss.Width(status)-lipgloss.Width(scrollPercentage)).Render(lipgloss.NewStyle().Render(m.list.Help.View(m.list))),
		scrollPercentage,
	)
}
