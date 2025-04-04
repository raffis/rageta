package processor

import (
	"context"
	"fmt"
	"time"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithProgress() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		return &Progress{
			stepName: spec.Name,
		}
	}
}

type Progress struct {
	stepName string
}

func (s *Progress) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {

		ticker := time.NewTicker(5 * time.Second)
		quit := make(chan struct{})
		defer func() {
			quit <- struct{}{}
		}()

		go func() {
			for {
				select {
				case <-ticker.C:
					if stepContext.Stderr != nil {
						stepContext.Stderr.Write([]byte(fmt.Sprintf("Waiting for %s to finish\n", s.stepName)))
					}
				case <-quit:
					ticker.Stop()
					return
				}
			}
		}()

		return next(ctx, stepContext)
	}, nil
}
