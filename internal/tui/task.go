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

// Column width percentages for wide layouts. Name and labels flex with the
// available width; the stat columns get fixed minimum widths (see
// computeColumnLayout) so they can be dropped one at a time, right to left,
// when the terminal is too narrow to fit them all.
const (
	NameColumnPercent     = 35
	LabelsColumnPercent   = 10
	DurationColumnPercent = 10
)

// Minimum widths for the resource-stat columns. computeColumnLayout hides
// columns right to left (DiskWWidth first) when the list isn't wide enough
// to show all of them at their minimum width.
const (
	MinCPUWidth      = 5
	MinMemWidth      = 6
	MinNetWidth      = 9
	MinDiskWidth     = 9
	MinDurationWidth = 8
)

// columnLayout describes which columns fit in a list of the given width and
// how wide each one is. Duration is always shown; the stat columns
// (cpu, mem, net rx/tx, disk r/w) are dropped right to left as space runs
// out. Shared between the list header (ui.go) and each row (TaskMsg.Title)
// so they always agree on layout.
type columnLayout struct {
	nameWidth     int
	labelsWidth   int
	cpuWidth      int
	memWidth      int
	netRxWidth    int
	netTxWidth    int
	diskRWidth    int
	diskWWidth    int
	durationWidth int

	showCPU   bool
	showMem   bool
	showNetRx bool
	showNetTx bool
	showDiskR bool
	showDiskW bool
}

// computeColumnLayout derives the visible columns and their widths for a
// list of the given width. listWidth is expected to already account for the
// status column and outer padding (see StatusColumnWidth usages at the call
// sites).
func computeColumnLayout(listWidth int) columnLayout {
	avail := max(listWidth, 0)

	nameWidth := int(float64(avail) * NameColumnPercent / 100)
	labelsWidth := int(float64(avail) * LabelsColumnPercent / 100)
	durationWidth := max(int(float64(avail)*DurationColumnPercent/100), MinDurationWidth)

	statMins := [...]int{MinCPUWidth, MinMemWidth, MinNetWidth, MinNetWidth, MinDiskWidth, MinDiskWidth}

	remaining := avail - nameWidth - labelsWidth - durationWidth

	visible := len(statMins)
	for visible > 0 {
		needed := 0
		for i := 0; i < visible; i++ {
			needed += statMins[i] + 1 // +1 for the separating space
		}
		if needed <= remaining {
			break
		}
		visible--
	}

	return columnLayout{
		nameWidth:     nameWidth,
		labelsWidth:   labelsWidth,
		cpuWidth:      MinCPUWidth,
		memWidth:      MinMemWidth,
		netRxWidth:    MinNetWidth,
		netTxWidth:    MinNetWidth,
		diskRWidth:    MinDiskWidth,
		diskWWidth:    MinDiskWidth,
		durationWidth: durationWidth,
		showCPU:       visible >= 1,
		showMem:       visible >= 2,
		showNetRx:     visible >= 3,
		showNetTx:     visible >= 4,
		showDiskR:     visible >= 5,
		showDiskW:     visible >= 6,
	}
}

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
	Ancestors         []string
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
	layout := computeColumnLayout(listWidth)

	var status string
	if t.Status == TaskStatusRunning {
		status = t.loader.View()
	} else {
		status = t.Status.Render()
	}

	prefixWidth := lipgloss.Width(t.treePrefix)
	name := t.treePrefix + ellipsis(t.DisplayName, max(layout.nameWidth-prefixWidth, EllipsisLength))

	cols := []string{
		listColumnStyle.Width(layout.nameWidth).Render(name),
		listColumnStyle.Width(layout.labelsWidth).Render(t.shortLabels()),
	}
	if layout.showCPU {
		cols = append(cols, listColumnStyle.Width(layout.cpuWidth).Align(lipgloss.Right).Render(t.cpuString()))
	}
	if layout.showMem {
		cols = append(cols, listColumnStyle.Width(layout.memWidth).Align(lipgloss.Right).Render(t.memString()))
	}
	if layout.showNetRx {
		cols = append(cols, listColumnStyle.Width(layout.netRxWidth).Align(lipgloss.Right).Render(t.netRxString()))
	}
	if layout.showNetTx {
		cols = append(cols, listColumnStyle.Width(layout.netTxWidth).Align(lipgloss.Right).Render(t.netTxString()))
	}
	if layout.showDiskR {
		cols = append(cols, listColumnStyle.Width(layout.diskRWidth).Align(lipgloss.Right).Render(t.diskRString()))
	}
	if layout.showDiskW {
		cols = append(cols, listColumnStyle.Width(layout.diskWWidth).Align(lipgloss.Right).Render(t.diskWString()))
	}
	cols = append(cols, durationStyle.Width(layout.durationWidth).Align(lipgloss.Right).
		Render(t.duration().Round(10*time.Millisecond).String()))

	return status + " " + strings.Join(cols, " ")
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

func (t *TaskMsg) netRxString() string {
	if t.Stats == nil || t.Stats.NetRxBytes == 0 {
		return "—"
	}
	return utils.FormatBps(t.Stats.NetRxBytes)
}

func (t *TaskMsg) netTxString() string {
	if t.Stats == nil || t.Stats.NetTxBytes == 0 {
		return "—"
	}
	return utils.FormatBps(t.Stats.NetTxBytes)
}

func (t *TaskMsg) diskRString() string {
	if t.Stats == nil || t.Stats.DiskReadBytes == 0 {
		return "—"
	}
	return utils.FormatBps(t.Stats.DiskReadBytes)
}

func (t *TaskMsg) diskWString() string {
	if t.Stats == nil || t.Stats.DiskWriteBytes == 0 {
		return "—"
	}
	return utils.FormatBps(t.Stats.DiskWriteBytes)
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
