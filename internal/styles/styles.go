package styles

import (
	"fmt"
	"image/color"
	"math/rand"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

var (
	Bold      = lipgloss.NewStyle().Bold(true)
	TagLabel  = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{Light: lipgloss.Color("#FFFFFF"), Dark: lipgloss.Color("#000000")}).PaddingRight(1).PaddingLeft(1)
	Highlight = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{Light: lipgloss.Color("#CCCCCC"), Dark: lipgloss.Color("#666666")}).Padding(0, 0, 0, 0).Margin(0, 0, 0, 0)
)

func RandHEXColor(min, max int) string {
	R := rand.Intn(max-min+1) + min
	G := rand.Intn(max-min+1) + min
	B := rand.Intn(max-min+1) + min
	return fmt.Sprintf("#%02x%02x%02x", R, G, B)
}

func RandAdaptiveColor() compat.AdaptiveColor {
	return compat.AdaptiveColor{
		Dark:  lipgloss.Color(RandHEXColor(127, 255)),
		Light: lipgloss.Color(RandHEXColor(0, 127)),
	}
}

func AdaptiveBrightnessColor(c color.Color) color.Color {
	r, g, b, a := c.RGBA()
	r8 := float64(r) / 257
	g8 := float64(g) / 257
	b8 := float64(b) / 257
	a8 := float64(a) / 257

	if a8 < 127 {
		return compat.AdaptiveColor{
			Dark:  lipgloss.Color("#FFFFFF"),
			Light: lipgloss.Color("#000000"),
		}
	}

	brightness := (r8*299 + g8*587 + b8*114) / 1000
	if brightness < 128 {
		return lipgloss.Color("#FFFFFF")
	}

	return lipgloss.Color("#000000")
}
