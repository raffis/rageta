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
	stepCachedStyle       lipgloss.Style
	stepFailedStyle       lipgloss.Style
	stepWaitingStyle      lipgloss.Style
	stepWarningStyle      lipgloss.Style
	stepRunningStyle      lipgloss.Style
	pipelineCachedStyle   lipgloss.Style
	pipelineOkStyle       lipgloss.Style
	pipelineFailedStyle   lipgloss.Style
	pipelineRunningStyle  lipgloss.Style
	pipelineWaitingStyle  lipgloss.Style
	listStyle             lipgloss.Style
	listColumnStyle       lipgloss.Style
	listHeaderStyle       lipgloss.Style
	viewportStyle         lipgloss.Style
	listPaginatorStyle    lipgloss.Style
	scrollPercentageStyle lipgloss.Style
	topStyle              lipgloss.Style
	topTitleStyle         lipgloss.Style
	durationStyle         lipgloss.Style
	helpDelimiterStyle    lipgloss.Style
	treeGuideStyle        lipgloss.Style

	lineNumberActiveStyle   lipgloss.Style
	lineNumberInactiveStyle lipgloss.Style
	listLabelStyle          lipgloss.Style
	listStatsStyle          lipgloss.Style
	selectedNameStyle       lipgloss.Style

	helpKeyStyle  lipgloss.Style
	helpDescStyle lipgloss.Style
	helpSepStyle  lipgloss.Style

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
	stepCachedStyle = newStyle().Foreground(lipgloss.Color("#00AACC"))
	stepFailedStyle = newStyle().Foreground(lipgloss.Color("#D22B2B"))
	stepWaitingStyle = newStyle().Foreground(lipgloss.Color("#FFC0CB"))
	stepWarningStyle = newStyle().Foreground(lipgloss.Color("#FFC300"))
	stepRunningStyle = newStyle().Foreground(lipgloss.Color("#0000FF"))

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
	pipelineRunningStyle = newStyle().Padding(0, 1).Height(1).Background(lipgloss.Color("#0000FF"))
	pipelineWaitingStyle = newStyle().Padding(0, 1).Height(1).Background(lipgloss.Color("#FFC0CB"))
	pipelineCachedStyle = newStyle().Padding(0, 1).Height(1).Background(lipgloss.Color("#00AACC"))

	listHeaderStyle = newStyle().Background(lipgloss.Color("#7D56F4")).
		Foreground(lipgloss.Color("#FFFFFF")).
		PaddingLeft(2).
		PaddingRight(2).
		BorderForeground(activePanelColor).
		Border(lipgloss.NormalBorder(), false, true, false, false).
		MaxHeight(1)

	listLabelStyle = newStyle().PaddingRight(1)
	selectedNameStyle = newStyle().Foreground(activePanelColor)
	// No MaxHeight here (unlike listColumnStyle/durationStyle): this style
	// also carries the stats bar's closing bottom border (added in
	// renderListStats), which needs its own extra line — a MaxHeight(1)
	// would crop that border line away.
	listStatsStyle = newStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lightGrey).
		PaddingLeft(2)
	listStyle = newStyle().
		BorderForeground(activePanelColor).
		Border(lipgloss.NormalBorder(), false, true, true, false)

	listColumnStyle = newStyle().MaxHeight(1)
	// MaxHeight(1) guards against lipgloss wrapping (rather than truncating)
	// a duration string that's wider than the column's Width, e.g.
	// "15m30.46s" against the 8-char minimum — without it, the wrap inserts
	// a literal newline into the row and pushes duration onto its own line.
	durationStyle = newStyle().Foreground(lightGrey).MaxHeight(1)
	treeGuideStyle = newStyle().Foreground(lightGrey)

	topStyle = lipgloss.NewStyle()
	topTitleStyle = newStyle()

	viewportStyle = newStyle().
		BorderForeground(inactivePanelColor).
		Border(lipgloss.NormalBorder(), false, false, true, true)

	listPaginatorStyle = newStyle().Padding(1, 0, 2, 2)

	scrollPercentageStyle = newStyle().
		Foreground(lipgloss.Color("#CCCCCC")).
		Padding(0, 1)

	helpDelimiterStyle = newStyle().
		Foreground(inactivePanelColor).
		Padding(0, 1)

	// Brighter than bubbles' default help styles (which use very dim greys
	// tuned for a light background and are hard to read on a dark terminal).
	helpKeyStyle = newStyle().Foreground(lipgloss.Color("#CCCCCC"))
	helpDescStyle = newStyle().Foreground(lipgloss.Color("#909090"))
	helpSepStyle = newStyle().Foreground(lightGrey)
}
