package processor

import (
	"fmt"
	"time"

	"github.com/raffis/rageta/internal/styles"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithMonitor(progress bool) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if !progress {
			return nil
		}

		return &Monitor{
			stepName: spec.Name,
		}
	}
}

type Monitor struct {
	stepName string
}

func (s *Monitor) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		ticker := time.NewTicker(5 * time.Second)
		quit := make(chan struct{})
		defer func() {
			quit <- struct{}{}
		}()

		progress := func() {
			if ctx.Stderr != nil {
				ctx.Stderr.Write([]byte(styles.Highlight.Render(fmt.Sprintf("Waiting for %q to finish\n", s.stepName))))
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
		ctx, err := next(ctx)
		if err != nil {
			ctx.Stderr.Write([]byte(styles.Highlight.Render(fmt.Sprintf("Error occurred: %q\n", err.Error()))))
		}

		return ctx, err
	}, nil
}
