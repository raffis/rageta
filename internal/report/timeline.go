package report

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

func Timeline(w io.Writer, steps []stepResult) error {
	if len(steps) == 0 {
		return nil
	}

	// Get terminal width dynamically
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width < 30 {
		width = 80
	}

	// Find min start time and max end time
	minTime := steps[0].result.StartedAt
	maxTime := steps[0].result.EndedAt
	maxNameLen := len(steps[0].stepName)

	for _, step := range steps {
		if len(step.stepName) > maxNameLen {
			maxNameLen = len(step.stepName)
		}
		if step.result.StartedAt.Before(minTime) {
			minTime = step.result.StartedAt
		}
		if step.result.EndedAt.After(maxTime) {
			maxTime = step.result.EndedAt
		}
	}

	barWidth := width - maxNameLen - 15
	totalDuration := maxTime.Sub(minTime)
	if totalDuration == 0 {
		return nil
	}

	barStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Background(lipgloss.Color("2"))

	// Render each step
	var result string
	for _, step := range steps {
		endedAt := step.result.EndedAt
		if endedAt.IsZero() {
			endedAt = maxTime
		}

		startOffset := int(float64(barWidth) * step.result.StartedAt.Sub(minTime).Seconds() / totalDuration.Seconds())
		endOffset := int(float64(barWidth) * endedAt.Sub(minTime).Seconds() / totalDuration.Seconds())

		barLength := endOffset - startOffset
		if barLength < 1 {
			barLength = 1
		}

		bar := barStyle.Render(strings.Repeat("█", barLength))
		line := fmt.Sprintf("%-*s | %*s%s\n", maxNameLen, step.stepName, startOffset, "", bar)
		result += line
	}

	timeAxis := renderTimeAxis(minTime, maxTime, barWidth, maxNameLen)
	_, err = w.Write([]byte(result + timeAxis))
	return err
}

// renderTimeAxis generates a time axis with labels at even intervals
func renderTimeAxis(minTime, maxTime time.Time, width, labelOffset int) string {
	tickInterval := maxTime.Sub(minTime) / 5
	timeLabels := fmt.Sprintf("%*s", labelOffset+3, "")

	for i := 0; i <= 5; i++ {
		t := minTime.Add(time.Duration(i) * tickInterval)
		label := t.Format("15:04:05") // Format HH:MM:SS
		pos := int(float64(width) * float64(i) / 5)
		timeLabels += fmt.Sprintf("%*s %s", pos-len(label)/2, "", label)
	}

	return timeLabels + "\n"
}
