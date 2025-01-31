package tui

import (
	"bytes"
	"embed"
	"image"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/disintegration/imaging"
	"github.com/lucasb-eyer/go-colorful"
)

//go:embed rocket.png
var logo embed.FS

func PrintLogo() (string, error) {
	imageContent, err := logo.ReadFile("rocket.png")
	if err != nil {
		return "", err
	}

	img, _, err := image.Decode(bytes.NewReader(imageContent))
	if err != nil {
		return "", err
	}

	width := 38
	img = imaging.Resize(img, width, 0, imaging.Lanczos)
	b := img.Bounds()
	imageWidth := b.Max.X
	h := b.Max.Y
	str := strings.Builder{}

	for heightCounter := 0; heightCounter < h; heightCounter += 2 {
		for x := 0; x < imageWidth; x++ {
			c1, _ := colorful.MakeColor(img.At(x, heightCounter))

			color1 := lipgloss.Color(c1.Hex())
			c2, _ := colorful.MakeColor(img.At(x, heightCounter+1))

			color2 := lipgloss.Color(c2.Hex())

			if color2 == "#000000" {
				str.WriteString(lipgloss.NewStyle().Render(" "))

			} else {
				str.WriteString(lipgloss.NewStyle().Foreground(color1).
					Background(color2).Render("▀"))
			}

		}

		str.WriteString("\n")
	}

	return str.String(), nil
}
