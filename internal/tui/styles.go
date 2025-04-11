package tui

import "github.com/charmbracelet/lipgloss"

var (
	taskOkStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))
	taskFailedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	taskWaitingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#0000FF"))
	taskWarningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00"))
	taskRunningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFC0CB"))

	taskTitle = lipgloss.NewStyle().Bold(true)

	lineNumberPrefixStyle = lipgloss.NewStyle().
				Background(lipgloss.AdaptiveColor{Light: "#174BFD", Dark: "#1D56F4"}).
				Foreground(lipgloss.Color("#FFFFFF")).
				MarginRight(1).
				AlignHorizontal(lipgloss.Right)

	pipelineOkStyle      = lipgloss.NewStyle().Padding(0, 1).Height(1).Background(lipgloss.Color("#00FF00"))
	pipelineFailedStyle  = lipgloss.NewStyle().Padding(0, 1).Height(1).Background(lipgloss.Color("#FF0000"))
	pipelineWaitingStyle = lipgloss.NewStyle().Padding(0, 1).Height(1).Background(lipgloss.Color("#0000FF"))

	docStyle = lipgloss.NewStyle()

	highlightColor = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	windowStyle    = lipgloss.NewStyle()
	listStyle      = lipgloss.NewStyle().
			BorderForeground(highlightColor).
			Border(lipgloss.BlockBorder(), false, true, false, false)
	listPaginatorStyle = lipgloss.NewStyle().Padding(1, 0, 2, 2)

	leftFooterPaddingStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#874BFD")).
		//		Padding(0, 1).
		Height(1)

	scrollPercentageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#000000")).
				Background(lipgloss.Color("#CCCCCC")).
				Align(lipgloss.Center).
				Padding(0, 1).
				Height(1)
)
