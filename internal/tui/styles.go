package tui

import "github.com/charmbracelet/lipgloss"

var (
	taskOkStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))
	taskFailedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	taskWaitingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#0000FF"))
	taskWarningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00"))
	taskRunningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFC0CB"))

	lineNumberPrefixStyle = lipgloss.NewStyle().
				Background(highlightColor).
				Foreground(lipgloss.Color("#FFFFFF")).
				MarginRight(1).
				AlignHorizontal(lipgloss.Right)

	pipelineOkStyle      = lipgloss.NewStyle().Padding(0, 1).Height(1).Background(lipgloss.Color("#00FF00"))
	pipelineFailedStyle  = lipgloss.NewStyle().Padding(0, 1).Height(1).Background(lipgloss.Color("#FF0000"))
	pipelineWaitingStyle = lipgloss.NewStyle().Padding(0, 1).Height(1).Background(lipgloss.Color("#0000FF"))

	docStyle       = lipgloss.NewStyle()
	highlightColor = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	windowStyle    = lipgloss.NewStyle()

	listStyle = lipgloss.NewStyle().
			BorderForeground(highlightColor).
			Border(lipgloss.OuterHalfBlockBorder(), false, true, false, false).
			PaddingTop(1)
	listPaginatorStyle = lipgloss.NewStyle().Padding(1, 0, 2, 2)

	leftFooterPaddingStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#874BFD")).
				Padding(0, 1).
				Height(1)

	rightFooterPagerPercentageStyle = lipgloss.NewStyle().
					BorderForeground(highlightColor).Border(lipgloss.OuterHalfBlockBorder(), false, true, false, true).
					Height(1)
	rightFooterPagerPercentageTopStyle = lipgloss.NewStyle().
						BorderForeground(highlightColor).Border(lipgloss.OuterHalfBlockBorder(), true, true, false, true).
						Height(1)

	rightFooterPaddingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#874BFD")).
				Background(lipgloss.Color("#874BFD")).
				Padding(0, 1).
				Height(1)
)
