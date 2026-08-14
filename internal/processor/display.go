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
	Close(ctx TaskContext, err error) error
	WriteStats(sample *stats.Sample) error
	WritePullProgress(current, total int64) error
}

type DisplayFactory func(ctx TaskContext, stepName, short string) Display

func WithDisplay(outputFactory DisplayFactory) ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		stdio := &displayBootstraper{
			stepName:      spec.Name,
			short:         spec.Short,
			spec:          spec,
			outputFactory: outputFactory,
		}

		return stdio
	}
}

type displayBootstraper struct {
	stepName      string
	short         string
	spec          *v1beta1.Task
	outputFactory DisplayFactory
	decouple      bool
}

type DisplayContext struct {
	Stdout            io.Writer
	Stderr            io.Writer
	Demuxer           *xio.Demuxer
	Events            io.Writer
	WriteStats        func(sample *stats.Sample) error `json:"-"`
	WritePullProgress func(current, total int64) error `json:"-"`
	Grouped           bool
}

func (s *displayBootstraper) Bootstrap(pipelineCtx Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		if ctx.Display.Grouped {
			return next(ctx)
		}

		d := s.outputFactory(ctx, s.stepName, s.short)

		ctx.Display.WriteStats = d.WriteStats
		ctx.Display.WritePullProgress = d.WritePullProgress
		ctx.Display.Events = d.Events()

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
