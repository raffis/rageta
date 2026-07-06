package processor

import (
	"github.com/raffis/rageta/internal/xio"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type StatsContext struct {
}

func newStatsContext() StatsContext {
	return StatsContext{}
}

func WithStats() ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if spec.Steps == nil {
			return nil
		}
		return &Stats{}
	}
}

type Stats struct{}

func (s *Stats) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		var lastNetRx, lastNetTx int64

		ctx.Display.Stdout = xio.NewStatsFilterWriter(ctx.Display.Stdout, func(cpu, mem, netRx, netTx int64) error {
			rxBps := max(netRx-lastNetRx, 0)
			txBps := max(netTx-lastNetTx, 0)
			ctx.Display.WriteStats(cpu, mem, rxBps, txBps)

			lastNetRx = netRx
			lastNetTx = netTx

			return nil
		})

		return next(ctx)
	}, nil
}
