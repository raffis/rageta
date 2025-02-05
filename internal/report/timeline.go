package report

import (
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/charmbracelet/lipgloss"
)

func Timeline(w io.Writer, store *Store) error {

	var totalDuration time.Duration
	for _, step := range store.steps {
		if duration := step.result.EndedAt.Sub(step.result.StartedAt); duration > totalDuration {
			totalDuration = duration
		}
	}

	width := getTerminalWidth() - 20 // Leave space for labels and padding
	if width < 30 {
		width = 30 // Ensure minimum width for readability
	}

	scale := float64(width) / float64(totalDuration.Milliseconds())
	var timeline strings.Builder

	divider := lipgloss.NewStyle().Foreground(lipgloss.Color("#555")).Render("│")
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#aaa"))

	timeline.WriteString(headerStyle.Render(" Time (ms): "))
	for i := 0; i <= width; i += width / 10 {
		label := fmt.Sprintf("%-4d", i*int(totalDuration.Milliseconds())/width)
		timeline.WriteString(label)
	}
	timeline.WriteString("\n" + headerStyle.Render(strings.Repeat("─", width+12)) + "\n")

	for _, step := range store.steps {
		duration := step.result.EndedAt.Sub(step.result.StartedAt)
		started := totalDuration - duration

		startPos := int(float64(started.Milliseconds()) * scale)
		endPos := startPos + int(float64(duration.Milliseconds())*scale)

		if endPos > width {
			endPos = width
		}

		spanBar := strings.Repeat(" ", startPos) + lipgloss.NewStyle().Background(lipgloss.Color("#CCCCCC")).Render(strings.Repeat("█", endPos-startPos))
		timeline.WriteString(fmt.Sprintf("%-10s %s %s\n", step.stepName, divider, spanBar))
	}

	timeline.WriteString(headerStyle.Render(strings.Repeat("─", width+12))) // Bottom border

	_, err := w.Write([]byte(timeline.String()))
	return err
}

// getTerminalWidth dynamically fetches the terminal's current width
func getTerminalWidth() int {
	var ws struct {
		Row, Col, Xpixel, Ypixel uint16
	}
	_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, uintptr(os.Stdout.Fd()), uintptr(unsafe.Pointer(&ws)), 0)
	return int(ws.Col)
}
