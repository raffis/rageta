package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
	"github.com/raffis/rageta/internal/tui/pager"
)

func NewTask(name string) *Task {
	viewport := pager.New(0, 0)
	viewport.Style = windowStyle
	viewport.ShowLineNumbers = true
	viewport.AutoScroll = true
	viewport.Styles.LineNumber = lineNumberPrefixStyle
	//viewport.SetContent("Loading...")

	task := &Task{
		viewport: &viewport,
		name:     name,
		status:   StepStatusWaiting,
	}

	return task
}

type Task struct {
	viewport *pager.Model
	name     string
	status   StepStatus
	ready    bool
	started  time.Time
	finished time.Time
}

func (t *Task) getViewport() *pager.Model {
	return t.viewport
}

func (t *Task) Write(b []byte) (int, error) {
	return t.viewport.Write(b)
}

func (t *Task) Update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	return tea.Batch(cmds...)
}

func (t *Task) Init() tea.Cmd {
	return nil
}

func (t *Task) GetName() string {
	return t.name
}

func (t *Task) SetStatus(status StepStatus) {
	if t.started.IsZero() && status == StepStatusRunning {
		t.started = time.Now()
	}

	if t.finished.IsZero() && (status > 1) {
		t.finished = time.Now()
	}

	t.status = status
}

func (t *Task) Title() string {
	return zone.Mark(t.name, fmt.Sprintf("%s %s", t.status.Render(), t.name))
}

func (t *Task) Description() string {
	if t.started.IsZero() {
		return zone.Mark(t.name, "<not started>")
	} else if t.finished.IsZero() {
		return zone.Mark(t.name, fmt.Sprintf("[%s]", time.Since(t.started).Round(time.Millisecond*10)))
	} else {
		return zone.Mark(t.name, fmt.Sprintf("[%s]", t.finished.Sub(t.started).Round(time.Millisecond*10)))
	}

}

func (t *Task) FilterValue() string {
	return t.name
}

type StepStatus int

const (
	StepStatusWaiting StepStatus = iota
	StepStatusRunning
	StepStatusFailed
	StepStatusDone
	StepStatusSkipped
)

var eventTypes = []string{"waiting", "running", "failed", "done", "skipped"}

func (e StepStatus) String() string {
	return eventTypes[e]
}

func (e StepStatus) Render() string {
	switch e {
	case StepStatusRunning:
		return taskRunningStyle.Render("◴")
	case StepStatusDone:
		return taskOkStyle.Render("✔")
	case StepStatusFailed:
		return taskFailedStyle.Render("✗")
	case StepStatusWaiting:
		return taskWaitingStyle.Render("◎")
	case StepStatusSkipped:
		return taskWarningStyle.Render("⚠")
	}

	return ""
}
