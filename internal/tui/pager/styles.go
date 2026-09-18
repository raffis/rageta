package pager

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

type Styles struct {
	LineNumber  lipgloss.Style
	MatchResult lipgloss.Style
	// SearchBox frames the search prompt across the full pager width.
	SearchBox lipgloss.Style
	// SearchPrompt styles the "❯" marker in front of the search input.
	SearchPrompt lipgloss.Style
	// SearchCount styles the "n/m" match counter shown once a search ran.
	SearchCount lipgloss.Style
}

// DefaultStyles returns a set of default style definitions for this component.
func DefaultStyles() (s Styles) {
	s.LineNumber = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{Light: lipgloss.Color("#DDDADA"), Dark: lipgloss.Color("#3C3C3C")}).
		MarginRight(1).
		AlignHorizontal(lipgloss.Right)

	s.MatchResult = lipgloss.NewStyle().Background(compat.AdaptiveColor{Light: lipgloss.Color("#9B9B9B"), Dark: lipgloss.Color("#5C5C5C")})

	s.SearchBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7D56F4")).
		Padding(0, 1)

	s.SearchPrompt = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Bold(true)

	s.SearchCount = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{Light: lipgloss.Color("#909090"), Dark: lipgloss.Color("#626262")})
	return
}
