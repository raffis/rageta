package processor

import "github.com/raffis/rageta/pkg/apis/core/v1beta1"

func WithGroup(groupBy []string) ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if len(groupBy) == 0 {
			return nil
		}

		for _, key := range groupBy {
			for _, label := range spec.Labels {
				if label.Name == key {
					return &Group{}
				}
			}
		}

		return nil
	}
}

type Group struct{}

func (s *Group) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		ctx.Display.Grouped = true
		return next(ctx)
	}, nil
}
