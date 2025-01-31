package processor

import (
	"context"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type ResultStore interface {
	Add(stepName string, result *StepResult)
}

func WithReport(store ResultStore, uniqueName string) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if store == nil {
			return nil
		}

		//store.Add(uniqueName, nil)

		return &Report{
			stepName: spec.Name,
			store:    store,
		}
	}
}

type Report struct {
	stepName string
	store    ResultStore
}

func (s *Report) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		stepContext, err := next(ctx, stepContext)
		s.store.Add(PrefixName(s.stepName, stepContext.NamePrefix), stepContext.Steps[s.stepName])
		return stepContext, err
	}, nil
}
