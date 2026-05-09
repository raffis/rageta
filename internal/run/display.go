package run

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/raffis/rageta/internal/display"
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/setup/flagset"
	"github.com/raffis/rageta/internal/tui"
	"github.com/raffis/rageta/internal/xio"
	"golang.org/x/term"
)

type RenderDisplay string

var (
	RenderDisplayPrefix                RenderDisplay = "prefix"
	RenderDisplayUI                    RenderDisplay = "ui"
	RenderDisplayPassthrough           RenderDisplay = "passthrough"
	RenderDisplayDiscard               RenderDisplay = "discard"
	RenderDisplayBuffer                RenderDisplay = "buffer"
	RenderDisplayBufferDefaultTemplate string        = "{{ .Buffer }}"
)

func (d RenderDisplay) String() string {
	return string(d)
}

type DisplayOptions struct {
	Display       string
	Expand        bool
	InternalSteps bool
}

func (s *DisplayOptions) BindFlags(flags flagset.Interface) {
	flags.StringVarP(&s.Display, "display", "o", s.Display, "Display renderer. One of [prefix, ui, buffer[=gotpl], passthrough, discard]. The default `prefix` adds a step name prefix with a distinguished color while `ui` renders the tasks in a terminal ui. `passthrough` dumps all displays directly without any modification.")
	flags.BoolVarP(&s.Expand, "expand", "", s.Expand, "Expand steps from inherited pipelines and display them as separate entities.")
	flags.BoolVarP(&s.InternalSteps, "with-internals", "", s.InternalSteps, "Expose internal steps")
}

func (s DisplayOptions) Build() Step {
	return &Display{opts: s}
}

func NewDisplayOptions() DisplayOptions {
	return DisplayOptions{
		Display: electDefaultDisplay(),
	}
}

func electDefaultDisplay() string {
	if term.IsTerminal(int(os.Stdout.Fd())) {
		return RenderDisplayUI.String()
	}

	return RenderDisplayPrefix.String()
}

type Display struct {
	opts    DisplayOptions
	tuiApp  *tea.Program
	model   tui.UI
	tuiDone chan struct{}
}

type DisplayContext struct {
	Factory       processor.DisplayFactory
	Expand        bool
	InternalSteps bool
	Type          string
	Stdout        io.Writer
	Stderr        io.Writer
	Stdin         io.Reader
}

func (s *Display) Run(rc *RunContext, next Next) error {
	displayFactory, err := s.buildDisplayFactory(rc)
	if err != nil {
		return err
	}

	if s.opts.Display == RenderDisplayUI.String() {
		rc.Logging.Logger = rc.Logging.FileLogger
	}

	rc.Display.Factory = displayFactory
	rc.Display.Expand = s.opts.Expand
	rc.Display.InternalSteps = s.opts.InternalSteps
	rc.Display.Type = s.opts.Display

	err = next(rc)
	if s.tuiApp == nil {
		return err
	}

	if err != nil {
		s.tuiApp.Send(tui.PipelineDoneMsg{Status: tui.StepStatusFailed, Error: err})
	} else {
		s.tuiApp.Send(tui.PipelineDoneMsg{Status: tui.StepStatusDone, Error: nil})
	}

	<-s.tuiDone

	return err
}

func (s *Display) buildDisplayFactory(rc *RunContext) (processor.DisplayFactory, error) {
	displayOpt := strings.Split(s.opts.Display, "=")
	renderer := displayOpt[0]
	opts := ""
	if len(displayOpt) == 2 {
		opts = displayOpt[1]
	}

	switch renderer {
	case RenderDisplayUI.String():
		return display.UI(s.uiDisplay(rc)), nil
	case RenderDisplayPrefix.String():
		return display.Prefix(rc.Display.Stdout, rc.Display.Stderr), nil
	case RenderDisplayPassthrough.String():
		return display.Passthrough(rc.Display.Stdout, rc.Display.Stderr), nil
	case RenderDisplayDiscard.String():
		return display.Discard(), nil
	case RenderDisplayBuffer.String():
		if opts == "" {
			opts = RenderDisplayBufferDefaultTemplate
		}
		tmpl, err := template.New("display").Parse(opts)
		if err != nil {
			return nil, fmt.Errorf("failed to parse report buffer template: %w", err)
		}
		return display.Buffer(tmpl, rc.Display.Stdout), nil
	default:
		return nil, fmt.Errorf("invalid display type given: %s", s.opts.Display)
	}
}

func (s *Display) uiDisplay(rc *RunContext) *tea.Program {
	if s.opts.Display != RenderDisplayUI.String() {
		return nil
	}

	if s.tuiApp != nil {
		return s.tuiApp
	}

	s.tuiDone = make(chan struct{})

	model := tui.NewUI(rc.Logging.Logger.WithValues("component", "tui"))
	s.tuiApp = tea.NewProgram(model,
		tea.WithOutput(xio.NewFDWrapper(rc.Display.Stdout, os.Stdout)),
		tea.WithEnvironment(bubbleTeaProgramEnv()),
	)
	s.model = model

	go func() {
		for c := range time.Tick(100 * time.Millisecond) {
			s.tuiApp.Send(tui.TickMsg(c))
		}
	}()

	go func() {
		_, _ = s.tuiApp.Run()
		//rc.Cancel()
		s.tuiDone <- struct{}{}
	}()
	return s.tuiApp
}

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
func bubbleTeaProgramEnv() []string {
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
