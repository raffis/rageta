package checklist

import (
	"fmt"
	"io"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/raffis/rageta/internal/tui"
)

// activePanelColor, okStyle and failedStyle mirror internal/tui/styles.go so
// the checklist spinner and status symbols look the same as the pipeline
// TUI's.
var (
	activePanelColor = lipgloss.Color("#7D56F4")
	okStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("#008000"))
	failedStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#D22B2B"))
)

type stepStatus int

const (
	stepRunning stepStatus = iota
	stepOK
	stepFailed
)

type checklistStep struct {
	label    string
	status   stepStatus
	err      error
	started  time.Time
	finished time.Time
}

func (s checklistStep) duration() time.Duration {
	if s.started.IsZero() {
		return 0
	}

	end := s.finished
	if end.IsZero() {
		end = time.Now()
	}

	return end.Sub(s.started)
}

// addStepMsg registers a new step as running, replacing whatever step was
// previously shown.
type addStepMsg struct {
	label string
}

// stepDoneMsg finalizes the current step.
type stepDoneMsg struct {
	err error
}

// clearMsg resets the model to an empty view, so the renderer erases every
// line it previously drew.
type clearMsg struct{}

// ttyModel renders a single step at a time: starting a new step replaces
// the previous one's line rather than appending below it.
type ttyModel struct {
	spinner spinner.Model
	step    checklistStep
}

func newTTYModel() ttyModel {
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(activePanelColor)
	return ttyModel{spinner: s}
}

func (m ttyModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m ttyModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case addStepMsg:
		m.step = checklistStep{label: msg.label, status: stepRunning, started: time.Now()}
		return m, nil
	case stepDoneMsg:
		if msg.err != nil {
			m.step.status = stepFailed
			m.step.err = msg.err
		} else {
			m.step.status = stepOK
		}
		m.step.finished = time.Now()
		return m, nil
	case clearMsg:
		m.step = checklistStep{}
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m ttyModel) View() tea.View {
	if m.step.label == "" {
		return tea.NewView("")
	}

	duration := fmt.Sprintf("%.1fs", m.step.duration().Seconds())

	switch m.step.status {
	case stepOK:
		return tea.NewView(fmt.Sprintf("%s %s", okStyle.Render("✔ "+m.step.label), duration))
	case stepFailed:
		return tea.NewView(fmt.Sprintf("%s %s", failedStyle.Render(fmt.Sprintf("✗ %s: %s", m.step.label, m.step.err)), duration))
	default:
		return tea.NewView(fmt.Sprintf("%s %s %s", m.spinner.View(), m.step.label, duration))
	}
}

type ttyDisplay struct {
	out     io.Writer
	program *tea.Program
	done    chan struct{}
	lastErr error
}

func newTTY(out io.Writer) *ttyDisplay {
	d := &ttyDisplay{
		out:  out,
		done: make(chan struct{}),
	}

	d.program = tea.NewProgram(newTTYModel(),
		tea.WithOutput(out),
		tea.WithInput(nil),
		tea.WithoutSignalHandler(),
		tea.WithEnvironment(tui.BubbleTeaProgramEnv()),
	)

	go func() {
		_, _ = d.program.Run()
		close(d.done)
	}()

	return d
}

func (d *ttyDisplay) Step(label string, fn func() error) error {
	d.program.Send(addStepMsg{label: label})
	err := fn()
	d.lastErr = err
	d.program.Send(stepDoneMsg{err: err})
	return err
}

func (d *ttyDisplay) Close() error {
	if d.lastErr == nil {
		d.program.Send(clearMsg{})
	}
	d.program.Quit()
	<-d.done

	return nil
}
