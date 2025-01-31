package processor

import (
	"context"
	"fmt"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func filterSteps(refs []string, pipeline Pipeline) ([]Step, error) {
	var steps []Step
	for _, v := range refs {
		step, err := pipeline.Step(v)
		if err != nil {
			return nil, err
		}

		steps = append(steps, step)
	}

	return steps, nil
}

func refSlice(steps []v1beta1.StepReference) []string {
	var refs []string
	for _, ref := range steps {
		refs = append(refs, ref.Name)
	}

	return refs
}

func Chain(pipeline Pipeline, s ...Bootstraper) (Next, error) {
	if len(s) == 0 {
		return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
			return stepContext, nil
		}, nil
	}

	next, err := Chain(pipeline, s[1:len(s)]...)
	if err != nil {
		return nil, err
	}

	return s[0].Bootstrap(pipeline, next)
}

func PrefixName(name, prefix string) string {
	if prefix == "" {
		return name
	}

	return fmt.Sprintf("%s-%s", prefix, name)
}
