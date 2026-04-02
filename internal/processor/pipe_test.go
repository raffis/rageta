package processor

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipeForwardsStdoutWhenMiddleStepIsConditionFalse(t *testing.T) {
	var receivedByThirdStep string

	pipeline := &mockPipelineWithPipeSteps{
		steps: map[string]Step{
			"s1": &pipeTestStep{
				fn: func(ctx StepContext) (StepContext, error) {
					for _, w := range ctx.Streams.AdditionalStdout {
						_, err := io.WriteString(w, "from-first-step")
						require.NoError(t, err)
						if closer, ok := w.(io.Closer); ok {
							require.NoError(t, closer.Close())
						}
					}
					return ctx, nil
				},
			},
			"s2": &pipeTestStep{
				fn: func(ctx StepContext) (StepContext, error) {
					return ctx, ErrConditionFalse
				},
			},
			"s3": &pipeTestStep{
				fn: func(ctx StepContext) (StepContext, error) {
					buf := make([]byte, len("from-first-step"))
					_, err := io.ReadFull(ctx.Streams.Stdin, buf)
					require.NoError(t, err)
					receivedByThirdStep = string(buf)
					return ctx, nil
				},
			},
		},
	}

	p := &Pipe{
		refs: []string{"s1", "s2", "s3"},
	}

	next, err := p.Bootstrap(pipeline, func(ctx StepContext) (StepContext, error) { return ctx, nil })
	require.NoError(t, err)

	ctx := NewContext()
	ctx.Context = context.Background()
	_, err = next(ctx)
	require.NoError(t, err)
	require.Equal(t, "from-first-step", strings.TrimSpace(receivedByThirdStep))
}

type pipeTestStep struct {
	fn func(ctx StepContext) (StepContext, error)
}

func (m *pipeTestStep) Processors() []Bootstraper {
	return nil
}

func (m *pipeTestStep) Entrypoint() (Next, error) {
	return m.fn, nil
}

type mockPipelineWithPipeSteps struct {
	steps map[string]Step
}

func (m *mockPipelineWithPipeSteps) Step(name string) (Step, error) {
	return m.steps[name], nil
}

func (m *mockPipelineWithPipeSteps) Entrypoint(name string) (Next, error) {
	return nil, nil
}

func (m *mockPipelineWithPipeSteps) EntrypointName() (string, error) {
	return "", nil
}

func (m *mockPipelineWithPipeSteps) Name() string {
	return "mock"
}

func (m *mockPipelineWithPipeSteps) ID() string {
	return "mock-id"
}
