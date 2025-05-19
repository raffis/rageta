package processor

import (
	"fmt"
	"time"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithProgress(progress bool) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if !progress {
			return nil
		}

		return &Progress{
			stepName: spec.Name,
		}
	}
}

type Progress struct {
	stepName string
}

func (s *Progress) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		ticker := time.NewTicker(5 * time.Second)
		quit := make(chan struct{})
		defer func() {
			quit <- struct{}{}
		}()

		progress := func() {
			if ctx.Stderr != nil {
				ctx.Stderr.Write(fmt.Appendf(nil, "Waiting for %s to finish\n", s.stepName))
			}
		}

		go func() {
			for {
				select {
				case <-ticker.C:
					progress()
				case <-quit:
					ticker.Stop()
					return
				}
			}
		}()

		progress()
		return next(ctx)
	}, nil
}
