package processor

import (
	"context"

	"github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithGarbageCollector(noGC bool, driver runtime.Interface, teardown chan Teardown) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if noGC {
			return nil
		}

		return &GarbageCollector{
			stepName: spec.Name,
			driver:   driver,
			teardown: teardown,
		}
	}
}

type GarbageCollector struct {
	stepName string
	driver   runtime.Interface
	teardown chan Teardown
}

func (s *GarbageCollector) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		stepContext, err := next(ctx, stepContext)
		if containerStatus, ok := stepContext.Containers[s.stepName]; ok {
			s.teardown <- func(ctx context.Context) error {
				return s.driver.DeletePod(ctx, &runtime.Pod{
					Status: runtime.PodStatus{
						Containers: []runtime.ContainerStatus{containerStatus},
					},
				})
			}
		}

		return stepContext, err
	}, nil
}
