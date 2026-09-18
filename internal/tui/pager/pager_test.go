package pager

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The search box is drawn across the pager's full width, so the query and its
// match counter sit on one line with the counter flush against the right edge.
func TestSearchViewSpansFullWidth(t *testing.T) {
	m := New(60, 20)
	m.SetContent("hello world\nfoo\nhello again\n")
	m.SetSearchState(Searching)
	m.filterInput.SetValue("hello")
	m.scan("hello")

	for _, width := range []int{20, 40, 60} {
		view := m.searchView(width)
		if got := lipgloss.Width(view); got != width {
			t.Errorf("searchView(%d) rendered width = %d, want %d", width, got, width)
		}

		row := ansi.Strip(strings.Split(view, "\n")[1])
		if !strings.HasSuffix(strings.TrimRight(row, " │"), "2 matches") {
			t.Errorf("searchView(%d) match counter isn't flush right: %q", width, row)
		}
	}
}

// Leaving the prompt — by running the search or by tabbing away — takes the
// box off screen, while the matches it found stay highlighted.
func TestExitSearchHidesPrompt(t *testing.T) {
	m := New(40, 10)
	m.SetContent("hello world\nfoo\nhello again\n")
	m.SetSearchState(Searching)
	m.filterInput.SetValue("hello")
	m.scan("hello")

	if !strings.Contains(m.View(), searchPrompt) {
		t.Fatal("search prompt not drawn while typing")
	}

	m.ExitSearch()

	if m.SearchState() != Searched {
		t.Errorf("search state = %v, want Searched", m.SearchState())
	}
	if strings.Contains(m.View(), searchPrompt) {
		t.Error("search prompt still drawn after leaving it")
	}
	if m.matchCount != 2 {
		t.Errorf("matches dropped on exit: got %d, want 2", m.matchCount)
	}
}

// Every key but escape and enter belongs to the search input while the prompt
// is open — none of the pager's single-key navigation bindings may fire, or
// they'd swallow the characters being typed.
func TestSearchPromptCapturesNavigationKeys(t *testing.T) {
	m := New(40, 5)
	m.SetContent(strings.Repeat("line\n", 100))
	m.SetYOffset(0)
	m.SetSearchState(Searching)

	for _, k := range []string{"n", "N", "/", " ", "a", "i", "q"} {
		var cmd tea.Cmd
		m, cmd = m.Update(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
		_ = cmd
	}

	if got := m.filterInput.Value(); got != "nN/ aiq" {
		t.Errorf("search input = %q, want %q", got, "nN/ aiq")
	}
	if m.YOffset != 0 {
		t.Errorf("pager scrolled to %d while typing a query", m.YOffset)
	}
}

// TestZeroSizedPagerKeepsContentVisible covers a pager that is written to
// before it is ever laid out: tasks produce output while the UI still has no
// window size, and a task whose panel never comes on screen is never resized
// at all. AutoScroll re-derives the offset on every Write, so an offset
// computed against a zero/negative size used to get pinned past the end of
// the content and the panel rendered empty even though it held the full log.
func TestZeroSizedPagerKeepsContentVisible(t *testing.T) {
	for _, size := range [][2]int{{0, 0}, {0, -8}, {80, -8}, {80, 0}} {
		m := New(size[0], size[1])
		m.ShowLineNumbers = true
		m.AutoScroll = true

		for i := range 10 {
			fmt.Fprintf(&m, "line %d\n", i)
		}

		if got := len(m.visibleLines()); got == 0 {
			t.Errorf("size %v: pager holds %d lines but renders none", size, len(m.lines))
		}
	}
}

// TestSetSizeReclampsStaleOffset covers the same pager once the real window
// size finally arrives: the offset left over from the unsized layout must be
// recomputed, not carried over, or the panel stays scrolled past its content
// until the next write happens to fix it.
func TestSetSizeReclampsStaleOffset(t *testing.T) {
	m := New(0, -8)
	m.ShowLineNumbers = true
	m.AutoScroll = true

	for i := range 10 {
		fmt.Fprintf(&m, "line %d\n", i)
	}

	m.SetSize(80, 20)

	if got := len(m.visibleLines()); got != 10 {
		t.Errorf("after resize: got %d visible lines, want all 10 (offset %d)", got, m.YOffset)
	}
}
