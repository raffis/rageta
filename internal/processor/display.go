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
}

type DisplayFactory func(ctx TaskContext, stepName, short string) Display

func WithDisplay(outputFactory DisplayFactory, withInternals, decouple bool) ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		/*internalTask := spec.Run == nil && spec.Inherit == nil

		if !withInternals && internalTask {
			return nil
		}*/

		stdio := &displayBootstraper{
			stepName:      spec.Name,
			short:         spec.Short,
			spec:          spec,
			outputFactory: outputFactory,
			decouple:      decouple,
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
	Stdout     io.Writer
	Stderr     io.Writer
	WriteStats func(cpu, mem, netRx, netTx int64) error
}

func (s *displayBootstraper) Bootstrap(pipelineCtx Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		if ctx.Labels.Has("pipeline") && !s.decouple {
			return next(ctx)
		}

		d := s.outputFactory(ctx, s.stepName, s.short)

		ctx.Display.WriteStats = d.WriteStats

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
