package styles

import (
	"fmt"
	"math/rand"

	"github.com/charmbracelet/lipgloss"
)

var (
	Bold      = lipgloss.NewStyle().Bold(true)
	TagLabel  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#000000"}).PaddingRight(1).PaddingLeft(1)
	Highlight = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#CCC", Dark: "#666666"})
)

func RandHEXColor(min, max int) string {
	R := rand.Intn(max-min+1) + min
	G := rand.Intn(max-min+1) + min
	B := rand.Intn(max-min+1) + min
	return fmt.Sprintf("#%02x%02x%02x", R, G, B)
}

func RandAdaptiveColor() lipgloss.AdaptiveColor {
	return lipgloss.AdaptiveColor{
		Dark:  RandHEXColor(127, 255),
		Light: RandHEXColor(0, 127),
	}
}

func AdaptiveBrightnessColor(color lipgloss.TerminalColor) lipgloss.TerminalColor {
	r, g, b, a := color.RGBA()
	r8 := float64(r) / 257
	g8 := float64(g) / 257
	b8 := float64(b) / 257
	a8 := float64(a) / 257

	if a8 < 127 {
		return lipgloss.AdaptiveColor{
			Dark:  "#FFFFFF",
			Light: "#000000",
		}
	}

	brightness := (r8*299 + g8*587 + b8*114) / 1000
	if brightness < 128 {
		return lipgloss.Color("#FFFFFF")
	}

	return lipgloss.Color("#000000")
}
