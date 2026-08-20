package run

import (
	"context"
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
	Display string
	GroupBy []string
}

func (s *DisplayOptions) BindFlags(flags flagset.Interface) {
	flags.StringVarP(&s.Display, "display", "o", s.Display, "Display renderer. One of [prefix, ui, buffer[=gotpl], passthrough, discard]. The default `prefix` adds a step name prefix with a distinguished color while `ui` renders the tasks in a terminal ui. `passthrough` dumps all displays directly without any modification.")
}

func (s DisplayOptions) Build() Task {
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
	Factory processor.DisplayFactory
	Type    string
	Stdout  io.Writer
	Stderr  io.Writer
	Stdin   io.Reader
}

func (s *Display) Label() string {
	return "Preparing display"
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
	rc.Display.Type = s.opts.Display

	err = next(rc)
	if s.tuiApp == nil {
		return err
	}

	if err != nil {
		s.tuiApp.Send(tui.PipelineDoneMsg{Status: tui.TaskStatusFailed, Error: err})
	} else {
		if rc.Execute.ResultContext.Build.Cached {
			s.tuiApp.Send(tui.PipelineDoneMsg{Status: tui.TaskStatusCached, Error: nil})
		} else {
			s.tuiApp.Send(tui.PipelineDoneMsg{Status: tui.TaskStatusDone, Error: nil})
		}
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
	model := tui.NewUI(rc.Logging.FileLogger.WithValues("component", "tui"))
	model.SetDebugShell(func(stepCtx processor.TaskContext) (tea.ExecCommand, error) {
		return &debugShellExec{rc: rc, stepCtx: stepCtx}, nil
	})
	s.tuiApp = tea.NewProgram(model,
		tea.WithOutput(xio.NewFDWrapper(rc.Display.Stdout, os.Stdout)),
		tea.WithEnvironment(tui.BubbleTeaProgramEnv()),
		tea.WithFPS(60),
	)
	s.model = model

	go func() {
		for c := range time.Tick(100 * time.Millisecond) {
			s.tuiApp.Send(tui.TickMsg(c))
		}
	}()

	go func() {
		_, _ = s.tuiApp.Run()
		if rc.Cancel != nil {
			rc.Cancel()
		}
		s.tuiDone <- struct{}{}
	}()
	return s.tuiApp
}

// debugShellExec implements tea.ExecCommand so the TUI can spawn a debug shell for a
// selected task via tea.Exec, which releases the terminal for the duration of the
// interactive session and restores it to the TUI afterwards.
type debugShellExec struct {
	rc      *RunContext
	stepCtx processor.TaskContext
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func (d *debugShellExec) SetStdin(r io.Reader)  { d.stdin = r }
func (d *debugShellExec) SetStdout(w io.Writer) { d.stdout = w }
func (d *debugShellExec) SetStderr(w io.Writer) { d.stderr = w }

func (d *debugShellExec) Run() error {
	return RunDebugShell(context.Background(), d.rc, d.stepCtx, d.stdin, d.stdout, d.stderr)
}
