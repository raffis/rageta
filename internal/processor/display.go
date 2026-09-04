package processor

import (
	"io"

	"github.com/raffis/rageta/internal/stats"
	"github.com/raffis/rageta/internal/xio"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type Display interface {
	Stdout() io.Writer
	Stderr() io.Writer
	Events() io.Writer
	// Interrupt suspends normal task output to run f interactively against a
	// terminal. It hands f the stdin/stdout/stderr to use rather than
	// letting f capture os.Stdin/os.Stdout/os.Stderr itself, because in UI
	// mode bubbletea owns the real terminal and must hand off its input
	// reader through tea.Exec's SetStdin — grabbing os.Stdin directly races
	// bubbletea's own (best-effort, see tty.go's waitForReadLoop) attempt to
	// stop reading it first.
	Interrupt(f func(stdin io.Reader, stdout, stderr io.Writer) error) error
	Close(ctx TaskContext, err error) error
	WriteStats(sample *stats.Sample) error
	WritePullProgress(current, total int64) error
}

type DisplayFactory func(ctx TaskContext, taskName, short string) Display

func WithDisplay(outputFactory DisplayFactory) ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		stdio := &display{
			taskName:      spec.Name,
			short:         spec.Short,
			spec:          spec,
			outputFactory: outputFactory,
		}

		return stdio
	}
}

type display struct {
	taskName      string
	short         string
	spec          *v1beta1.Task
	outputFactory DisplayFactory
}

type DisplayContext struct {
	Stdout            io.Writer
	Stderr            io.Writer
	Demuxer           *xio.Demuxer
	Events            io.Writer
	Interrupt         func(f func(stdin io.Reader, stdout, stderr io.Writer) error) error `json:"-"`
	WriteStats        func(sample *stats.Sample) error                                    `json:"-"`
	WritePullProgress func(current, total int64) error                                    `json:"-"`
}

func (s *display) Bootstrap(pipelineCtx Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		d := s.outputFactory(ctx, s.taskName, s.short)

		ctx.Display.WriteStats = d.WriteStats
		ctx.Display.WritePullProgress = d.WritePullProgress
		ctx.Display.Events = d.Events()
		ctx.Display.Interrupt = d.Interrupt

		if ctx.Display.Stdout != io.Discard {
			ctx.Display.Stdout = d.Stdout()
		}

		if ctx.Display.Stderr != io.Discard {
			ctx.Display.Stderr = d.Stderr()
		}

		ctx.Display.Demuxer = xio.NewDemuxer().WithSink(xio.StreamOutput, ctx.Display.Stderr)

		ctx, err := next(ctx)
		if err := d.Close(ctx, err); err != nil {
			return ctx, err
		}

		return ctx, err
	}, nil
}
