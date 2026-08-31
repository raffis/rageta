package tui

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/go-logr/logr"
	"github.com/raffis/rageta/internal/processor"
)

type Panel int8

const (
	PanelList Panel = iota
	PanelDetails
)

const (
	ListWidthPercentage     = 35.0
	ListHeightPercentage    = 40.0
	LayoutAreaHeight        = 4
	LayoutAreaHeightNarrow  = 7
	FilterInputHeightOffset = 1
	LabelsHeightOffset      = 1
	// StatsHeightOffset reserves 2 lines, not 1: the stats bar's own
	// content line plus the closing bottom border it draws for the list
	// body's box (see renderListPanel/renderListStats).
	StatsHeightOffset = 2
	// WideStatsHeightOffset is the wide-layout equivalent of
	// StatsHeightOffset — see its use in handleWindowResize for why the
	// two layouts need different values.
	WideStatsHeightOffset     = 1
	AlignHorizontalBreakpoint = 350
)

const (
	KeyFilter     = "/"
	KeyEscape     = "esc"
	KeyTab        = "tab"
	KeyEnter      = "enter"
	KeyQuit       = "ctrl+c"
	KeyQ          = "q"
	KeyDebugShell = "i"
	KeyShowAll    = "a"
)

// uiKeyMap is the help.KeyMap rendered in the bottom help bar. It's a
// hand-picked list of bindings rather than a delegate to list.KeyMap so we
// can fully control what's shown (no "?" full-help toggle, arrow keys only).
type uiKeyMap struct{}

func (uiKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "up")),
		key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "down")),
		key.NewBinding(key.WithKeys(KeyFilter), key.WithHelp(KeyFilter, "filter")),
		key.NewBinding(key.WithKeys(KeyTab), key.WithHelp("⇅", "switch panel")),
		key.NewBinding(key.WithKeys(KeyShowAll), key.WithHelp(KeyShowAll, "show/hide all")),
		key.NewBinding(key.WithKeys(KeyDebugShell), key.WithHelp(KeyDebugShell, "interactive")),
		key.NewBinding(key.WithKeys(KeyQ), key.WithHelp(KeyQ, "quit")),
	}
}

func (k uiKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.ShortHelp()}
}

// DebugShellFactory builds the tea.ExecCommand used to run a debug shell for the given
// task context. It is provided by the run package, which owns the buildkit client and
// container runtime needed to export and start the task's state as a container.
type DebugShellFactory func(taskCtx processor.TaskContext) (tea.ExecCommand, error)

// DebugShellDoneMsg is delivered once a debug shell spawned via tea.Exec exits.
type DebugShellDoneMsg struct {
	Name string
	Err  error
}

type UI struct {
	list         list.Model
	loader       spinner.Model
	help         help.Model
	status       TaskStatus
	scanInput    textinput.Model
	width        int
	height       int
	mu           *sync.Mutex
	logger       logr.Logger
	activePanel  Panel
	lastSelected list.Item
	debugShell   DebugShellFactory
	tasks        []TaskMsg
	showAll      bool

	// taskIndex maps a task's Name to its position in m.tasks, so per-task
	// messages (resource stats, pull progress) can find their task in O(1)
	// instead of scanning m.tasks. Rebuilt in sortList, which is the only
	// place m.tasks is reordered; a plain status update on an existing task
	// doesn't move it, so the index stays valid across those.
	taskIndex map[string]int

	// visibleIndex maps a task's Name to its row index in the list's
	// current items, rebuilt in refreshList. Lets per-task updates patch
	// their row in O(1) via updateVisibleItem instead of scanning
	// m.list.Items().
	visibleIndex map[string]int
}

// SetDebugShell registers the factory used to spawn a debug shell for the selected task.
func (m *UI) SetDebugShell(fn DebugShellFactory) {
	m.debugShell = fn
}

type TickMsg time.Time

type PipelineDoneMsg struct {
	Status TaskStatus
	Error  error
}

// compactDelegate wraps list.DefaultDelegate to drop the blank description
// row bubbles' DefaultDelegate otherwise always reserves (it renders
// "title\ndesc" even when desc is empty), which shows up as a gap line
// between every task. The description row is only ever used to display the
// image-pull progress bar (see TaskMsg.Description), so it's included only
// when that's actually non-empty.
type compactDelegate struct {
	list.DefaultDelegate
}

func (d compactDelegate) Height() int {
	return 1
}

func (d compactDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	if di, ok := item.(list.DefaultItem); !ok || di.Description() == "" {
		d.ShowDescription = false
	}

	// Color the task name explicitly when this row is the selected one,
	// rather than relying on the outer SelectedTitle wrap bubbles applies:
	// the status icon segment carries its own ANSI color+reset, and that
	// reset clears the wrap's color for everything after it (ANSI resets
	// aren't scoped to the style that opened them), so the name would
	// otherwise render in the default color regardless of SelectedTitle.
	if task, ok := item.(TaskMsg); ok {
		task.selected = index == m.Index() && m.FilterState() != list.Filtering
		item = task
	}

	d.DefaultDelegate.Render(w, m, index, item)
}

func NewUI(logger logr.Logger) UI {
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		BorderForeground(activePanelColor).
		Border(lipgloss.BlockBorder(), false, false, false, true)

	delegate.ShowDescription = true
	delegate.SetSpacing(0)
	noBorderDescPadding := lipgloss.NewStyle().Padding(0, 0, 0, 2)
	delegate.Styles.SelectedDesc = noBorderDescPadding
	delegate.Styles.NormalDesc = noBorderDescPadding
	delegate.Styles.DimmedDesc = noBorderDescPadding

	ui := UI{
		status:      TaskStatusWaiting,
		list:        list.New(nil, compactDelegate{delegate}, 0, 0),
		help:        help.New(),
		mu:          &sync.Mutex{},
		activePanel: PanelList,
		logger:      logger,
		// Done/cached/skipped tasks are shown by default; 'a' toggles them
		// back to hidden for users who want a quieter, in-progress-only view.
		showAll: true,
	}

	ui.initializeList()
	ui.initializeScanInput()
	ui.initializeLoader()
	ui.initializeHelp()

	return ui
}

// initializeList sets up the list component
func (m *UI) initializeList() {
	m.list.SetShowTitle(false)
	m.list.SetShowStatusBar(false)
	m.list.SetShowHelp(false)
	m.list.SetShowFilter(false)
	m.list.SetFilteringEnabled(true)
	m.list.Styles.PaginationStyle = listPaginatorStyle
	m.list.KeyMap.CursorUp = key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "up"))
	m.list.KeyMap.CursorDown = key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "down"))
}

// initializeScanInput sets up the scan input component
func (m *UI) initializeScanInput() {
	scanInput := textinput.New()
	scanInput.Prompt = "Filter: "
	scanInput.CharLimit = 64
	scanInput.Focus()
	m.scanInput = scanInput
}

// initializeLoader sets up the loading spinner
func (m *UI) initializeLoader() {
	m.loader = spinner.New()
	m.loader.Spinner = spinner.Dot
	m.loader.Style = lipgloss.NewStyle().Foreground(activePanelColor)
}

// initializeHelp sets up the bottom help bar, brightening it beyond bubbles'
// default styles (tuned dim for a light background, hard to read on dark).
func (m *UI) initializeHelp() {
	m.help.Styles.ShortKey = helpKeyStyle
	m.help.Styles.ShortDesc = helpDescStyle
	m.help.Styles.ShortSeparator = helpSepStyle
	m.help.Styles.Ellipsis = helpSepStyle
}

// sortList reorders m.tasks into dependency-tree order (see treeOrder),
// rebuilds the by-name index (m.tasks positions just changed), and
// refreshes the visible list.
func (m *UI) sortList() {
	m.tasks = m.treeOrder()

	m.taskIndex = make(map[string]int, len(m.tasks))
	for i, t := range m.tasks {
		m.taskIndex[t.Name] = i
	}

	m.refreshList()
}

// treeOrder returns m.tasks reordered so that every task appears after all
// of its resolved dependencies (Ancestors entries that match another task
// currently in the list), with a task's dependents grouped as a contiguous
// run immediately following it (depth-first), rather than levelled
// breadth-first. Tasks with no resolved dependency are treated as roots.
// Ties among roots/siblings are broken alphabetically by label, same as the
// previous flat sort.
//
// A task depending on more than one other task is placed exactly once, the
// first time all of its dependencies have already been placed — which, by
// construction, is somewhere below all of them, without duplicating the row.
func (m *UI) treeOrder() []TaskMsg {
	byName := make(map[string]TaskMsg, len(m.tasks))
	for _, t := range m.tasks {
		byName[t.Name] = t
	}

	labelKey := func(name string) string {
		return strings.Join(m.formatLabelsForSorting(byName[name].Labels), "-")
	}
	sortByLabel := func(names []string) {
		sort.SliceStable(names, func(i, j int) bool {
			return labelKey(names[i]) < labelKey(names[j])
		})
	}

	children := make(map[string][]string, len(m.tasks))
	hasResolvedDep := make(map[string]bool, len(m.tasks))
	for _, t := range m.tasks {
		for _, dep := range t.Ancestors {
			if _, ok := byName[dep]; ok {
				children[dep] = append(children[dep], t.Name)
				hasResolvedDep[t.Name] = true
			}
		}
	}
	for parent := range children {
		sortByLabel(children[parent])
	}

	var roots []string
	for _, t := range m.tasks {
		if !hasResolvedDep[t.Name] {
			roots = append(roots, t.Name)
		}
	}
	sortByLabel(roots)

	ordered := make([]TaskMsg, 0, len(m.tasks))
	placed := make(map[string]bool, len(m.tasks))

	var place func(name string)
	place = func(name string) {
		if placed[name] {
			return
		}
		placed[name] = true
		ordered = append(ordered, byName[name])

		for _, child := range children[name] {
			if placed[child] {
				continue
			}

			ready := true
			for _, dep := range byName[child].Ancestors {
				if _, ok := byName[dep]; ok && !placed[dep] {
					ready = false
					break
				}
			}

			if ready {
				place(child)
			}
		}
	}

	for _, r := range roots {
		place(r)
	}

	// Dependency cycles (which shouldn't occur, but the list shouldn't
	// silently drop tasks if they do) leave tasks unplaced since none of
	// them ever becomes fully "ready". Force them in as extra roots.
	for _, t := range m.tasks {
		place(t.Name)
	}

	return ordered
}

// treeGuide describes how a single row's tree branch prefix should be
// rendered: how many ancestor levels deep it is, whether it's the last
// child of its assigned parent (renders "└─" instead of "├─"), and for each
// ancestor level, whether that ancestor was itself a last child (renders a
// blank column instead of a continuing "│").
type treeGuide struct {
	depth        int
	isLast       bool
	ancestorLast []bool
}

// buildTreeGuides computes tree-branch rendering info for an already
// tree-ordered slice of tasks (as produced by treeOrder, in full or
// filtered to only the currently visible rows). Because the slice is
// guaranteed to list a task after every dependency of its that's present in
// the slice, each task's "visual parent" can simply be taken as whichever
// of its resolved dependencies appears last in the slice — every other
// dependency is necessarily above that one already. Recomputing this per
// call (rather than caching it on treeOrder's output) lets connectors adapt
// to whichever rows are actually visible, e.g. when a finished ancestor is
// hidden and a running descendant becomes a visual root.
func buildTreeGuides(tasks []TaskMsg) map[string]treeGuide {
	index := make(map[string]int, len(tasks))
	for i, t := range tasks {
		index[t.Name] = i
	}

	parentOf := make(map[string]string, len(tasks))
	childrenOf := make(map[string][]string, len(tasks))
	for _, t := range tasks {
		parent := ""
		parentIdx := -1
		for _, dep := range t.Ancestors {
			if idx, ok := index[dep]; ok && idx > parentIdx {
				parentIdx = idx
				parent = dep
			}
		}

		if parent != "" {
			parentOf[t.Name] = parent
			childrenOf[parent] = append(childrenOf[parent], t.Name)
		}
	}

	isLastChild := make(map[string]bool, len(tasks))
	for _, kids := range childrenOf {
		for i, k := range kids {
			isLastChild[k] = i == len(kids)-1
		}
	}

	guides := make(map[string]treeGuide, len(tasks))
	for _, t := range tasks {
		parent, ok := parentOf[t.Name]
		if !ok {
			guides[t.Name] = treeGuide{isLast: true}
			continue
		}

		// The parent's own guide column is only added from depth 1 onward:
		// a root (depth 0) renders no prefix at all (see the g.depth == 0
		// case in renderTreePrefix), so there's no vertical guide line to
		// continue under it. Adding a column for it anyway is what used to
		// push every first-level row's branch glyph one extra "   "/"│  "
		// to the right, as if the invisible root still occupied a column.
		pg := guides[parent]
		ancestorLast := append([]bool{}, pg.ancestorLast...)
		if pg.depth > 0 {
			ancestorLast = append(ancestorLast, pg.isLast)
		}

		guides[t.Name] = treeGuide{
			depth:        pg.depth + 1,
			isLast:       isLastChild[t.Name],
			ancestorLast: ancestorLast,
		}
	}

	return guides
}

// renderTreePrefix renders a treeGuide into the branch/indentation string
// drawn before a task's name, e.g. "│  └─ ".
func renderTreePrefix(g treeGuide) string {
	if g.depth == 0 {
		return ""
	}

	var sb strings.Builder
	for _, last := range g.ancestorLast {
		if last {
			sb.WriteString("   ")
		} else {
			sb.WriteString("│  ")
		}
	}

	if g.isLast {
		sb.WriteString("└─ ")
	} else {
		sb.WriteString("├─ ")
	}

	return treeGuideStyle.Render(sb.String())
}

// isHiddenStatus reports whether a task in this status is hidden from the
// list by default (successful/skipped tasks are noise once they're done).
func isHiddenStatus(status TaskStatus) bool {
	switch status {
	case TaskStatusDone, TaskStatusCached, TaskStatusSkipped:
		return true
	default:
		return false
	}
}

func isTaskFinished(status TaskStatus) bool {
	switch status {
	case TaskStatusFailed, TaskStatusDone, TaskStatusCached, TaskStatusSkipped:
		return true
	default:
		return false
	}
}

// refreshList rebuilds the visible list from m.tasks, applying the
// show-all/hide-finished filter and preserving the current selection, and
// rebuilds m.visibleIndex to match. This rebuilds and reassigns the entire
// list contents, so it's relatively expensive with many tasks — reserve it
// for changes that affect which tasks are visible (a task added, a status
// crossing the hidden threshold, the show-all toggle, a resize). For
// updates to a single already-visible task's content (stats, pull progress,
// spinner/timer ticks), use updateVisibleItem instead.
func (m *UI) refreshList() {
	visible := make([]int, 0, len(m.tasks))
	for i, t := range m.tasks {
		if m.showAll || !isHiddenStatus(t.Status) {
			visible = append(visible, i)
		}
	}

	visibleTasks := make([]TaskMsg, len(visible))
	for i, idx := range visible {
		visibleTasks[i] = m.tasks[idx]
	}
	guides := buildTreeGuides(visibleTasks)

	items := make([]list.Item, 0, len(visible))
	visibleIndex := make(map[string]int, len(visible))
	for _, idx := range visible {
		t := m.tasks[idx]
		t.treePrefix = renderTreePrefix(guides[t.Name])
		m.tasks[idx] = t

		visibleIndex[t.Name] = len(items)
		items = append(items, t)
	}

	current := m.findCurrentSelection(items)
	m.list.SetItems(items)
	m.list.Select(current)
	m.visibleIndex = visibleIndex
}

// updateVisibleItem patches a single task's entry in the list in place,
// via m.visibleIndex, without rebuilding or reassigning the whole
// visible-items slice. It's a no-op if the task isn't currently visible
// (e.g. hidden because it's done and showAll is off). Cheap alternative to
// refreshList for the common case where a single task's content changed but
// list membership didn't.
func (m *UI) updateVisibleItem(t TaskMsg) tea.Cmd {
	if idx, ok := m.visibleIndex[t.Name]; ok {
		return m.list.SetItem(idx, t)
	}
	return nil
}

// formatLabelsForSorting formats labels for sorting purposes
func (m *UI) formatLabelsForSorting(labels []processor.Label) []string {
	var formattedLabels []string
	for _, label := range labels {
		formattedLabels = append(formattedLabels, fmt.Sprintf("%s:%s", label.Key, label.Value))
	}
	return formattedLabels
}

// findCurrentSelection finds the index of the currently selected item
func (m *UI) findCurrentSelection(items []list.Item) int {
	if m.lastSelected == nil {
		return 0
	}

	for i, item := range items {
		if item.(TaskMsg).Name == m.lastSelected.(TaskMsg).Name {
			return i
		}
	}
	return 0
}

// renderStatus renders the current pipeline status
func (m *UI) renderStatus() string {
	switch m.status {
	case TaskStatusCached:
		return pipelineCachedStyle.Render("CACHED")
	case TaskStatusDone:
		return pipelineOkStyle.Render("SUCCESS")
	case TaskStatusFailed:
		return pipelineFailedStyle.Render("FAILED")
	case TaskStatusWaiting:
		return pipelineWaitingStyle.Render("INITIALIZING")
	case TaskStatusRunning:
		return pipelineRunningStyle.Render("RUNNING")
	}
	return ""
}

// Init initializes the UI model
func (m UI) Init() tea.Cmd {
	return nil
}

// Update handles all UI updates and events
func (m UI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// The top-level loader is only ever visible on the initial "waiting"
	// screen, before any task has been selected (see View). Updating it on
	// every message regardless wastes work once the list is populated and
	// receiving frequent stats/progress messages.
	if m.lastSelected == nil {
		loader, cmd := m.loader.Update(msg)
		m.loader = loader
		cmds = append(cmds, cmd)
	}

	m.logger.V(7).Info("tui update msg", "msg", msg)

	switch msg := msg.(type) {
	case tea.MouseMsg:
		cmds = append(cmds, m.handleMouseMessage(msg))
	case tea.KeyPressMsg:
		return m.handleKeyMessage(msg)
	case PipelineDoneMsg:
		cmds = append(cmds, m.handlePipelineDone(msg)...)
	case TaskMsg:
		cmds = append(cmds, m.handleTaskMessage(msg)...)
	case TickMsg:
		cmds = append(cmds, m.handleTick(msg)...)
	case ResourceStatsMsg:
		m.handleResourceStats(msg)
	case PullProgressMsg:
		m.handlePullProgress(msg)
	case tea.WindowSizeMsg:
		cmds = append(cmds, m.handleWindowResize(msg)...)
	case DebugShellDoneMsg:
		if msg.Err != nil {
			m.logger.Error(msg.Err, "debug shell exited with an error", "task", msg.Name)
			m.writeDebugShellError(msg.Name, msg.Err)
		}
	}

	m.updateLastSelected()
	return m, tea.Batch(cmds...)
}

// handlePipelineDone handles pipeline completion
func (m *UI) handlePipelineDone(msg PipelineDoneMsg) []tea.Cmd {
	if m.status == TaskStatusWaiting {
		return []tea.Cmd{tea.Quit}
	}

	m.status = msg.Status

	if msg.Status == TaskStatusFailed {
		for i, t := range m.tasks {
			if t.Status == TaskStatusRunning {
				m.tasks[i] = t.WithStatus(TaskStatusFailed)
			}
		}
		m.refreshList()
	}

	return nil
}

// handleTaskMessage handles task status updates
func (m *UI) handleTaskMessage(msg TaskMsg) []tea.Cmd {
	m.mu.Lock()
	defer m.mu.Unlock()

	var cmds []tea.Cmd

	existing, idx, err := m.getTaskMsg(msg.Name)
	// New task
	if err != nil {
		msg.ready = true
		msg.listWidth = m.list.Width()
		msg.listHeight = m.list.Height()

		// Initialize viewport dimensions
		if msg.viewport != nil {
			m.updateViewportDimensions(&msg)
		}

		cmds = append(cmds, msg.loader.Tick)

		m.tasks = append(m.tasks, msg.WithStatus(msg.Status))
		m.sortList()
	} else {
		// Update existing task
		item := existing
		if isTaskFinished(msg.Status) {
			item.Stats = nil
			item.Pull = PullProgress{}
			item.Context = msg.Context
		}

		if !isTaskFinished(item.Status) && isTaskFinished(msg.Status) && m.debugShell != nil {
			m.writeDebugShellHint(&item)
		}

		m.tasks[idx] = item.WithStatus(msg.Status)
		m.refreshList()
	}

	if msg.Status == TaskStatusRunning {
		m.status = TaskStatusRunning
	}

	return cmds
}

// getTaskMsg retrieves a task message by name along with its index in m.tasks
func (m *UI) getTaskMsg(name string) (TaskMsg, int, error) {
	for i, task := range m.tasks {
		if task.Name == name {
			return task, i, nil
		}
	}
	return TaskMsg{}, -1, fmt.Errorf("no such task: %s", name)
}

// writeDebugShellError writes a visible error into the given task's viewport
// when spawning or running its debug shell failed. UI mode redirects the
// logger to a file (see Display.Run in the run package) so errors logged via
// m.logger alone are otherwise invisible to the user, making a failed 'i'
// press look like a silent no-op.
func (m *UI) writeDebugShellError(name string, err error) {
	m.writeTaskNotice(name, stepFailedStyle.Render(fmt.Sprintf("Debug shell failed: %s", err)))
}

// writeDebugShellInfo writes a visible, non-error notice into the given
// task's viewport, e.g. explaining why an 'i' press was a no-op, so the key
// never appears to silently do nothing.
func (m *UI) writeDebugShellInfo(name, msg string) {
	m.writeTaskNotice(name, stepWarningStyle.Render(msg))
}

// writeTaskNotice appends a rendered, already-styled line to the given
// task's viewport and refreshes its visible row.
func (m *UI) writeTaskNotice(name, rendered string) {
	idx, ok := m.taskIndex[name]
	if !ok {
		return
	}

	item := m.tasks[idx]
	fmt.Fprintf(&item, "\n%s\n", rendered)
	item.Flush()
	m.updateVisibleItem(item)
}

// writeDebugShellHint writes a hint into the task's viewport telling the user
// they can press the debug shell key now that the task has finished. Tasks
// can transition out of "running" more than once (e.g. across retries), so
// this only fires once per task to avoid spamming the viewport.
func (m *UI) writeDebugShellHint(item *TaskMsg) {
	if item.shellHintShown {
		return
	}
	item.shellHintShown = true

	fmt.Fprintf(item, "\n%s\n", durationStyle.Render(fmt.Sprintf("Press '%s' to start a shell", KeyDebugShell)))
	item.Flush()
}

// handleMouseMessage handles mouse interactions
func (m *UI) handleMouseMessage(msg tea.MouseMsg) tea.Cmd {
	var cmd tea.Cmd

	switch mmsg := msg.(type) {
	case tea.MouseWheelMsg:
		if m.activePanel == PanelList {
			switch mmsg.Button {
			case tea.MouseWheelUp:
				m.list.CursorUp()
			case tea.MouseWheelDown:
				m.list.CursorDown()
			}
		} else {
			cmd = m.updateSelectedViewport(msg)
		}
	default:
		if m.activePanel != PanelList {
			cmd = m.updateSelectedViewport(msg)
		}
	}

	return cmd
}

// handleKeyMessage handles keyboard input. Single-letter shortcuts (as
// opposed to ctrl+c, which always quits) are suppressed while the list
// filter is actively capturing keystrokes, so typing e.g. "queue" or
// "database" into the filter box doesn't quit the app or toggle show-all
// instead of inserting the letter.
func (m UI) handleKeyMessage(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	filtering := m.list.FilterState() == list.Filtering

	switch msg.String() {
	case KeyQuit:
		return m, tea.Quit
	case KeyQ:
		if !filtering {
			return m, tea.Quit
		}
	case KeyTab:
		m.toggleActivePanel()
		return m, nil
	case KeyShowAll:
		if !filtering {
			m.showAll = !m.showAll
			m.refreshList()
			return m, nil
		}
	case KeyDebugShell:
		if !filtering {
			if cmd := m.openDebugShell(); cmd != nil {
				return m, cmd
			}
		}
	}

	if m.activePanel == PanelList {
		return m.handleListPanelKeys(msg)
	}
	return m, m.updateSelectedViewport(msg)
}

// openDebugShell spawns a debug shell for the currently selected task, if the task has
// finished (so its final buildkit state is available) and a debug shell factory has been
// registered. It uses tea.Exec so bubbletea releases the terminal for the duration of the
// interactive shell and restores it to the TUI once the shell exits.
func (m *UI) openDebugShell() tea.Cmd {
	if m.debugShell == nil {
		return nil
	}

	task, ok := m.lastSelected.(TaskMsg)
	if !ok {
		return nil
	}

	if task.Status == TaskStatusRunning || task.Status == TaskStatusWaiting {
		m.writeDebugShellInfo(task.Name, "Task hasn't finished yet — wait for it to complete before starting a debug shell")
		return nil
	}

	execCmd, err := m.debugShell(task.Context)
	if err != nil {
		m.logger.Error(err, "failed to prepare debug shell", "task", task.Name)
		m.writeDebugShellError(task.Name, err)
		return nil
	}

	return tea.Exec(execCmd, func(err error) tea.Msg {
		return DebugShellDoneMsg{Name: task.Name, Err: err}
	})
}

// toggleActivePanel switches between the list and details panels
func (m *UI) toggleActivePanel() {
	if m.activePanel == PanelList {
		m.activePanel = PanelDetails
	} else {
		m.activePanel = PanelList
	}
}

// handleListPanelKeys handles keyboard input for the list panel
func (m UI) handleListPanelKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case KeyFilter:
		m.list.SetFilterState(list.Filtering)
		m.list.FilterInput.Focus()
	case KeyEscape:
		m.list.FilterInput.Reset()
		m.list.SetFilterState(list.Unfiltered)
	default:
		if m.list.FilterState() > 0 {
			m.list.FilterInput, cmd = m.list.FilterInput.Update(msg)
			filterText := m.list.FilterInput.Value()
			m.list.SetFilterText(filterText)
			m.list.SetFilterState(list.Filtering)
		} else {
			m.list, cmd = m.list.Update(msg)
		}
	}

	return m, cmd
}

// updateSelectedViewport updates the viewport for the selected item
func (m *UI) updateSelectedViewport(msg tea.Msg) tea.Cmd {
	if m.lastSelected == nil {
		return nil
	}

	name := m.lastSelected.(TaskMsg).Name
	for i, t := range m.tasks {
		if t.Name == name {
			viewport, cmd := t.viewport.Update(msg)
			t.viewport = &viewport
			m.tasks[i] = t

			m.refreshList()
			return cmd
		}
	}

	return nil
}

// handleWindowResize handles window resize events
func (m *UI) handleWindowResize(msg tea.WindowSizeMsg) []tea.Cmd {
	m.height = msg.Height
	m.width = msg.Width

	if m.width < AlignHorizontalBreakpoint {
		listHeight := float64(m.height) * ListHeightPercentage / 100
		m.list.SetSize(m.width, int(listHeight)-LayoutAreaHeightNarrow-StatsHeightOffset)
	} else {
		// Wide layout sits list and pager side by side (see
		// renderMainLayout), so unlike the narrow layout above, the list
		// body's height must make the list panel's total rendered height
		// (list header + body + stats bar) come out exactly equal to the
		// pager panel's (viewport + its border), or JoinHorizontal pads
		// the shorter one with blank lines — visible as a gap under the
		// shorter panel. That equality needs only WideStatsHeightOffset
		// (1) subtracted here, not the full StatsHeightOffset (2) used in
		// the narrow branch, because the two layouts' height formulas
		// have different structure (the narrow branch's viewport height
		// is itself derived from the list's height, which isn't true in
		// wide layout — see updateViewportDimensions).
		listWidth := float64(m.width) * ListWidthPercentage / 100
		m.list.SetSize(int(listWidth), m.height-LayoutAreaHeight-WideStatsHeightOffset)
	}

	for i, t := range m.tasks {
		t.listWidth = m.list.Width()
		t.listHeight = m.list.Height()
		m.tasks[i] = t
	}
	m.refreshList()

	return nil
}

// handlePullProgress updates the pull progress of a running task
func (m *UI) handlePullProgress(msg PullProgressMsg) {
	m.mu.Lock()
	defer m.mu.Unlock()

	idx, ok := m.taskIndex[msg.Name]
	if !ok {
		return
	}

	t := m.tasks[idx]
	t.Pull = PullProgress{Current: msg.Current, Total: msg.Total}
	m.tasks[idx] = t
	m.updateVisibleItem(t)
}

// handleResourceStats updates the Stats field of a running task
func (m *UI) handleResourceStats(msg ResourceStatsMsg) {
	m.mu.Lock()
	defer m.mu.Unlock()

	idx, ok := m.taskIndex[msg.Name]
	if !ok {
		return
	}

	t := m.tasks[idx]
	t.Stats = msg.Stats
	m.tasks[idx] = t
	m.updateVisibleItem(t)
}

// handleTick handles the UI's own low-frequency tick (see TickMsg) for
// spinner animation. Only non-terminal tasks animate, so only they need
// updating and re-rendering.
func (m *UI) handleTick(msg TickMsg) []tea.Cmd {
	for i, t := range m.tasks {
		if isTaskFinished(t.Status) {
			continue
		}

		loader, _ := t.loader.Update(t.loader.Tick())
		t.loader = loader
		m.tasks[i] = t
		m.updateVisibleItem(t)
	}
	return nil
}

// updateLastSelected updates the last selected item
func (m *UI) updateLastSelected() {
	if selectedItem := m.list.SelectedItem(); selectedItem != nil {
		task := selectedItem.(TaskMsg)
		if task.viewport != nil {
			// Initialize viewport dimensions if not set
			if task.viewport.Width == 0 || task.viewport.Height == 0 {
				m.updateViewportDimensions(&task)
			}
			m.lastSelected = selectedItem
		}
	}
}

// View renders the UI
func (m UI) View() tea.View {
	m.logger.Info("tui view", "height", m.height, "width", m.width, "last", m.lastSelected)

	var content string
	if m.lastSelected == nil || m.height == 0 || m.width == 0 {
		content = m.loader.View()
	} else {
		content = m.renderMainLayout()
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// renderMainLayout renders the main UI layout
func (m UI) renderMainLayout() string {
	headerPanel := m.renderHeaderPanel()
	listPanel := m.renderListPanel()
	pagerPanel := m.renderPagerPanel()
	bottomPanel := m.renderBottomPanel()

	// Stack panels vertically if the terminal is too narrow
	if m.width < AlignHorizontalBreakpoint {
		return lipgloss.JoinVertical(
			lipgloss.Top,
			listPanel,
			headerPanel,
			pagerPanel,
			bottomPanel,
		)
	}

	// Otherwise use horizontal layout
	return lipgloss.JoinVertical(
		lipgloss.Top,
		headerPanel,
		lipgloss.JoinHorizontal(lipgloss.Top, listPanel, pagerPanel),
		bottomPanel,
	)
}

// renderHeaderPanel renders the combined header panel
func (m UI) renderHeaderPanel() string {
	var listStyle lipgloss.Style
	var pagerStyle lipgloss.Style
	var activeStyle = lipgloss.NewStyle().Foreground(activePanelColor)
	var pagerHeader string

	if m.activePanel == PanelList {
		listStyle = listStyle.Foreground(activePanelColor)
		pagerStyle = listStyle.Foreground(inactivePanelColor)
	} else {
		listStyle = listStyle.Foreground(inactivePanelColor)
		pagerStyle = listStyle.Foreground(activePanelColor)
	}
	task := m.lastSelected.(TaskMsg)

	if m.width < AlignHorizontalBreakpoint {
		tab := activeStyle.Render(" ⇅ ")
		line1 := strings.Repeat("─", max(0, (((m.width-4)/2)-10-lipgloss.Width(task.GetName()))))
		line2 := strings.Repeat("─", max(0, ((m.width-5)/2)))

		pagerHeader = fmt.Sprintf("%s%s %s %s %s %s%s%s",
			pagerStyle.Render("┌"),
			pagerStyle.Render("─── ·"),
			topTitleStyle.Render(task.GetName()),
			pagerStyle.Render("·"),
			pagerStyle.Render(line1),
			pagerStyle.Render(tab),
			pagerStyle.Render(line2),
			pagerStyle.Render("┐"),
		)

		return lipgloss.JoinVertical(lipgloss.Top, pagerHeader)
	}

	listHeader := listStyle.
		Render(strings.Repeat("─", max(0, m.list.Width()-2)))

	if m.lastSelected != nil {
		headerWidth := max(0, task.viewport.Width-10-lipgloss.Width(task.GetName()))
		pagerHeader = fmt.Sprintf("%s %s %s %s",
			pagerStyle.Render("─── ·"),
			topTitleStyle.Render(task.GetName()),
			pagerStyle.Render("·"),
			pagerStyle.Render(strings.Repeat("─", headerWidth)),
		)
	}

	tab := activeStyle.Render(" ⇆ ")
	return lipgloss.JoinHorizontal(lipgloss.Top, listHeader, tab, pagerHeader)
}

// renderListPanel renders the left list panel
func (m UI) renderListPanel() string {
	listPanelContent := []string{m.list.View()}

	if m.list.FilterState() > 0 {
		listPanelContent = append(listPanelContent, m.list.FilterInput.View())
		m.list.SetHeight(m.list.Height() - FilterInputHeightOffset)
	}

	active := m.activePanel == PanelList
	narrow := m.width < AlignHorizontalBreakpoint

	// No bottom border here — the stats bar below supplies it. Rendering
	// the stats bar as its own top-level bordered block (rather than
	// appending it into listPanelContent, to then be re-rendered through
	// this Width()-applying style) matters: lipgloss trims trailing
	// whitespace off nested content before repadding it to a style's own
	// Width, which would strip the stats bar's background fill and leave
	// it looking cut short instead of spanning the full panel width.
	style := listStyle
	if active {
		style = style.BorderForeground(activePanelColor)
	} else {
		style = style.BorderForeground(inactivePanelColor)
	}
	if narrow {
		style = style.Border(lipgloss.NormalBorder(), false, true, false, true)
	} else {
		style = style.Border(lipgloss.NormalBorder(), false, true, false, false)
	}

	header := m.renderListHeader()
	list := style.
		Height(m.list.Height()).
		Width(m.list.Width()).
		Render(lipgloss.JoinVertical(lipgloss.Top, listPanelContent...))

	statsBar := m.renderListStats(active, narrow)

	return lipgloss.JoinVertical(lipgloss.Top, header, list, statsBar)
}

func (m UI) renderListHeader() string {
	listWidth := m.list.Width() - StatusColumnWidth - 2 // Account for status and padding
	layout := computeColumnLayout(listWidth)

	headerStyle := listHeaderStyle
	if m.activePanel == PanelList {
		headerStyle = headerStyle.Background(activePanelColor).BorderForeground(activePanelColor)
	} else {
		headerStyle = headerStyle.Background(inactivePanelColor).BorderForeground(inactivePanelColor)
	}
	if m.width < AlignHorizontalBreakpoint {
		// Close off the top of the box and mirror the list panel's left
		// border added in vertical layout. The extra top border line adds
		// a second line to the block, so bump the height cap to match.
		headerStyle = headerStyle.Border(lipgloss.NormalBorder(), true, true, false, true).MaxHeight(2)
	}

	cols := []string{
		listColumnStyle.Width(layout.nameWidth).Render(ellipsis("TASK", layout.nameWidth)),
		listColumnStyle.Width(layout.labelsWidth).Render("LABELS"),
	}
	if layout.showCPU {
		cols = append(cols, listColumnStyle.Width(layout.cpuWidth).Align(lipgloss.Right).Render("CPU"))
	}
	if layout.showMem {
		cols = append(cols, listColumnStyle.Width(layout.memWidth).Align(lipgloss.Right).Render("MEM"))
	}
	if layout.showNetRx {
		cols = append(cols, listColumnStyle.Width(layout.netRxWidth).Align(lipgloss.Right).Render("NET RX"))
	}
	if layout.showNetTx {
		cols = append(cols, listColumnStyle.Width(layout.netTxWidth).Align(lipgloss.Right).Render("NET TX"))
	}
	if layout.showDiskR {
		cols = append(cols, listColumnStyle.Width(layout.diskRWidth).Align(lipgloss.Right).Render("DISK R"))
	}
	if layout.showDiskW {
		cols = append(cols, listColumnStyle.Width(layout.diskWWidth).Align(lipgloss.Right).Render("DISK W"))
	}
	cols = append(cols, listColumnStyle.Width(layout.durationWidth).Align(lipgloss.Right).Render("DUR"))

	return headerStyle.Width(m.list.Width()).Render("  " + strings.Join(cols, " "))
}

// renderListStats renders the bottom bar of the list panel, summarizing how
// many of the total tasks are left to run versus finished. It closes off
// the list body's box (see renderListPanel) with its own bottom border, so
// active/narrow need to match the body's border styling.
func (m UI) renderListStats(active, narrow bool) string {
	total := len(m.tasks)
	done := 0
	for _, t := range m.tasks {
		if isTaskFinished(t.Status) {
			done++
		}
	}

	text := fmt.Sprintf("Total: %d │ Left: %d │ Done: %d", total, total-done, done)

	style := listStatsStyle.Width(m.list.Width())
	if active {
		style = style.Background(activePanelColor).BorderForeground(activePanelColor)
	} else {
		style = style.Background(inactivePanelColor).BorderForeground(inactivePanelColor)
	}
	if narrow {
		style = style.Border(lipgloss.NormalBorder(), false, true, true, true)
	} else {
		style = style.Border(lipgloss.NormalBorder(), false, true, true, false)
	}

	return style.Render(text)
}

func (m UI) renderPagerPanel() string {
	task := m.lastSelected.(TaskMsg)

	m.updatePanelStyles()
	m.updateViewportDimensions(&task)

	detailsContent := m.buildPagerContent(task)

	style := viewportStyle
	if m.width < AlignHorizontalBreakpoint {
		// In vertical layout the pager isn't flanked by anything on the
		// right, so add a right border to close off the box.
		style = style.Border(lipgloss.NormalBorder(), false, true, true, true)
	}

	return lipgloss.JoinVertical(
		lipgloss.Top,
		style.Render(lipgloss.JoinVertical(lipgloss.Top, detailsContent...)),
	)
}

// updatePanelStyles updates the styles based on the active panel
func (m *UI) updatePanelStyles() {
	if m.activePanel == PanelList {
		topStyle = topStyle.Foreground(inactivePanelColor)
		topTitleStyle = topTitleStyle.Foreground(inactivePanelColor)
		viewportStyle = viewportStyle.BorderForeground(inactivePanelColor)
		m.lastSelected.(TaskMsg).viewport.Styles.LineNumber = lineNumberInactiveStyle
		return
	}

	topStyle = topStyle.Foreground(activePanelColor)
	topTitleStyle = newStyle()
	viewportStyle = viewportStyle.BorderForeground(activePanelColor)
	m.lastSelected.(TaskMsg).viewport.Styles.LineNumber = lineNumberActiveStyle
}

// updateViewportDimensions updates the viewport dimensions
func (m *UI) updateViewportDimensions(task *TaskMsg) {
	// Set viewport width based on layout
	if m.width < AlignHorizontalBreakpoint {
		// In vertical layout, viewport takes full width, minus the left
		// and right borders drawn around the pager panel.
		task.viewport.Width = m.width - 2
		// Height is reduced by list height and bottom panel
		task.viewport.Height = m.height - m.list.Height() - LayoutAreaHeightNarrow
	} else {
		// In horizontal layout, viewport takes remaining width
		task.viewport.Width = m.width - m.list.Width()
		task.viewport.Height = m.height - LayoutAreaHeight + 1
	}

	if task.LabelsAsString() != "" {
		task.viewport.Height -= LabelsHeightOffset
	}
}

// buildPagerContent builds the content for the details panel
func (m UI) buildPagerContent(task TaskMsg) []string {
	var content []string

	// Add labels if present
	if labels := task.LabelsAsString(); labels != "" {
		content = append(content, lipgloss.NewStyle().
			Width(task.viewport.Width).
			Render(labels))
	}

	// Add viewport content
	content = append(content, task.viewport.View())

	return content
}

// renderBottomPanel renders the bottom status panel
func (m UI) renderBottomPanel() string {
	status := m.renderStatus()
	scrollPercentage := scrollPercentageStyle.Render(
		fmt.Sprintf("%3.f%%", m.lastSelected.(TaskMsg).viewport.ScrollPercent()*100))
	delimiter := helpDelimiterStyle.Render("│")

	helpWidth := max(0, m.width-lipgloss.Width(status)-lipgloss.Width(scrollPercentage)-lipgloss.Width(delimiter))
	m.help.SetWidth(helpWidth)

	// MaxHeight(1): bubbles' own ShortHelpView doesn't strictly respect the
	// width passed to SetWidth — when there's no room left for its "…"
	// ellipsis, it renders the last oversized item anyway rather than
	// dropping it. Without this cap, that overflow would wrap onto a
	// second line here (Width() alone wraps rather than truncates), which
	// pushes this entire bottom bar's content down and, since nothing
	// budgets height for that extra line, off the bottom of the screen.
	return lipgloss.JoinHorizontal(
		lipgloss.Bottom,
		status,
		delimiter,
		lipgloss.NewStyle().Width(helpWidth).MaxHeight(1).Render(m.help.View(uiKeyMap{})),
		scrollPercentage,
	)
}
