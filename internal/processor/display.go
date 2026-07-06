package processor

import (
	"io"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type Display interface {
	Stdout() io.Writer
	Stderr() io.Writer
	Close(ctx TaskContext, err error) error
	WriteStats(cpu, mem, netRx, netTx int64) error
	WriteProgress(current, total int64) error
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
	Stdout        io.Writer
	Stderr        io.Writer
	WriteStats    func(cpu, mem, netRx, netTx int64) error
	WriteProgress func(current, total int64) error
	Grouped       bool
}

func (s *displayBootstraper) Bootstrap(pipelineCtx Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		if ctx.Display.Grouped {
			return next(ctx)
		}

		d := s.outputFactory(ctx, s.stepName, s.short)

		ctx.Display.WriteStats = d.WriteStats
		ctx.Display.WriteProgress = d.WriteProgress

		if ctx.Display.Stdout != io.Discard {
			ctx.Display.Stdout = d.Stdout()
		}

		if ctx.Display.Stderr != io.Discard {
			ctx.Display.Stderr = d.Stderr()
		}

		ctx, err := next(ctx)
		if err := d.Close(ctx, err); err != nil {
			return ctx, err
		}

		return ctx, err
	}, nil
}
