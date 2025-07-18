package processor

import (
	"os"
	"testing"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvVarsBuilder(t *testing.T) {
	tests := []struct {
		name       string
		spec       *v1beta1.Step
		osEnv      map[string]string
		defaultEnv map[string]string
		expected   map[string]string
	}{
		{
			name: "empty env vars",
			spec: &v1beta1.Step{},
			osEnv: map[string]string{
				"OS_VAR": "os_value",
			},
			defaultEnv: map[string]string{
				"DEFAULT_VAR": "default_value",
			},
			expected: map[string]string{
				"DEFAULT_VAR": "default_value",
			},
		},
		{
			name: "spec env vars with values",
			spec: &v1beta1.Step{
				StepOptions: v1beta1.StepOptions{
					Env: []v1beta1.EnvVar{
						{Name: "SPEC_VAR1", Value: stringPtr("spec_value1")},
						{Name: "SPEC_VAR2", Value: stringPtr("spec_value2")},
					},
				},
			},
			osEnv:      map[string]string{},
			defaultEnv: map[string]string{},
			expected: map[string]string{
				"SPEC_VAR1": "spec_value1",
				"SPEC_VAR2": "spec_value2",
			},
		},
		{
			name: "spec env vars with OS fallback",
			spec: &v1beta1.Step{
				StepOptions: v1beta1.StepOptions{
					Env: []v1beta1.EnvVar{
						{Name: "OS_VAR", Value: nil}, // Will use OS value
						{Name: "SPEC_VAR", Value: stringPtr("spec_value")},
					},
				},
			},
			osEnv: map[string]string{
				"OS_VAR": "os_value",
			},
			defaultEnv: map[string]string{},
			expected: map[string]string{
				"OS_VAR":   "os_value",
				"SPEC_VAR": "spec_value",
			},
		},
		{
			name: "default env vars override",
			spec: &v1beta1.Step{
				StepOptions: v1beta1.StepOptions{
					Env: []v1beta1.EnvVar{
						{Name: "VAR1", Value: stringPtr("spec_value")},
					},
				},
			},
			osEnv: map[string]string{},
			defaultEnv: map[string]string{
				"VAR1": "default_value",
				"VAR2": "default_value2",
			},
			expected: map[string]string{
				"VAR1": "default_value",
				"VAR2": "default_value2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := WithEnvVars(tt.osEnv, tt.defaultEnv)
			bootstraper := builder(tt.spec)

			assert.NotNil(t, bootstraper)
			envVars, ok := bootstraper.(*EnvVars)
			assert.True(t, ok)
			assert.Equal(t, tt.expected, envVars.env)
		})
	}
}

func TestEnvVarsBootstrap(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		inputCtx    StepContext
		nextError   error
		expectError bool
	}{
		{
			name: "basic env vars setup",
			env: map[string]string{
				"TEST_VAR":    "test_value",
				"ANOTHER_VAR": "another_value",
			},
			inputCtx: StepContext{
				Dir:  "/tmp",
				Envs: map[string]string{},
			},
			nextError:   nil,
			expectError: false,
		},
		{
			name: "env vars with existing context envs",
			env: map[string]string{
				"NEW_VAR": "new_value",
			},
			inputCtx: StepContext{
				Dir: "/tmp",
				Envs: map[string]string{
					"EXISTING_VAR": "existing_value",
				},
			},
			nextError:   nil,
			expectError: false,
		},
		{
			name: "next function error propagation",
			env: map[string]string{
				"TEST_VAR": "test_value",
			},
			inputCtx: StepContext{
				Dir:  "/tmp",
				Envs: map[string]string{},
			},
			nextError:   assert.AnError,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envVars := &EnvVars{env: tt.env}
			pipeline := &mockPipeline{}
			nextCalled := false

			next := func(ctx StepContext) (StepContext, error) {
				nextCalled = true
				// Verify that env vars are set during execution
				for k, v := range tt.env {
					assert.Equal(t, v, ctx.Envs[k], "Env var %s should be set", k)
				}
				return ctx, tt.nextError
			}

			nextFunc, err := envVars.Bootstrap(pipeline, next)
			require.NoError(t, err)
			require.NotNil(t, nextFunc)

			resultCtx, resultErr := nextFunc(tt.inputCtx)

			assert.True(t, nextCalled)

			if tt.expectError {
				assert.Error(t, resultErr)
				assert.Equal(t, tt.nextError, resultErr)
			} else {
				assert.NoError(t, resultErr)
				// The processor should return a valid context
				assert.NotNil(t, resultCtx)
			}
		})
	}
}

func TestEnvVarsFileCreation(t *testing.T) {
	envVars := &EnvVars{
		env: map[string]string{
			"TEST_VAR1": "test_value1",
			"TEST_VAR2": "test_value2",
		},
	}
	pipeline := &mockPipeline{}
	nextCalled := false

	next := func(ctx StepContext) (StepContext, error) {
		nextCalled = true
		// Verify that env file is created
		assert.NotEmpty(t, ctx.Env, "Env file path should be set")
		_, err := os.Stat(ctx.Env)
		assert.NoError(t, err, "Env file should exist")
		return ctx, nil
	}

	nextFunc, err := envVars.Bootstrap(pipeline, next)
	require.NoError(t, err)
	require.NotNil(t, nextFunc)

	inputCtx := StepContext{
		Dir:  "/tmp",
		Envs: map[string]string{},
	}
	resultCtx, resultErr := nextFunc(inputCtx)

	assert.True(t, nextCalled)
	assert.NoError(t, resultErr)
	assert.NotEmpty(t, resultCtx.Env, "Result context should have env file path")
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}
