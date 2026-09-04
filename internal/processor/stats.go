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
			last.MemBytes += sample.MemBytes
			last.NetRxBytes += sample.NetRxBytes
			last.NetTxBytes += sample.NetTxBytes
			last.DiskReadBytes += sample.DiskReadBytes
			last.DiskWriteBytes += sample.DiskWriteBytes
			count++

			if count == 3 {
				rateSample := stats.Sample{
					CPUMillicores:  last.CPUMillicores / 3,
					MemBytes:       last.MemBytes / 3,
					NetRxBytes:     last.NetRxBytes / 3,
					NetTxBytes:     last.NetTxBytes / 3,
					DiskReadBytes:  last.DiskReadBytes / 3,
					DiskWriteBytes: last.DiskWriteBytes / 3,
				}

				ctx.Display.WriteStats(&rateSample)
				last = stats.Sample{}
				count = 0
			}

			return len(payload), nil
		}))

		return next(ctx)
	}, nil
}
