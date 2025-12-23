package processor

import (
	"context"
	"time"

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
	return func(ctx StepContext) (StepContext, error) {
		ctx, err := next(ctx)

		if containerStatus, ok := ctx.Containers[s.stepName]; ok {
			s.teardown <- func(ctx context.Context, timeout time.Duration) error {
				return s.driver.DeletePod(ctx, &runtime.Pod{
					Name: containerStatus.ContainerID,
					Status: runtime.PodStatus{
						Containers: []runtime.ContainerStatus{containerStatus},
					},
				}, timeout)
			}
		}

		return ctx, err
	}, nil
}
