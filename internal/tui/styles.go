package tui

import (
	"os"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
	"github.com/raffis/rageta/internal/styles"
)

var uiDebug = false

func newStyle() lipgloss.Style {
	style := lipgloss.NewStyle()

	if uiDebug {
		return style.Background(styles.RandAdaptiveColor()).BorderBackground(styles.RandAdaptiveColor())
	}

	return style
}

var (
	stepOkStyle           lipgloss.Style
	stepFailedStyle       lipgloss.Style
	stepWaitingStyle      lipgloss.Style
	stepWarningStyle      lipgloss.Style
	stepRunningStyle      lipgloss.Style
	pipelineOkStyle       lipgloss.Style
	pipelineFailedStyle   lipgloss.Style
	pipelineWaitingStyle  lipgloss.Style
	listStyle             lipgloss.Style
	listColumnStyle       lipgloss.Style
	viewportStyle         lipgloss.Style
	listPaginatorStyle    lipgloss.Style
	scrollPercentageStyle lipgloss.Style
	topStyle              lipgloss.Style
	topTitleStyle         lipgloss.Style
	durationStyle         lipgloss.Style

	lineNumberActiveStyle   lipgloss.Style
	lineNumberInactiveStyle lipgloss.Style
	listTagLabelStyle       lipgloss.Style

	activePanelColor = lipgloss.Color("#7D56F4")
	lightGrey        = compat.AdaptiveColor{
		Light: lipgloss.Color("#909090"),
		Dark:  lipgloss.Color("#626262"),
	}
	inactivePanelColor        = lightGrey
	inactiveFordergroundColor = compat.AdaptiveColor{
		Dark:  lipgloss.Color("#CCCCCC"),
		Light: lipgloss.Color("#CCCCCC"),
	}
)

func init() {
	uiDebug = os.Getenv("RAGETA_TUI_DEBUG") != ""

	stepOkStyle = newStyle().Foreground(lipgloss.Color("#008000"))
	stepFailedStyle = newStyle().Foreground(lipgloss.Color("#D22B2B"))
	stepWaitingStyle = newStyle().Foreground(lipgloss.Color("#0000FF"))
	stepWarningStyle = newStyle().Foreground(lipgloss.Color("#FFC300"))
	stepRunningStyle = newStyle().Foreground(lipgloss.Color("#FFC0CB"))

	lineNumberInactiveStyle = newStyle().
		Background(inactivePanelColor).
		Foreground(inactiveFordergroundColor).
		MarginRight(1).
		AlignHorizontal(lipgloss.Right)

	lineNumberActiveStyle = newStyle().
		Background(lipgloss.Color("#7D56F4")).
		Foreground(lipgloss.Color("#FFFFFF")).
		MarginRight(1).
		AlignHorizontal(lipgloss.Right)

	pipelineOkStyle = newStyle().Padding(0, 1).Height(1).Background(lipgloss.Color("#008000"))
	pipelineFailedStyle = newStyle().Padding(0, 1).Height(1).Background(lipgloss.Color("#D22B2B"))
	pipelineWaitingStyle = newStyle().Padding(0, 1).Height(1).Background(lipgloss.Color("#0000FF"))

	listTagLabelStyle = newStyle().PaddingRight(1)
	listStyle = newStyle().
		BorderForeground(activePanelColor).
		Border(lipgloss.NormalBorder(), false, true, true, false)

	listColumnStyle = newStyle().MaxHeight(1)
	durationStyle = newStyle().Foreground(lightGrey)

	topStyle = lipgloss.NewStyle()
	topTitleStyle = newStyle()

	viewportStyle = newStyle().
		BorderForeground(inactivePanelColor).
		Border(lipgloss.NormalBorder(), false, false, true, true)

	listPaginatorStyle = newStyle().Padding(1, 0, 2, 2)

	scrollPercentageStyle = newStyle().
		Foreground(lipgloss.Color("#CCCCCC")).
		Padding(0, 1)
}
