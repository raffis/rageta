package run

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"

	"github.com/raffis/rageta/internal/output"
	"github.com/raffis/rageta/internal/processor"
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
	opts OutputOptions
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

	return next(rc)
}

func (s *Output) buildOutputFactory(rc *RunContext) (processor.OutputFactory, error) {
	outputOpt := strings.Split(s.opts.Output, "=")
	renderer := outputOpt[0]
	opts := ""
	if len(outputOpt) == 2 {
		opts = outputOpt[1]
	}

	switch renderer {
	//case renderOutputUI.String():
	//return output.UI(s.uiOutput(rc)), nil
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

/*

func (s *Output) uiOutput(rc *RunContext) *tea.Program {
	if s.opts.Output != renderOutputUI.String() {
		return nil
	}
	if rc.TUIApp != nil {
		return rc.TUIApp.(*tea.Program)
	}
	rc.TUIDone = make(chan struct{})
	model := tui.NewUI(rc.Logging.Logger.WithValues("component", "tui"))
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
*/
