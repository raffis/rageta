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

// Display constants for step formatting
const (
	MinListWidth        = 50
	ShortListThreshold  = 50
	StatusColumnWidth   = 4
	EllipsisLength      = 3
	NotStartedDuration  = "<not started>"
)

// Column width percentages for wide layouts
const (
	NameColumnPercent     = 65
	TagsColumnPercent     = 15
	DurationColumnPercent = 20
)

// NewStep creates a new StepMsg with initialized components
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

// StepMsg represents a pipeline step with its state and UI components
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

// Write implements io.Writer interface for the step's viewport
func (t StepMsg) Write(b []byte) (int, error) {
	return t.viewport.Write(b)
}

// GetName returns the display name of the step
func (t StepMsg) GetName() string {
	return t.DisplayName
}

// WithStatus creates a new StepMsg with the given status, updating timestamps
func (t StepMsg) WithStatus(status StepStatus) StepMsg {
	if t.started.IsZero() && status == StepStatusRunning {
		t.started = time.Now()
	}

	if t.finished.IsZero() && status > StepStatusRunning {
		t.finished = time.Now()
	}

	t.Status = status
	return t
}

// TagsAsString returns a formatted string representation of all tags
func (t *StepMsg) TagsAsString() string {
	if len(t.Tags) == 0 {
		return ""
	}

	var tags []string
	for _, tag := range t.Tags {
		tagLabel := styles.TagLabel.
			Background(lipgloss.Color(tag.Color)).
			Render(fmt.Sprintf("%s: %s", tag.Key, tag.Value))
		tags = append(tags, tagLabel)
	}

	return strings.Join(tags, "")
}

// shortTags returns a compact representation of tags using colored dots
func (t *StepMsg) shortTags() string {
	if len(t.Tags) == 0 {
		return ""
	}

	var tags []string
	for _, tag := range t.Tags {
		dot := listTagLabelStyle.
			Foreground(lipgloss.Color(tag.Color)).
			Render("●")
		tags = append(tags, dot)
	}

	return strings.Join(tags, "")
}

// Title returns the formatted title for list display
func (t StepMsg) Title() string {
	listWidth := t.listWidth - StatusColumnWidth - 2 // Account for status and padding

	var status string
	if t.Status == StepStatusRunning {
		status = t.loader.View()
	} else {
		status = t.Status.Render()
	}

	switch {
	case listWidth >= MinListWidth:
		return t.formatWideTitle(status, listWidth)
	default:
		return t.formatNarrowTitle(status, listWidth)
	}
}

// formatWideTitle formats the title for wide display with multiple columns
func (t StepMsg) formatWideTitle(status string, width int) string {
	nameWidth := int(float64(width) * NameColumnPercent / 100)
	tagsWidth := int(float64(width) * TagsColumnPercent / 100)
	durationWidth := int(float64(width) * DurationColumnPercent / 100)

	return fmt.Sprintf("%s %s %s %s",
		status,
		listColumnStyle.Width(nameWidth).Render(ellipsis(t.DisplayName, nameWidth)),
		listColumnStyle.Width(tagsWidth).Render(t.shortTags()),
		durationStyle.Width(durationWidth).Align(lipgloss.Right).Render(t.duration()),
	)
}

// formatNarrowTitle formats the title for narrow display
func (t StepMsg) formatNarrowTitle(status string, width int) string {
	nameWidth := width - len(t.shortTags()) - 1
	return fmt.Sprintf("%s %s %s", 
		status, 
		ellipsis(t.DisplayName, nameWidth), 
		t.shortTags())
}

// duration returns a formatted duration string for the step
func (t *StepMsg) duration() string {
	if t.started.IsZero() {
		return NotStartedDuration
	} 
	
	if t.finished.IsZero() {
		return time.Since(t.started).Round(time.Millisecond * 10).String()
	}
	
	return t.finished.Sub(t.started).Round(time.Millisecond * 10).String()
}

// Description returns the description for list display (used in narrow layouts)
func (t StepMsg) Description() string {
	if t.listWidth-5 < ShortListThreshold {
		return t.duration()
	}
	return ""
}

// ellipsis truncates a string to maxLen characters, adding "..." if needed
func ellipsis(s string, maxLen int) string {
	if maxLen < EllipsisLength {
		maxLen = EllipsisLength
	}

	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}

	return string(runes[0:maxLen-EllipsisLength]) + "..."
}

// FilterValue returns the value used for filtering
func (t StepMsg) FilterValue() string {
	values := []string{
		t.DisplayName,
		t.Name,
	}
	
	for _, tag := range t.Tags {
		values = append(values, tag.Key)
		values = append(values, tag.Value)
	}
	
	return strings.Join(values, " ")
}

// StepStatus represents the current state of a pipeline step
type StepStatus int

const (
	StepStatusWaiting StepStatus = iota
	StepStatusRunning
	StepStatusFailed
	StepStatusDone
	StepStatusSkipped
)

// Step status string representations
var stepStatusStrings = []string{
	"waiting", 
	"running", 
	"failed", 
	"done", 
	"skipped",
}

// String returns the string representation of the step status
func (e StepStatus) String() string {
	if int(e) >= len(stepStatusStrings) {
		return "unknown"
	}
	return stepStatusStrings[e]
}

// Render returns the styled visual representation of the step status
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
	default:
		return stepWaitingStyle.Render("?")
	}
}
