package processor

import (
	"github.com/raffis/rageta/internal/stats"
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
		if spec.Steps == nil && spec.Service == nil {
			return nil
		}
		return &Stats{}
	}
}

type Stats struct{}

func (s *Stats) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		var (
			last  stats.Sample
			count int
		)

		ctx.Display.Demuxer.WithSink(xio.StreamStats, xio.WriterFunc(func(payload []byte) (int, error) {
			var sample stats.Sample
			if err := sample.Unmarshal(payload); err != nil {
				return 0, err
			}

			last.CPUMillicores += sample.CPUMillicores
			last.MemBytes += last.MemBytes
			count++

			if count == 3 {
				rateSample := stats.Sample{
					CPUMillicores: last.CPUMillicores / 3,
					MemBytes:      last.MemBytes / 3,
					NetRxBytes:    max(sample.NetRxBytes-last.NetRxBytes, 0) / 3,
					NetTxBytes:    max(sample.NetTxBytes-last.NetTxBytes, 0) / 3,
				}

				ctx.Display.WriteStats(&rateSample)
				last = stats.Sample{}
			}

			return len(payload), nil
		}))

		return next(ctx)
	}, nil
}
