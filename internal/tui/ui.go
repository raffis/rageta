package tui

import (
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/raffis/rageta/internal/tui/pager"
)

type UI interface {
	tea.Model
	AddTasks(tasks ...*Task)
	GetTask(name string) (*Task, error)
	SetStatus(status StepStatus)
	Report(b []byte)
}

type model struct {
	list      list.Model
	status    StepStatus
	scanInput textinput.Model
	scanState bool
	sizeMsg   *tea.WindowSizeMsg
}

func (m *model) AddTasks(tasks ...*Task) {
	for _, task := range tasks {
		if m.sizeMsg != nil {
			task.viewport.Width = m.sizeMsg.Width
			task.viewport.Height = m.sizeMsg.Height - 1
			task.ready = true
		}

		m.list.InsertItem(len(m.list.Items()), task)
	}
}

func (m *model) GetTask(name string) (*Task, error) {
	for _, task := range m.list.Items() {
		if v, ok := task.(*Task); ok && v.GetName() == name {
			return task.(*Task), nil
		}
	}

	return nil, fmt.Errorf("no such task: %s", name)
}

func NewModel() *model {
	delegateStyles := list.NewDefaultItemStyles()
	delegateStyles.SelectedTitle.Border(lipgloss.BlockBorder(), false, false, false, true).BorderForeground(lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#874BFD"})
	delegateStyles.SelectedDesc.Border(lipgloss.BlockBorder(), false, false, false, true).BorderForeground(lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#874BFD"}).Foreground(lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#874BFD"})

	delegate := list.NewDefaultDelegate()
	delegate.Styles = delegateStyles

	m := &model{
		status: StepStatusWaiting,
		list:   list.New(nil, delegate, 0, 0),
	}

	m.list.SetShowTitle(false)
	m.list.SetShowStatusBar(false)
	m.list.SetShowHelp(false)
	m.list.Styles.PaginationStyle = listPaginatorStyle

	m.list.SetShowFilter(false)
	m.list.SetFilteringEnabled(false)

	scanInput := textinput.New()
	scanInput.Prompt = "Filter: "
	//filterInput.PromptStyle = styles.FilterPrompt
	//filterInput.Cursor.Style = styles.FilterCursor
	scanInput.CharLimit = 64
	scanInput.Focus()
	m.scanInput = scanInput

	m.list.KeyMap.CursorUp = key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("↑/k", "up"),
	)

	m.list.KeyMap.CursorDown = key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("↓/j", "down"),
	)

	return m
}

func Run(model tea.Model) {
	zone.NewGlobal()

	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	go func() {
		for c := range time.Tick(300 * time.Millisecond) {
			p.Send(tickMsg(c))
		}
	}()

	if _, err := p.Run(); err != nil {
		fmt.Println("could not run program:", err)
		os.Exit(1)
	}
}

type navItem interface {
	getViewport() *pager.Model
	GetName() string
}

func (m *model) Report(b []byte) {
	viewport := pager.New(0, 0)
	viewport.Style = windowStyle

	if m.sizeMsg != nil {
		viewport.Width = m.sizeMsg.Width
		viewport.Height = m.sizeMsg.Height - 1
	}

	report := &report{
		viewport: &viewport,
	}

	report.Write(b)

	m.list.InsertItem(len(m.list.Items()), report)
}

func (m *model) SetStatus(status StepStatus) {
	m.status = status

	if status == StepStatusFailed {
		for _, task := range m.list.Items() {
			if v, ok := task.(*Task); ok && v.status == StepStatusRunning {
				task.(*Task).SetStatus(StepStatusFailed)
			}
		}
	}
}

func (m *model) renderStatus() string {
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

type tickMsg time.Time

func (m *model) Init() tea.Cmd {
	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.MouseMsg:
		if !zone.Get("tasks").InBounds(msg) {
			break
		}

		if msg.Type == tea.MouseWheelUp {
			m.list.CursorUp()
			return m, nil
		}

		if msg.Type == tea.MouseWheelDown {
			m.list.CursorDown()
			return m, nil
		}

		if msg.Type == tea.MouseLeft {
			for i, listItem := range m.list.VisibleItems() {
				item, _ := listItem.(navItem)
				// Check each item to see if it's in bounds.
				if zone.Get(item.GetName()).InBounds(msg) {
					// If so, select it in the list.
					m.list.Select(i)
					break
				}
			}
		}

	case tea.KeyMsg:
		m.list, cmd = m.list.Update(msg)
		switch keypress := msg.String(); keypress {
		case "/":
			m.scanState = true
			m.scanInput.CursorEnd()
			m.scanInput.Focus()
			return m, textinput.Blink

		case "ctrl+c", "q":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		h, _ := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-1)

		for _, task := range m.list.Items() {
			if task != nil {
				task.(navItem).getViewport().Width = msg.Width
				task.(navItem).getViewport().Height = msg.Height - 1
			}

		}

		m.sizeMsg = &msg
	}

	task := m.list.SelectedItem()
	if task != nil {
		if task, ok := task.(*Task); ok {
			viewport, cmd := task.viewport.Update(msg)
			task.viewport = &viewport
			cmds = append(cmds, cmd)
		}
		if report, ok := task.(*report); ok {
			viewport, cmd := report.viewport.Update(msg)
			report.viewport = &viewport
			cmds = append(cmds, cmd)
		}

		m.scanInput, cmd = m.scanInput.Update(msg)
	}

	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *model) View() string {
	selectedItem := m.list.SelectedItem()

	if selectedItem == nil /*|| !task.ready*/ {
		return "\n  Initializing..."
	}

	task := selectedItem.(navItem)

	list := listStyle.Render(m.list.View())
	return zone.Scan(
		lipgloss.JoinHorizontal(
			lipgloss.Bottom,
			lipgloss.JoinVertical(
				lipgloss.Top,
				zone.Mark("tasks", list),
				m.footerLeftView(lipgloss.Width(list)),
			),
			lipgloss.JoinVertical(
				lipgloss.Top,
				zone.Mark("pager", task.getViewport().View()),
				m.queryView(lipgloss.Width(list)),
			),
		),
	)
}

func (m *model) footerLeftView(width int) string {
	firstColumn := m.renderStatus()
	secondColumn := leftFooterPaddingStyle.Copy().Width(width - lipgloss.Width(firstColumn)).Render()

	return lipgloss.JoinHorizontal(lipgloss.Bottom,
		firstColumn,
		secondColumn,
	)
}

func (m *model) queryView(width int) string {
	task := m.list.SelectedItem().(navItem)
	secondColumn := scrollPercentageStyle.Width(8).Render(fmt.Sprintf("%3.f%%", task.getViewport().ScrollPercent()*100))
	firstColumn := leftFooterPaddingStyle.Width(task.getViewport().Width - width - 8).Render()

	return lipgloss.JoinHorizontal(lipgloss.Bottom,
		firstColumn,
		secondColumn,
	)
}
