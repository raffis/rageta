package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"

	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/stats"
	"github.com/raffis/rageta/internal/styles"
	"github.com/raffis/rageta/internal/tui/pager"
	"github.com/raffis/rageta/internal/utils"
	"github.com/raffis/rageta/internal/xio"
)

// Display constants for step formatting
const (
	ShortListThreshold = 50
	StatusColumnWidth  = 4
	EllipsisLength     = 3
	NotStartedDuration = "<not started>"
)

// Column width percentages for wide layouts
const (
	NameColumnPercent     = 35
	LabelsColumnPercent   = 10
	CPUColumnPercent      = 5
	MemColumnPercent      = 5
	NetColumnPercent      = 15
	DiskColumnPercent     = 15
	DurationColumnPercent = 10
)

// ResourceStatsMsg is sent by the stats filter writer to update a task's metrics.
type ResourceStatsMsg struct {
	Name  string
	Stats *stats.Sample
}

// PullProgress holds the aggregated byte progress of an image pull.
type PullProgress struct {
	Current int64
	Total   int64
}

// PullProgressMsg is sent by the build display to update a task's image pull progress.
type PullProgressMsg struct {
	Name    string
	Current int64
	Total   int64
}

// TaskMsg represents a pipeline step with its state and UI components
type TaskMsg struct {
	w                 *xio.LineWriter
	viewport          *pager.Model
	loader            spinner.Model
	pullImageProgress progress.Model
	Name              string
	DisplayName       string
	Labels            []processor.Label
	Status            TaskStatus
	Stats             *stats.Sample
	Pull              PullProgress
	Context           processor.TaskContext
	Parents           []string
	ready             bool
	started           time.Time
	finished          time.Time
	listWidth         int
	listHeight        int
	shellHintShown    bool
	// treePrefix is the tree branch/indentation string rendered before the
	// task name, computed by buildTreeGuides in refreshList and cached here
	// so per-task patch updates (stats, pull progress, ticks) don't need to
	// recompute it.
	treePrefix string
}

// NewTask creates a new TaskMsg with initialized components
func NewTask() TaskMsg {
	viewport := pager.New(0, 0)
	viewport.ShowLineNumbers = true
	viewport.AutoScroll = true
	viewport.Styles.LineNumber = lineNumberInactiveStyle

	loader := spinner.New()
	loader.Spinner = spinner.MiniDot
	loader.Style = lipgloss.NewStyle().Foreground(activePanelColor)

	var (
		blockFull  rune = '▔'
		blockEmpty rune = ' '
	)
	bar := progress.New(
		progress.WithDefaultBlend(),
		progress.WithoutPercentage(),
		progress.WithFillCharacters(blockFull, blockEmpty),
	)

	return TaskMsg{
		w:                 xio.NewLineWriter(&viewport),
		viewport:          &viewport,
		loader:            loader,
		pullImageProgress: bar,
	}
}

func (t TaskMsg) Flush() error {
	return t.w.Flush()
}

// Write implements io.Writer interface for the step's viewport
func (t TaskMsg) Write(b []byte) (int, error) {
	return t.w.Write(b)
}

// GetName returns the display name of the step
func (t TaskMsg) GetName() string {
	return t.DisplayName
}

// WithStatus creates a new TaskMsg with the given status, updating timestamps.
// Nothing in the pipeline ever reports TaskStatusRunning explicitly — a task
// goes straight from TaskStatusWaiting (set when it's registered, which
// coincides with it starting to execute) to a terminal status — so
// "started" is stamped unconditionally on first transition rather than
// gated on status == Running.
func (t TaskMsg) WithStatus(status TaskStatus) TaskMsg {
	if t.started.IsZero() {
		t.started = time.Now()
	}

	if t.finished.IsZero() && isTaskFinished(status) {
		t.finished = time.Now()
	}

	t.Status = status
	return t
}

// duration returns how long the task has been running: elapsed so far if
// it's still in flight, or its total run time once finished. Computed
// on-the-fly from timestamps rather than a ticking stopwatch, so rendering
// it costs nothing beyond a render call — no per-task recurring messages
// are needed to keep it up to date.
func (t TaskMsg) duration() time.Duration {
	if t.started.IsZero() {
		return 0
	}

	end := t.finished
	if end.IsZero() {
		end = time.Now()
	}

	return end.Sub(t.started)
}

// LabelsAsString returns a formatted string representation of all tags
func (t *TaskMsg) LabelsAsString() string {
	if len(t.Labels) == 0 {
		return ""
	}

	var tags []string
	for _, tag := range t.Labels {
		tagLabel := styles.Label.
			Background(lipgloss.Color(tag.HEXColor)).
			Foreground(styles.AdaptiveBrightnessColor(lipgloss.Color(tag.HEXColor))).
			PaddingLeft(1).
			PaddingRight(1).
			Render(fmt.Sprintf("%s: %s", tag.Key, tag.Value))
		tags = append(tags, tagLabel)
	}

	return strings.Join(tags, "")
}

// shortLabels returns a compact representation of tags using colored dots
func (t *TaskMsg) shortLabels() string {
	if len(t.Labels) == 0 {
		return ""
	}

	var tags []string
	for _, tag := range t.Labels {
		dot := listLabelStyle.
			Foreground(lipgloss.Color(tag.HEXColor)).
			Render("●")
		tags = append(tags, dot)
	}

	return strings.Join(tags, "")
}

// Title returns the formatted title for list display
func (t TaskMsg) Title() string {
	listWidth := t.listWidth - StatusColumnWidth - 2 // Account for status and padding

	var status string
	if t.Status == TaskStatusRunning {
		status = t.loader.View()
	} else {
		status = t.Status.Render()
	}

	nameWidth := int(float64(listWidth) * NameColumnPercent / 100)
	tagsWidth := int(float64(listWidth) * LabelsColumnPercent / 100)
	cpuWidth := int(float64(listWidth) * CPUColumnPercent / 100)
	memWidth := int(float64(listWidth) * MemColumnPercent / 100)
	netWidth := int(float64(listWidth) * NetColumnPercent / 100)
	diskWidth := int(float64(listWidth) * DiskColumnPercent / 100)
	durationWidth := int(float64(listWidth) * DurationColumnPercent / 100)

	prefixWidth := lipgloss.Width(t.treePrefix)
	name := t.treePrefix + ellipsis(t.DisplayName, max(nameWidth-prefixWidth, EllipsisLength))

	return fmt.Sprintf("%s %s %s %s %s %s %s %s",
		status,
		listColumnStyle.Width(nameWidth).Render(name),
		listColumnStyle.Width(tagsWidth).Render(t.shortLabels()),
		listColumnStyle.Width(cpuWidth).Align(lipgloss.Right).Render(t.cpuString()),
		listColumnStyle.Width(memWidth).Align(lipgloss.Right).Render(t.memString()),
		listColumnStyle.Width(netWidth).Align(lipgloss.Right).Render(t.netString()),
		listColumnStyle.Width(diskWidth).Align(lipgloss.Right).Render(t.diskString()),
		durationStyle.Width(durationWidth).Align(lipgloss.Right).Render(t.duration().Round(10*time.Millisecond).String()),
	)
}

func (t *TaskMsg) cpuString() string {
	if t.Stats == nil || t.Stats.CPUMillicores == 0 {
		return "—"
	}
	return fmt.Sprintf("%dm", t.Stats.CPUMillicores)
}

func (t *TaskMsg) memString() string {
	if t.Stats == nil || t.Stats.MemBytes == 0 {
		return "—"
	}
	return utils.FormatBytes(t.Stats.MemBytes)
}

func (t *TaskMsg) netString() string {
	if t.Stats == nil || t.Stats.NetRxBytes == 0 && t.Stats.NetTxBytes == 0 {
		return "—"
	}
	return fmt.Sprintf("⇩ %s ⇧ %s", utils.FormatBps(t.Stats.NetRxBytes), utils.FormatBps(t.Stats.NetTxBytes))
}

func (t *TaskMsg) diskString() string {
	if t.Stats == nil || t.Stats.DiskReadBytes == 0 && t.Stats.DiskWriteBytes == 0 {
		return "—"
	}
	return fmt.Sprintf("R %s W %s", utils.FormatBps(t.Stats.DiskReadBytes), utils.FormatBps(t.Stats.DiskWriteBytes))
}

// Description returns the description line rendered below the task's title,
// used to display an image pull progress bar while a build step is running.
// It is hidden once the pull completes (current >= total).
func (t TaskMsg) Description() string {
	if t.Status != TaskStatusRunning || t.Pull.Total <= 0 || t.Pull.Current >= t.Pull.Total {
		return ""
	}

	percent := float64(t.Pull.Current) / float64(t.Pull.Total)
	return t.pullImageProgress.ViewAs(percent)
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
func (t TaskMsg) FilterValue() string {
	values := []string{
		t.DisplayName,
		t.Name,
	}

	for _, tag := range t.Labels {
		values = append(values, tag.Key)
		values = append(values, tag.Value)
	}

	return strings.Join(values, " ")
}

// TaskStatus represents the current state of a pipeline step
type TaskStatus int

const (
	TaskStatusWaiting TaskStatus = iota
	TaskStatusRunning
	TaskStatusFailed
	TaskStatusDone
	TaskStatusCached
	TaskStatusSkipped
)

// Task status string representations
var stepStatusStrings = []string{
	"waiting",
	"running",
	"failed",
	"done",
	"cached",
	"skipped",
}

// String returns the string representation of the step status
func (e TaskStatus) String() string {
	if int(e) >= len(stepStatusStrings) {
		return "unknown"
	}
	return stepStatusStrings[e]
}

// Render returns the styled visual representation of the step status
func (e TaskStatus) Render() string {
	switch e {
	case TaskStatusRunning:
		return stepRunningStyle.Render("◴")
	case TaskStatusDone:
		return stepOkStyle.Render("✔")
	case TaskStatusFailed:
		return stepFailedStyle.Render("✗")
	case TaskStatusWaiting:
		return stepWaitingStyle.Render("◎")
	case TaskStatusCached:
		return stepCachedStyle.Render("◈")
	case TaskStatusSkipped:
		return stepWarningStyle.Render("⚠")
	default:
		return stepWaitingStyle.Render("?")
	}
}
