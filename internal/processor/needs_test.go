package processor

import (
	"errors"
	"testing"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNeedsBuilder(t *testing.T) {
	tests := []struct {
		name      string
		spec      *v1beta1.Step
		expectNil bool
	}{
		{
			name:      "no needs returns nil",
			spec:      &v1beta1.Step{},
			expectNil: true,
		},
		{
			name: "needs present returns Needs struct",
			spec: &v1beta1.Step{
				StepOptions: v1beta1.StepOptions{
					Needs: []v1beta1.StepReference{
						{Name: "step1"},
						{Name: "step2"},
					},
				},
			},
			expectNil: false,
		},
		{
			name: "empty needs slice returns nil",
			spec: &v1beta1.Step{
				StepOptions: v1beta1.StepOptions{
					Needs: []v1beta1.StepReference{},
				},
			},
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := WithNeeds()
			bootstraper := builder(tt.spec)

			if tt.expectNil {
				assert.Nil(t, bootstraper)
			} else {
				assert.NotNil(t, bootstraper)
				needs, ok := bootstraper.(*Needs)
				assert.True(t, ok)

				expectedRefs := make([]string, len(tt.spec.Needs))
				for i, ref := range tt.spec.Needs {
					expectedRefs[i] = ref.Name
				}
				assert.Equal(t, expectedRefs, needs.refs)
			}
		})
	}
}

func TestNeedsBootstrap(t *testing.T) {
	tests := []struct {
		name            string
		needsRefs       []string
		existingSteps   map[string]*StepContext
		stepError       error
		entrypointError error
		expectError     bool
		expectedCalls   int
	}{
		{
			name:      "no needs to execute",
			needsRefs: []string{},
			existingSteps: map[string]*StepContext{
				"step1": {},
				"step2": {},
			},
			expectError:   false,
			expectedCalls: 1, // Only the main next function
		},
		{
			name:      "all needed steps already executed",
			needsRefs: []string{"step1", "step2"},
			existingSteps: map[string]*StepContext{
				"step1": {},
				"step2": {},
			},
			expectError:   false,
			expectedCalls: 1, // Only the main next function
		},
		{
			name:      "some needed steps not executed",
			needsRefs: []string{"step1", "step2"},
			existingSteps: map[string]*StepContext{
				"step1": {},
			},
			expectError:   false,
			expectedCalls: 1, // Only the main next function since step2 execution fails
		},
		{
			name:          "step not found error",
			needsRefs:     []string{"nonexistent"},
			existingSteps: map[string]*StepContext{},
			stepError:     errors.New("step not found"),
			expectError:   true,
			expectedCalls: 0, // Should fail before calling next
		},
		{
			name:            "entrypoint error",
			needsRefs:       []string{"step1"},
			existingSteps:   map[string]*StepContext{},
			entrypointError: errors.New("entrypoint error"),
			expectError:     true,
			expectedCalls:   0, // Should fail before calling next
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			needs := &Needs{refs: tt.needsRefs}
			callCount := 0

			// Create a mock pipeline that returns mock steps
			pipeline := &mockPipelineWithSteps{
				steps: make(map[string]*mockStep),
			}

			// Set up mock steps for the needs
			for _, ref := range tt.needsRefs {
				pipeline.steps[ref] = &mockStep{
					entrypointError: tt.entrypointError,
				}
			}

			// Mock the Step method to return errors when needed
			if tt.stepError != nil {
				pipeline.stepError = tt.stepError
			}

			next := func(ctx StepContext) (StepContext, error) {
				callCount++
				return ctx, nil
			}

			nextFunc, err := needs.Bootstrap(pipeline, next)
			require.NoError(t, err)
			require.NotNil(t, nextFunc)

			inputCtx := StepContext{
				Steps:    tt.existingSteps,
				Dir:      "/tmp",
				Template: &v1beta1.Template{},
			}
			resultCtx, resultErr := nextFunc(inputCtx)

			if tt.expectError {
				assert.Error(t, resultErr)
				if tt.stepError != nil {
					assert.ErrorIs(t, resultErr, tt.stepError)
				} else if tt.entrypointError != nil {
					assert.ErrorIs(t, resultErr, tt.entrypointError)
				}
			} else {
				assert.NoError(t, resultErr)
				assert.Equal(t, inputCtx, resultCtx)
			}

			assert.Equal(t, tt.expectedCalls, callCount)
		})
	}
}

func TestNeedsContextMerging(t *testing.T) {
	needs := &Needs{refs: []string{"step1"}}
	pipeline := &mockPipelineWithSteps{
		steps: map[string]*mockStep{
			"step1": {
				entrypointError: nil,
				outputCtx: StepContext{
					Envs: map[string]string{
						"STEP1_VAR": "step1_value",
					},
					Secrets: map[string]string{
						"STEP1_SECRET": "step1_secret",
					},
				},
			},
		},
	}

	next := func(ctx StepContext) (StepContext, error) {
		return ctx, nil
	}

	nextFunc, err := needs.Bootstrap(pipeline, next)
	require.NoError(t, err)
	require.NotNil(t, nextFunc)

	inputCtx := StepContext{
		Steps:    map[string]*StepContext{},
		Dir:      "/tmp",
		Template: &v1beta1.Template{},
		Envs: map[string]string{
			"INPUT_VAR": "input_value",
		},
		Secrets: map[string]string{
			"INPUT_SECRET": "input_secret",
		},
	}

	resultCtx, resultErr := nextFunc(inputCtx)

	assert.NoError(t, resultErr)
	// Verify that contexts were merged
	assert.Equal(t, "input_value", resultCtx.Envs["INPUT_VAR"])
	assert.Equal(t, "step1_value", resultCtx.Envs["STEP1_VAR"])
	assert.Equal(t, "input_secret", resultCtx.Secrets["INPUT_SECRET"])
	assert.Equal(t, "step1_secret", resultCtx.Secrets["STEP1_SECRET"])
}

// Mock pipeline with steps for testing
type mockPipelineWithSteps struct {
	steps     map[string]*mockStep
	stepError error
}

func (m *mockPipelineWithSteps) Step(name string) (Step, error) {
	if m.stepError != nil {
		return nil, m.stepError
	}
	if step, exists := m.steps[name]; exists {
		return step, nil
	}
	return nil, errors.New("step not found")
}

func (m *mockPipelineWithSteps) Entrypoint(name string) (Next, error) {
	return nil, nil
}

func (m *mockPipelineWithSteps) EntrypointName() (string, error) {
	return "", nil
}

func (m *mockPipelineWithSteps) Name() string {
	return "mock"
}

func (m *mockPipelineWithSteps) ID() string {
	return "mock-id"
}

// Mock step for testing
type mockStep struct {
	entrypointError error
	outputCtx       StepContext
}

func (m *mockStep) Processors() []Bootstraper {
	return nil
}

func (m *mockStep) Entrypoint() (Next, error) {
	if m.entrypointError != nil {
		return nil, m.entrypointError
	}
	return func(ctx StepContext) (StepContext, error) {
		// Ensure the output context has a template to avoid nil pointer
		if m.outputCtx.Template == nil {
			m.outputCtx.Template = &v1beta1.Template{}
		}
		return m.outputCtx, nil
	}, nil
}
