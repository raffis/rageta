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
		if spec.Steps == nil || spec.Service != nil {
			return nil
		}
		return &Stats{}
	}
}

type Stats struct{}

func (s *Stats) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		var (
			cpuSum, memSum       int64
			count                int
			lastNetRx, lastNetTx int64
		)

		ctx.Display.Stdout = xio.NewStatsFilterWriter(ctx.Display.Stdout, func(cpu, mem, netRx, netTx int64) error {
			cpuSum += cpu
			memSum += mem
			count++

			if count == 3 {
				rxBps := max(netRx-lastNetRx, 0) / 3
				txBps := max(netTx-lastNetTx, 0) / 3

				ctx.Display.WriteStats(cpuSum/3, memSum/3, rxBps, txBps)

				cpuSum, memSum, count = 0, 0, 0
				lastNetRx, lastNetTx = netRx, netTx
			}

			return nil
		})

		return next(ctx)
	}, nil
}
