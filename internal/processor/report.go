package processor

import (
	"context"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type Reporter interface {
	Report(ctx context.Context, name string, stepContext StepContext) error
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
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		stepContext, err := next(ctx, stepContext)
		s.report.Report(ctx, suffixName(s.stepName, stepContext.NamePrefix), stepContext)
		return stepContext, err
	}, nil
}
