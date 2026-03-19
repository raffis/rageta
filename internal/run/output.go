package run

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/raffis/rageta/internal/output"
	"github.com/raffis/rageta/internal/pipeline"
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/tui"
	"github.com/raffis/rageta/internal/xio"
	"github.com/spf13/pflag"
	"golang.org/x/term"
)

type renderOutput string

var (
	renderOutputPrefix                renderOutput = "prefix"
	renderOutputUI                    renderOutput = "ui"
	renderOutputPassthrough           renderOutput = "passthrough"
	renderOutputDiscard               renderOutput = "discard"
	renderOutputBuffer                renderOutput = "buffer"
	renderOutputBufferDefaultTemplate string       = "{{ .Buffer }}"
)

func (d renderOutput) String() string {
	return string(d)
}

type OutputOptions struct {
	Output        string
	Expand        bool
	InternalSteps bool
}

func (s *OutputOptions) BindFlags(flags *pflag.FlagSet) {
	flags.StringVarP(&s.Output, "output", "o", s.Output, "Output renderer. One of [prefix, ui, buffer[=gotpl], passthrough, discard]. The default `prefix` adds a colored task name prefix to the output lines while `ui` renders the tasks in a terminal ui. `passthrough` dumps all outputs directly without any modification.")
	flags.BoolVarP(&s.Expand, "expand", "", s.Expand, "Expand steps from inherited pipelines and display them as separate entities.")
	flags.BoolVarP(&s.InternalSteps, "with-internals", "", s.InternalSteps, "Expose internal steps")
}

func (s OutputOptions) Build() Step {
	return &Output{opts: s}
}

func NewOutputOptions() OutputOptions {
	return OutputOptions{
		Output: electDefaultOutput(),
	}
}

func electDefaultOutput() string {
	if term.IsTerminal(int(os.Stdout.Fd())) {
		return renderOutputUI.String()
	}

	return renderOutputPrefix.String()
}

type Output struct {
	opts    OutputOptions
	tuiApp  *tea.Program
	tuiDone chan struct{}
}

type OutputContext struct {
	Factory       processor.OutputFactory
	Expand        bool
	InternalSteps bool
	Type          string
	Stdout        io.Writer
	Stderr        io.Writer
}

func (s *Output) Run(rc *RunContext, next Next) error {
	outputFactory, err := s.buildOutputFactory(rc)
	if err != nil {
		return err
	}

	rc.Output.Factory = outputFactory
	rc.Output.Expand = s.opts.Expand
	rc.Output.InternalSteps = s.opts.InternalSteps
	rc.Output.Type = s.opts.Output

	err = next(rc)
	if s.tuiApp == nil {
		return err
	}

	if errors.Is(err, pipeline.ErrInvalidInput) {
		s.tuiApp.Quit()
	}

	if err != nil {
		s.tuiApp.Send(tui.PipelineDoneMsg{Status: tui.StepStatusFailed, Error: err})
	} else {
		s.tuiApp.Send(tui.PipelineDoneMsg{Status: tui.StepStatusDone, Error: nil})
	}

	<-s.tuiDone

	return err
}

func (s *Output) buildOutputFactory(rc *RunContext) (processor.OutputFactory, error) {
	outputOpt := strings.Split(s.opts.Output, "=")
	renderer := outputOpt[0]
	opts := ""
	if len(outputOpt) == 2 {
		opts = outputOpt[1]
	}

	switch renderer {
	case renderOutputUI.String():
		return output.UI(s.uiOutput(rc)), nil
	case renderOutputPrefix.String():
		return output.Prefix(rc.Output.Stdout, rc.Output.Stderr), nil
	case renderOutputPassthrough.String():
		return output.Passthrough(rc.Output.Stdout, rc.Output.Stderr), nil
	case renderOutputDiscard.String():
		return output.Discard(), nil
	case renderOutputBuffer.String():
		if opts == "" {
			opts = renderOutputBufferDefaultTemplate
		}
		tmpl, err := template.New("output").Parse(opts)
		if err != nil {
			return nil, fmt.Errorf("failed to parse report buffer template: %w", err)
		}
		return output.Buffer(tmpl, rc.Output.Stdout), nil
	default:
		return nil, fmt.Errorf("invalid output type given: %s", s.opts.Output)
	}
}

func (s *Output) uiOutput(rc *RunContext) *tea.Program {
	if s.opts.Output != renderOutputUI.String() {
		return nil
	}

	if s.tuiApp != nil {
		return s.tuiApp
	}

	s.tuiDone = make(chan struct{})

	model := tui.NewUI(rc.Logging.Logger.WithValues("component", "tui"))
	s.tuiApp = tea.NewProgram(model,
		tea.WithOutput(xio.NewFDWrapper(rc.Output.Stdout, os.Stdout)),
		tea.WithEnvironment(bubbleTeaProgramEnv()),
	)

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
// Bubble Tea v2 probes modes 2026/2027 via CSI when [shouldQuerySynchronizedOutput]
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
