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
)

type UI interface {
	tea.Model
	AddTasks(tasks ...*Task)
	GetTask(name string) (*Task, error)
	SetStatus(status StepStatus)
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
		if task.(*Task).GetName() == name {
			return task.(*Task), nil
		}
	}

	return nil, fmt.Errorf("no such task: %s", name)
}

func NewModel() *model {
	itemStyle := list.NewDefaultDelegate()

	m := &model{
		status: StepStatusWaiting,
		list:   list.New(nil, itemStyle, 0, 0),
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

func (m *model) SetStatus(status StepStatus) {
	m.status = status

	if status == StepStatusFailed {
		for _, task := range m.list.Items() {
			if task.(*Task).status == StepStatusRunning {
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
	var cmds []tea.Cmd

	for _, task := range m.list.Items() {
		cmds = append(cmds, task.(*Task).Init())
	}

	return tea.Batch(cmds...)
}

func (m *model) SelectedTask() *Task {
	if len(m.list.Items()) == 0 {
		return nil
	}

	return m.list.Items()[m.list.Index()].(*Task)
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
				item, _ := listItem.(*Task)
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

	case tickMsg:

	case tea.WindowSizeMsg:
		h, _ := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-1)

		for _, task := range m.list.Items() {
			task := task.(*Task)
			if !task.ready {
				task.viewport.Width = msg.Width
				task.viewport.Height = msg.Height - 1
				task.ready = true
			} else {
				task.viewport.Width = msg.Width
				task.viewport.Height = msg.Height - 1
			}
		}

		m.sizeMsg = &msg
	default:
		for _, task := range m.list.Items() {
			cmds = append(cmds, task.(*Task).Update(msg))
		}
	}

	task := m.SelectedTask()
	if task != nil {
		viewport, cmd := task.viewport.Update(msg)
		task.viewport = &viewport
		cmds = append(cmds, cmd)

		m.scanInput, cmd = m.scanInput.Update(msg)
	}

	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *model) View() string {
	task := m.SelectedTask()

	if task == nil || !task.ready {
		return "\n  Initializing..."
	}

	list := listStyle.Render(m.list.View())
	return zone.Scan(
		lipgloss.JoinHorizontal(1, lipgloss.JoinVertical(
			lipgloss.Top,
			zone.Mark("tasks", list),
			m.footerLeftView(lipgloss.Width(list))),
			lipgloss.JoinVertical(
				lipgloss.Top,
				zone.Mark("pager", task.viewport.View()),
				m.queryView(),
			)),
	)
}

func (m *model) footerLeftView(width int) string {
	firstColumn := m.renderStatus()
	secondColumn := leftFooterPaddingStyle.Width(width - lipgloss.Width(firstColumn)).Render()

	return lipgloss.JoinHorizontal(lipgloss.Bottom,
		firstColumn,
		secondColumn,
	)
}

func (m *model) queryView() string {
	task := m.SelectedTask()
	secondColumn := scrollPercentageStyle.Width(8).Render(fmt.Sprintf("%3.f%%", task.viewport.ScrollPercent()*100))
	firstColumn := leftFooterPaddingStyle.Width(task.viewport.Width - lipgloss.Width(secondColumn) - 25).Render()

	return lipgloss.JoinHorizontal(lipgloss.Bottom,
		firstColumn,
		secondColumn,
	)
}
