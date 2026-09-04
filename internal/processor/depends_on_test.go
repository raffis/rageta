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
		spec      *v1beta1.Task
		expectNil bool
	}{
		{
			name:      "no needs returns nil",
			spec:      &v1beta1.Task{},
			expectNil: true,
		},
		{
			name: "needs present returns Needs struct",
			spec: &v1beta1.Task{
				TaskOptions: v1beta1.TaskOptions{
					Needs: []v1beta1.TaskReference{
						{Name: "step1"},
						{Name: "step2"},
					},
				},
			},
			expectNil: false,
		},
		{
			name: "empty needs slice returns nil",
			spec: &v1beta1.Task{
				TaskOptions: v1beta1.TaskOptions{
					Needs: []v1beta1.TaskReference{},
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
		existingTasks   map[string]*TaskContext
		stepError       error
		entrypointError error
		expectError     bool
		expectedCalls   int
	}{
		{
			name:      "no needs to execute",
			needsRefs: []string{},
			existingTasks: map[string]*TaskContext{
				"step1": {},
				"step2": {},
			},
			expectError:   false,
			expectedCalls: 1, // Only the main next function
		},
		{
			name:      "all needed steps already executed",
			needsRefs: []string{"step1", "step2"},
			existingTasks: map[string]*TaskContext{
				"step1": {},
				"step2": {},
			},
			expectError:   false,
			expectedCalls: 1, // Only the main next function
		},
		{
			name:      "some needed steps not executed",
			needsRefs: []string{"step1", "step2"},
			existingTasks: map[string]*TaskContext{
				"step1": {},
			},
			expectError:   false,
			expectedCalls: 1, // Only the main next function since step2 execution fails
		},
		{
			name:          "step not found error",
			needsRefs:     []string{"nonexistent"},
			existingTasks: map[string]*TaskContext{},
			stepError:     errors.New("step not found"),
			expectError:   true,
			expectedCalls: 0, // Should fail before calling next
		},
		{
			name:            "entrypoint error",
			needsRefs:       []string{"step1"},
			existingTasks:   map[string]*TaskContext{},
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
			pipeline := &mockPipelineWithTasks{
				steps: make(map[string]*mockTask),
			}

			// Set up mock steps for the needs
			for _, ref := range tt.needsRefs {
				pipeline.steps[ref] = &mockTask{
					entrypointError: tt.entrypointError,
				}
			}

			// Mock the Task method to return errors when needed
			if tt.stepError != nil {
				pipeline.stepError = tt.stepError
			}

			next := func(ctx TaskContext) (TaskContext, error) {
				callCount++
				return ctx, nil
			}

			nextFunc, err := needs.Bootstrap(pipeline, next)
			require.NoError(t, err)
			require.NotNil(t, nextFunc)

		inputCtx := TaskContext{
			Tasks:      tt.existingTasks,
			ContextDir: "/tmp",
			Template:   TemplateContext{Template: &v1beta1.Template{}},
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
	pipeline := &mockPipelineWithTasks{
		steps: map[string]*mockTask{
			"step1": {
				entrypointError: nil,
				outputCtx: TaskContext{
					EnvVars: EnvVarsContext{Envs: map[string]string{
						"STEP1_VAR": "step1_value",
					}},
					SecretVars: SecretVarsContext{Secrets: map[string]string{
						"STEP1_SECRET": "step1_secret",
					}},
				},
			},
		},
	}

	next := func(ctx TaskContext) (TaskContext, error) {
		return ctx, nil
	}

	nextFunc, err := needs.Bootstrap(pipeline, next)
	require.NoError(t, err)
	require.NotNil(t, nextFunc)

	inputCtx := TaskContext{
		Tasks:      map[string]*TaskContext{},
		ContextDir: "/tmp",
		Template:   TemplateContext{Template: &v1beta1.Template{}},
		EnvVars: EnvVarsContext{Envs: map[string]string{
			"INPUT_VAR": "input_value",
		}},
		SecretVars: SecretVarsContext{Secrets: map[string]string{
			"INPUT_SECRET": "input_secret",
		}},
	}

	resultCtx, resultErr := nextFunc(inputCtx)

	assert.NoError(t, resultErr)
	// Verify that contexts were merged
	assert.Equal(t, "input_value", resultCtx.EnvVars.Envs["INPUT_VAR"])
	assert.Equal(t, "step1_value", resultCtx.EnvVars.Envs["STEP1_VAR"])
	assert.Equal(t, "input_secret", resultCtx.SecretVars.Secrets["INPUT_SECRET"])
	assert.Equal(t, "step1_secret", resultCtx.SecretVars.Secrets["STEP1_SECRET"])
}

// Mock pipeline with steps for testing
type mockPipelineWithTasks struct {
	steps     map[string]*mockTask
	stepError error
}

func (m *mockPipelineWithTasks) Task(name string) (Task, error) {
	if m.stepError != nil {
		return nil, m.stepError
	}
	if step, exists := m.steps[name]; exists {
		return step, nil
	}
	return nil, errors.New("step not found")
}

func (m *mockPipelineWithTasks) Entrypoint(name string) (Next, error) {
	return nil, nil
}

func (m *mockPipelineWithTasks) EntrypointName() (string, error) {
	return "", nil
}

func (m *mockPipelineWithTasks) Name() string {
	return "mock"
}

func (m *mockPipelineWithTasks) ID() string {
	return "mock-id"
}

// Mock step for testing
type mockTask struct {
	entrypointError error
	outputCtx       TaskContext
}

func (m *mockTask) Processors() []Bootstraper {
	return nil
}

func (m *mockTask) Entrypoint() (Next, error) {
	if m.entrypointError != nil {
		return nil, m.entrypointError
	}
	return func(ctx TaskContext) (TaskContext, error) {
		// Ensure the output context has a template to avoid nil pointer
		if m.outputCtx.Template.Template == nil {
			m.outputCtx.Template.Template = &v1beta1.Template{}
		}
		return m.outputCtx, nil
	}, nil
}
