package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/styles"
	"github.com/raffis/rageta/internal/tui/pager"
)

func NewStep() StepMsg {
	viewport := pager.New(0, 0)
	viewport.ShowLineNumbers = true
	viewport.AutoScroll = true
	viewport.Styles.LineNumber = lineNumberInactiveStyle

	loader := spinner.New()
	loader.Spinner = spinner.MiniDot
	loader.Style = lipgloss.NewStyle().Foreground(activePanelColor)

	return StepMsg{
		viewport: &viewport,
		loader:   loader,
	}
}

type StepMsg struct {
	viewport    *pager.Model
	loader      spinner.Model
	Name        string
	DisplayName string
	Tags        []processor.Tag
	Status      StepStatus
	ready       bool
	started     time.Time
	finished    time.Time
	listWidth   int
	listHeight  int
}

func (t StepMsg) Write(b []byte) (int, error) {
	return t.viewport.Write(b)
}
func (t StepMsg) GetName() string {
	return t.DisplayName
}

func (t StepMsg) WithStatus(status StepStatus) StepMsg {
	if t.started.IsZero() && status == StepStatusRunning {
		t.started = time.Now()
	}

	if t.finished.IsZero() && (status > 1) {
		t.finished = time.Now()
	}

	t.Status = status
	return t
}

func (t *StepMsg) TagsAsString() string {
	var tags []string
	for _, tag := range t.Tags {
		tags = append(tags, styles.TagLabel.Background(lipgloss.Color(tag.Color)).Render(fmt.Sprintf("%s: %s", tag.Key, tag.Value)))
	}

	return strings.Join(tags, "")
}

func (t *StepMsg) shortTags() string {
	var tags []string
	for _, tag := range t.Tags {
		tags = append(tags, listTagLabelStyle.Foreground(lipgloss.Color(tag.Color)).Render("●"))
	}

	return strings.Join(tags, "")
}

func (t StepMsg) Title() string {
	listWidth := t.listWidth - 6

	var status string
	if t.Status == StepStatusRunning {
		status = t.loader.View()
	} else {
		status = t.Status.Render()
	}

	switch {
	case listWidth >= 50:
		return fmt.Sprintf("%s %s %s %s",
			status,
			listColumnStyle.Width(int(float64(listWidth)/100*65)).Render(ellipsis(t.DisplayName, int(float64(listWidth)/100*65))),
			listColumnStyle.Width(int(float64(listWidth)/100*15)).Render(t.shortTags()),
			durationStyle.Width(int(float64(listWidth)/100*20)).Align(lipgloss.Right).Render(t.duration()),
		)
	case listWidth < 50:
		return fmt.Sprintf("%s %s %s", status, ellipsis(t.DisplayName, t.listWidth-5), t.shortTags())
	}

	return ""
}

func (t *StepMsg) duration() string {
	if t.started.IsZero() {
		return "<not started>"
	} else if t.finished.IsZero() {
		return fmt.Sprintf("%s", time.Since(t.started).Round(time.Millisecond*10))
	} else {
		return fmt.Sprintf("%s", t.finished.Sub(t.started).Round(time.Millisecond*10))
	}
}

func (t StepMsg) Description() string {
	listWidth := t.listWidth - 5

	switch {
	case listWidth < 50:
		return t.duration()
	}

	return ""
}

func ellipsis(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen < 3 {
		maxLen = 3
	}
	return string(runes[0:maxLen-3]) + "..."
}

func (t StepMsg) FilterValue() string {
	return t.DisplayName
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
		return stepRunningStyle.Render("◴")
	case StepStatusDone:
		return stepOkStyle.Render("✔")
	case StepStatusFailed:
		return stepFailedStyle.Render("✗")
	case StepStatusWaiting:
		return stepWaitingStyle.Render("◎")
	case StepStatusSkipped:
		return stepWarningStyle.Render("⚠")
	}

	return ""
}
