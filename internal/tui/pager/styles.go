package pager

import "github.com/charmbracelet/lipgloss"

type Styles struct {
	LineNumber  lipgloss.Style
	MatchResult lipgloss.Style
}

// DefaultStyles returns a set of default style definitions for this component.
func DefaultStyles() (s Styles) {
	s.LineNumber = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#DDDADA", Dark: "#3C3C3C"}).
		MarginRight(1).
		AlignHorizontal(lipgloss.Right)

	s.MatchResult = lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Light: "#9B9B9B", Dark: "#5C5C5C"})
	return
}
