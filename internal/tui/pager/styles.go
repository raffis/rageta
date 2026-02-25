package pager

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

type Styles struct {
	LineNumber  lipgloss.Style
	MatchResult lipgloss.Style
}

// DefaultStyles returns a set of default style definitions for this component.
func DefaultStyles() (s Styles) {
	s.LineNumber = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{Light: lipgloss.Color("#DDDADA"), Dark: lipgloss.Color("#3C3C3C")}).
		MarginRight(1).
		AlignHorizontal(lipgloss.Right)

	s.MatchResult = lipgloss.NewStyle().Background(compat.AdaptiveColor{Light: lipgloss.Color("#9B9B9B"), Dark: lipgloss.Color("#5C5C5C")})
	return
}
