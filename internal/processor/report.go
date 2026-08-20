package processor

import (
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type Reporter interface {
	Report(ctx TaskContext, name string) error
}

func WithReport(report Reporter) ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if report == nil {
			return nil
		}

		return &Report{
			taskName: spec.Name,
			report:   report,
		}
	}
}

type Report struct {
	taskName string
	report   Reporter
}

func (s *Report) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		ctx, err := next(ctx)
		if reportErr := s.report.Report(ctx, s.taskName); reportErr != nil {
			if err == nil {
				err = reportErr
			}
		}
		return ctx, err
	}, nil
}
