package pager

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SearchState represents the current state of the filter
type SearchState int

const (
	// Unsearched means no filter is active
	Unsearched SearchState = iota
	// Searching means the filter input is active
	Searching
	// Searching means the filter input is active
	Searched
)

// New returns a new model with the given width and height as well as default
// key mappings.
func New(width, height int) (m Model) {
	m.Width = width
	m.Height = height
	m.setInitialValues()
	m.Styles = DefaultStyles()
	m.initializeSearch()
	return m
}

// Model is the Bubble Tea model for this viewport element.
type Model struct {
	Width  int
	Height int
	KeyMap KeyMap

	// Whether or not to respond to the mouse. The mouse must be enabled in
	// Bubble Tea for this to work. For details, see the Bubble Tea docs.
	MouseWheelEnabled bool

	// The number of lines the mouse wheel will scroll. By default, this is 3.
	MouseWheelDelta int

	// YOffset is the vertical scroll position.
	YOffset int

	ShowLineNumbers bool
	AutoScroll      bool
	Styles          Styles
	Style           lipgloss.Style

	initialized  bool
	lines        []line
	lineEnd      bool
	scanString   string
	matchCount   int
	currentMatch int
	matchLines   []int
	searchState  SearchState
	filterInput  textinput.Model
}

type line struct {
	msg   string
	width int
}

func (m *Model) setInitialValues() {
	m.KeyMap = DefaultKeyMap()
	m.MouseWheelEnabled = true
	m.MouseWheelDelta = 3
	m.initialized = true
}

// Init exists to satisfy the tea.Model interface for composability purposes.
func (m Model) Init() tea.Cmd {
	return nil
}

// AtTop returns whether or not the viewport is at the very top position.
func (m Model) AtTop() bool {
	return m.YOffset <= 0
}

// AtBottom returns whether or not the viewport is at or past the very bottom
// position.
func (m Model) AtBottom() bool {
	return m.YOffset >= m.maxYOffset()
}

// ScrollPercent returns the amount scrolled as a float between 0 and 1.
func (m Model) ScrollPercent() float64 {
	if m.Height >= len(m.lines) {
		return 1.0
	}
	y := float64(m.YOffset)
	h := float64(m.Height)
	t := float64(len(m.lines))
	v := y / (t - h)
	return math.Max(0.0, math.Min(1.0, v))
}

// SetContent replaces the pager's text content.
func (m *Model) SetContent(s string) {
	s = strings.ReplaceAll(s, "\r\n", "\n") // normalize line endings
	s = strings.ReplaceAll(s, "\r", "")     // remove carriage returns (avoid breaking the ui)

	lines := strings.Split(s, "\n")
	m.lines = make([]line, 0, len(lines))

	for _, l := range lines {
		m.lines = append(m.lines, line{
			msg:   l,
			width: lipgloss.Width(l),
		})
	}

	if m.YOffset > len(m.lines)-1 || m.AutoScroll {
		m.GotoBottom()
	}
}

// Write adds new content to the pager
func (m *Model) Write(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}

	s := string(b)
	s = strings.ReplaceAll(s, "\r\n", "\n") // normalize line endings
	s = strings.ReplaceAll(s, "\r", "")     // remove carriage returns (avoid breaking the ui)

	if len(m.lines) > 0 && !m.lineEnd {
		lastLine := m.lines[len(m.lines)-1]
		m.lines = m.lines[:len(m.lines)-1]
		s = lastLine.msg + s
	}

	m.lineEnd = strings.HasSuffix(s, "\n")

	if m.lineEnd {
		s = strings.TrimSuffix(s, "\n")
	}

	for l := range strings.SplitSeq(s, "\n") {
		m.lines = append(m.lines, line{
			msg:   l,
			width: lipgloss.Width(l),
		})
	}

	if m.AutoScroll {
		m.GotoBottom()
	}

	return len(b), nil
}

// maxYOffset returns the maximum possible value of the y-offset based on the
// viewport's content and set height.
func (m Model) maxYOffset() int {
	var offset int

	for i := len(m.lines) - 1; i >= 0; i-- {
		offset += int(math.Ceil(float64(m.lines[i].width) / float64(m.Width)))

		if offset >= m.Height {
			return i
		}
	}

	return len(m.lines) - 1
}

// visibleLines returns the lines that should currently be visible in the
// viewport.
func (m Model) visibleLines() []string {
	var lines []string
	if len(m.lines) == 0 {
		return lines
	}

	top := max(0, m.YOffset)
	var contentHeight int

	if top >= len(m.lines) {
		top = len(m.lines)
	}

	for _, line := range m.lines[top:] {
		contentHeight += int(math.Ceil(float64(line.width) / float64(m.Width)))
		lines = append(lines, line.msg)

		if contentHeight >= m.Height {
			break
		}
	}

	return lines
}

// SetYOffset sets the Y offset.
func (m *Model) SetYOffset(n int) {
	m.YOffset = clamp(n, 0, m.maxYOffset())
}

// LineDown moves the view down by the given number of lines.
func (m *Model) LineDown(n int) {
	if m.AtBottom() || n == 0 || len(m.lines) == 0 {
		return
	}

	m.SetYOffset(m.YOffset + n)
}

// LineUp moves the view down by the given number of lines. Returns the new
// lines to show.
func (m *Model) LineUp(n int) {
	if m.AtTop() || n == 0 || len(m.lines) == 0 {
		return
	}

	m.SetYOffset(m.YOffset - n)
}

// TotalLineCount returns the total number of lines (both hidden and visible) within the viewport.
func (m Model) TotalLineCount() int {
	return len(m.lines)
}

// GotoBottom sets the viewport to the bottom position.
func (m *Model) GotoBottom() {
	m.SetYOffset(m.maxYOffset())
}

// Update handles standard message-based viewport updates.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd

	if !m.initialized {
		m.setInitialValues()
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.KeyMap.PageDown):
			m.LineDown(m.Height)

		case key.Matches(msg, m.KeyMap.PageUp):
			m.LineUp(m.Height)

		case key.Matches(msg, m.KeyMap.HalfPageDown):
			m.LineDown(m.Height / 2)

		case key.Matches(msg, m.KeyMap.HalfPageUp):
			m.LineUp(m.Height / 2)

		case key.Matches(msg, m.KeyMap.Down):
			m.LineDown(1)

		case key.Matches(msg, m.KeyMap.Up):
			m.LineUp(1)

		case key.Matches(msg, m.KeyMap.Search):
			m.searchState = Searching
			cmd = m.filterInput.Focus()

		case key.Matches(msg, m.KeyMap.NextMatch) && m.searchState == Searched:
			m.NextMatch()

		case key.Matches(msg, m.KeyMap.PrevMatch) && m.searchState == Searched:
			m.PrevMatch()

		case key.Matches(msg, m.KeyMap.ExitSearchMode):
			m.SetSearchState(Unsearched)
			m.filterInput.Reset()
			m.ScanAfter("")

		case m.searchState == Searching:
			m.filterInput, cmd = m.filterInput.Update(msg)
			if msg.String() == "enter" {
				m.searchState = Searched
				m.ScanAfter(m.filterInput.Value())
				m.filterInput.SetValue("")
			}
		}
	case tea.MouseMsg:
		if !m.MouseWheelEnabled || msg.Action != tea.MouseActionPress {
			break
		}
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.LineUp(m.MouseWheelDelta)

		case tea.MouseButtonWheelDown:
			m.LineDown(m.MouseWheelDelta)
		}
	}

	return m, cmd
}

func (m *Model) ScanAfter(str string) int {
	m.scanString = str
	m.matchLines = nil
	m.matchCount = 0
	m.currentMatch = 0

	if str == "" {
		return 0
	}

	// Find all matches
	for i, line := range m.lines {
		if strings.Contains(line.msg, str) {
			m.matchLines = append(m.matchLines, i)
			m.matchCount++
		}
	}

	if m.matchCount > 0 {
		// Find the first match after current position
		for i, line := range m.matchLines {
			if line >= m.YOffset {
				m.currentMatch = i
				m.SetYOffset(line)
				return line
			}
		}
		// If no match found after current position, wrap around to first match
		m.currentMatch = 0
		m.SetYOffset(m.matchLines[0])
		return m.matchLines[0]
	}

	return 0
}

func (m *Model) ScanBefore(str string) int {
	m.scanString = str
	m.matchLines = nil
	m.matchCount = 0
	m.currentMatch = 0

	if str == "" {
		return 0
	}

	// Find all matches
	for i, line := range m.lines {
		if strings.Contains(line.msg, str) {
			m.matchLines = append(m.matchLines, i)
			m.matchCount++
		}
	}

	if m.matchCount > 0 {
		// Find the first match before current position
		for i := len(m.matchLines) - 1; i >= 0; i-- {
			if m.matchLines[i] <= m.YOffset {
				m.currentMatch = i
				m.SetYOffset(m.matchLines[i])
				return m.matchLines[i]
			}
		}
		// If no match found before current position, wrap around to last match
		m.currentMatch = len(m.matchLines) - 1
		m.SetYOffset(m.matchLines[m.currentMatch])
		return m.matchLines[m.currentMatch]
	}

	return 0
}

func (m *Model) NextMatch() int {
	if m.matchCount == 0 {
		return 0
	}

	m.currentMatch = (m.currentMatch + 1) % m.matchCount
	m.SetYOffset(m.matchLines[m.currentMatch])
	return m.matchLines[m.currentMatch]
}

func (m *Model) PrevMatch() int {
	if m.matchCount == 0 {
		return 0
	}

	m.currentMatch = (m.currentMatch - 1 + m.matchCount) % m.matchCount
	m.SetYOffset(m.matchLines[m.currentMatch])
	return m.matchLines[m.currentMatch]
}

// View renders the viewport into a string.
func (m Model) View() string {
	w, h := m.Width, m.Height
	if sw := m.Style.GetWidth(); sw != 0 {
		w = min(w, sw)
	}
	if sh := m.Style.GetHeight(); sh != 0 {
		h = min(h, sh)
	}

	var lines []string
	if m.ShowLineNumbers {
		firstLine := m.YOffset
		if firstLine == 0 {
			firstLine = 1
		}

		lineNumber := max(0, firstLine)
		maxLines := len(m.lines)
		if maxLines < h {
			maxLines = h
		}

		width := lipgloss.Width(fmt.Sprintf("%d", clamp(firstLine+m.Height, lineNumber, maxLines)))

		// If we have fewer lines than the visible area, start from line 1
		if len(m.lines) < h {
			lineNumber = 1
			firstLine = 1
		}

		for _, line := range m.visibleLines() {
			// Highlight all matches in the line
			if m.scanString != "" {
				line = strings.ReplaceAll(line, m.scanString, m.Styles.MatchResult.Render(m.scanString))
			}
			lines = append(lines, m.Styles.LineNumber.Width(width).Render(fmt.Sprintf("%d", lineNumber))+line)
			lineNumber++
		}

		// Fill remaining height with empty line numbers
		for ; lineNumber <= firstLine+h-1; lineNumber++ {
			lines = append(lines, m.Styles.LineNumber.Width(width).Render(fmt.Sprintf("%d", lineNumber)))
		}
	} else {
		lines = m.visibleLines()
		// Highlight all matches in the lines
		if m.scanString != "" {
			for i, line := range lines {
				lines[i] = strings.ReplaceAll(line, m.scanString, m.Styles.MatchResult.Render(m.scanString))
			}
		}
	}

	contentWidth := w - m.Style.GetHorizontalFrameSize()
	contentHeight := h - m.Style.GetVerticalFrameSize()

	if m.searchState > 0 {
		contentHeight--
	}

	contents := lipgloss.NewStyle().
		Width(contentWidth).
		Height(contentHeight).
		MaxHeight(contentHeight).
		MaxWidth(contentWidth).
		Render(strings.Join(lines, "\n"))

	// Add filter input if active
	if m.searchState > 0 {
		contents = lipgloss.JoinVertical(lipgloss.Top,
			contents,
			m.filterInput.View(),
		)
	}

	return m.Style.Render(contents)
}

func clamp(v, low, high int) int {
	if high < low {
		low, high = high, low
	}
	return min(high, max(low, v))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// initializeSearch sets up the filter input component
func (m *Model) initializeSearch() {
	filterInput := textinput.New()
	filterInput.Prompt = "Search: "
	filterInput.CharLimit = 64
	m.filterInput = filterInput
}

// SearchState returns the current filter state
func (m Model) SearchState() SearchState {
	return m.searchState
}

// SetSearchState sets the filter state
func (m *Model) SetSearchState(state SearchState) {
	m.searchState = state
	if state == Searching {
		m.filterInput.Focus()
	} else {
		m.filterInput.Blur()
	}
}

// SearchInput returns the filter input model
func (m Model) SearchInput() textinput.Model {
	return m.filterInput
}
