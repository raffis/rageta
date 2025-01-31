package processor

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithEventEmitter() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		return &EventEmitter{
			stepName: spec.Name,
		}
	}
}

type EventEmitter struct {
	stepName string
}

func (s *EventEmitter) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		start := time.Now()

		var outputTmp, envTmp *os.File
		outputTmp, err := os.CreateTemp(stepContext.TmpDir(), "output")
		if err != nil {
			return stepContext, err
		}

		defer outputTmp.Close()
		defer os.Remove(outputTmp.Name())

		envTmp, err = os.CreateTemp(stepContext.TmpDir(), "env")
		if err != nil {
			return stepContext, err
		}

		defer envTmp.Close()
		defer os.Remove(envTmp.Name())

		stepContext.Output = outputTmp.Name()
		stepContext.Env = envTmp.Name()
		stepContext.Steps[s.stepName] = &StepResult{
			StartedAt: start,
		}

		stepContext, nextErr := next(ctx, stepContext)
		endedAt := time.Now()
		outputTmp.Sync()
		envTmp.Sync()

		outputs, err := parseVars(outputTmp)
		if err != nil {
			return stepContext, err
		}

		envs, err := parseVars(envTmp)
		if err != nil {
			return stepContext, err
		}

		stepContext.Steps[s.stepName].Outputs = outputs
		stepContext.Steps[s.stepName].Envs = envs
		stepContext.Steps[s.stepName].EndedAt = endedAt
		stepContext.Steps[s.stepName].Error = nextErr

		if nextErr != nil {
			nextErr = fmt.Errorf("step %s failed: %w", s.stepName, nextErr)
		}

		return stepContext, nextErr

	}, nil
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
