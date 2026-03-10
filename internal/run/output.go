package runner

import (
	"fmt"
	"os"
	"strings"
	"text/template"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/raffis/rageta/internal/output"
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/tui"
	"github.com/raffis/rageta/internal/xio"
)

const (
	renderOutputUI          = "ui"
	renderOutputPrefix      = "prefix"
	renderOutputPassthrough = "passthrough"
	renderOutputDiscard     = "discard"
	renderOutputBuffer      = "buffer"
	bufferDefaultTemplate   = "{{ .Buffer }}"
)

type OutputStep struct {
	output string
}

func WithOutput(output string) *OutputStep {
	return &OutputStep{output: output}
}

func (s *OutputStep) Run(rc *RunContext, next Next) error {
	outputFactory, err := s.buildOutputFactory(rc)
	if err != nil {
		return err
	}
	rc.OutputFactory = outputFactory
	return next(rc)
}

func (s *OutputStep) buildOutputFactory(rc *RunContext) (processor.OutputFactory, error) {
	outputOpt := strings.Split(s.output, "=")
	renderer := outputOpt[0]
	opts := ""
	if len(outputOpt) == 2 {
		opts = outputOpt[1]
	}

	switch renderer {
	case renderOutputUI:
		return output.UI(s.uiOutput(rc)), nil
	case renderOutputPrefix:
		return output.Prefix(rc.Stdout, rc.Stderr), nil
	case renderOutputPassthrough:
		return output.Passthrough(rc.Stdout, rc.Stderr), nil
	case renderOutputDiscard:
		return output.Discard(), nil
	case renderOutputBuffer:
		if opts == "" {
			opts = bufferDefaultTemplate
		}
		tmpl, err := template.New("output").Parse(opts)
		if err != nil {
			return nil, fmt.Errorf("failed to parse report buffer template: %w", err)
		}
		return output.Buffer(tmpl, rc.Stdout), nil
	default:
		return nil, fmt.Errorf("invalid output type given: %s", s.output)
	}
}

func (s *OutputStep) uiOutput(rc *RunContext) *tea.Program {
	if s.output != renderOutputUI {
		return nil
	}
	if rc.TUIApp != nil {
		return rc.TUIApp.(*tea.Program)
	}
	rc.TUIDone = make(chan struct{})
	model := tui.NewUI(rc.Logger.WithValues("component", "tui"))
	prog := tea.NewProgram(model, tea.WithOutput(xio.NewFDWrapper(rc.Stdout, os.Stdout)))
	rc.TUIApp = prog

	go func() {
		for c := range time.Tick(100 * time.Millisecond) {
			prog.Send(tui.TickMsg(c))
		}
	}()
	go func() {
		_, _ = prog.Run()
		_ = prog.ReleaseTerminal()
		rc.Cancel()
		rc.TUIDone <- struct{}{}
	}()
	return prog
}
