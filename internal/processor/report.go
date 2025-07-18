package processor

import (
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type Reporter interface {
	Report(ctx StepContext, name string) error
}

func WithReport(report Reporter) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if report == nil {
			return nil
		}

		return &Report{
			stepName: spec.Name,
			report:   report,
		}
	}
}

type Report struct {
	stepName string
	report   Reporter
}

func (s *Report) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		ctx, err := next(ctx)
		if reportErr := s.report.Report(ctx, SuffixName(s.stepName, ctx.NamePrefix)); reportErr != nil {
			if err == nil {
				err = reportErr
			}
		}
		return ctx, err
	}, nil
}
