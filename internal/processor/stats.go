package processor

import (
	"sync"
	"time"

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
			mu    sync.Mutex
		)

		setZero := func() {
			mu.Lock()
			defer mu.Unlock()
			last = stats.Sample{}
			ctx.Display.WriteStats(&last)
			count = 0
		}

		// If for any reason the stream is interrupted or no stats are received after 4 seconds
		// the stats need to be reset
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		defer setZero()

		go func() {
			for {
				select {
				case <-ticker.C:
					setZero()
				case <-ctx.Context.Done():
					return
				}
			}
		}()

		ctx.Display.Demuxer.WithSink(xio.StreamStats, xio.WriterFunc(func(payload []byte) (int, error) {
			mu.Lock()
			defer mu.Unlock()

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
				ticker.Reset(4 * time.Second)

			}

			return len(payload), nil
		}))

		return next(ctx)

	}, nil
}
