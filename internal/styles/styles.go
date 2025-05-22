package styles

import (
	"fmt"
	"math/rand"

	"github.com/charmbracelet/lipgloss"
)

var (
	Bold     = lipgloss.NewStyle().Bold(true)
	TagLabel = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#000000"}).PaddingRight(1).PaddingLeft(1)
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
