package processor

import (
	"fmt"
	"io"

	"github.com/joho/godotenv"
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
		return func(ctx StepContext) (StepContext, error) {
			return ctx, nil
		}, nil
	}

	next, err := Chain(pipeline, s[1:]...)
	if err != nil {
		return nil, err
	}

	return s[0].Bootstrap(pipeline, next)
}

func SuffixName(name, suffix string) string {
	if suffix == "" {
		return name
	}

	return fmt.Sprintf("%s-%s", name, suffix)
}

func parseVars(f io.Reader) (map[string]string, error) {
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	envMap, err := godotenv.UnmarshalBytes(b)
	if err != nil {
		return nil, fmt.Errorf("dotenv failed: %w", err)
	}

	return envMap, err
}
