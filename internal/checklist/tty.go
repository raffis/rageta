package checklist

import (
	"fmt"
	"io"
	"sync"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/raffis/rageta/internal/tui"
)

// activePanelColor and okStyle mirror internal/tui/styles.go so the checklist
// spinner and status symbols look the same as the pipeline TUI's.
var (
	activePanelColor = lipgloss.Color("#7D56F4")
	okStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("#008000"))
)

type stepStatus int

const (
	stepRunning stepStatus = iota
	stepOK
)

type checklistStep struct {
	label    string
	status   stepStatus
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

// stepDoneMsg marks the current step as completed successfully. A failing
// step never reaches the model: the display shuts down instead, so the caller
// owns the terminal for its error report.
type stepDoneMsg struct{}

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
		m.step.status = stepOK
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
	default:
		return tea.NewView(fmt.Sprintf("%s %s %s", m.spinner.View(), m.step.label, duration))
	}
}

type ttyDisplay struct {
	out       io.Writer
	program   *tea.Program
	done      chan struct{}
	closeErr  error
	closeOnce sync.Once

	mu     sync.Mutex
	closed bool
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
	if !d.send(addStepMsg{label: label}) {
		return fn()
	}

	err := fn()

	// The caller reports the error itself, and it writes to the very terminal
	// this program is painting. Tear the program down before handing the error
	// back so the report cannot interleave with a spinner frame, and so the
	// line this display drew is erased instead of being left behind.
	if err != nil {
		_ = d.Close()
		return err
	}

	d.send(stepDoneMsg{})
	return nil
}

// send delivers msg and reports whether this display is still rendering.
// Holding the lock across the send keeps Close from quitting the program
// between the check and the delivery.
func (d *ttyDisplay) send(msg tea.Msg) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return false
	}

	d.program.Send(msg)
	return true
}

func (d *ttyDisplay) Close() error {
	d.mu.Lock()
	d.closed = true
	d.mu.Unlock()

	d.closeOnce.Do(func() {
		// An empty view as the last state makes bubbletea's final render erase
		// every line it drew, leaving the terminal as it found it.
		d.program.Send(clearMsg{})
		d.program.Quit()
		<-d.done
	})

	return d.closeErr
}
