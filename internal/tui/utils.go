package tui

import (
	"os"
	"strings"
)

// bubbleTeaProgramEnv is only passed to [tea.NewProgram] (not the whole process).
// Bubble Tea v2 probes modes 2026/2027 via CSI when [shouldQuerySynchronizedDisplay]
// is true (see charm.land/bubbletea/v2 tea.go). If the program exits before the
// terminal’s DECRQM replies are read, those bytes end up on stdin for the shell
// (e.g. "^[[?2026;4$y" / "2026;4$y2027;0$y").
//
// We adjust env so that function returns false: set TERM_PROGRAM to a value
// containing "Apple" (per bubbletea’s condition), drop WT_SESSION (otherwise
// Windows Terminal always opts into queries), and normalize TERM when it would
// still trigger queries by name (kitty, wezterm, …). [uv.Environ] uses the last
// assignment per key.
func BubbleTeaProgramEnv() []string {
	origTerm := strings.ToLower(os.Getenv("TERM"))
	base := os.Environ()
	out := make([]string, 0, len(base)+4)
	for _, e := range base {
		switch {
		case strings.HasPrefix(e, "WT_SESSION="):
			continue
		case strings.HasPrefix(e, "TERM_PROGRAM="):
			continue
		default:
			out = append(out, e)
		}
	}
	out = append(out, "TERM_PROGRAM=Apple_Terminal")
	for _, sub := range []string{"ghostty", "wezterm", "alacritty", "kitty", "rio"} {
		if strings.Contains(origTerm, sub) {
			out = append(out, "TERM=xterm-256color")
			break
		}
	}
	return out
}
