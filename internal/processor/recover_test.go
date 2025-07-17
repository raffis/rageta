package processor

import (
	"errors"
	"testing"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecoverBuilder(t *testing.T) {
	tests := []struct {
		name      string
		spec      *v1beta1.Step
		expectNil bool
	}{
		{
			name:      "always returns Recover struct",
			spec:      &v1beta1.Step{Name: "test-step"},
			expectNil: false,
		},
		{
			name:      "empty step name",
			spec:      &v1beta1.Step{Name: ""},
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := WithRecover()
			bootstraper := builder(tt.spec)

			assert.NotNil(t, bootstraper)
			recover, ok := bootstraper.(*Recover)
			assert.True(t, ok)
			if tt.spec != nil {
				assert.Equal(t, tt.spec.Name, recover.stepName)
			}
		})
	}
}

func TestRecoverBootstrap(t *testing.T) {
	tests := []struct {
		name        string
		stepName    string
		nextError   error
		shouldPanic bool
		expectError bool
	}{
		{
			name:        "no error from next function",
			stepName:    "test-step",
			nextError:   nil,
			shouldPanic: false,
			expectError: false,
		},
		{
			name:        "error from next function",
			stepName:    "test-step",
			nextError:   errors.New("test error"),
			shouldPanic: false,
			expectError: true,
		},
		{
			name:        "panic in next function",
			stepName:    "test-step",
			nextError:   nil,
			shouldPanic: true,
			expectError: true,
		},
		{
			name:        "panic with error in next function",
			stepName:    "test-step",
			nextError:   errors.New("test error"),
			shouldPanic: true,
			expectError: true,
		},
		{
			name:        "panic with empty step name",
			stepName:    "",
			nextError:   nil,
			shouldPanic: true,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recover := &Recover{stepName: tt.stepName}
			pipeline := &mockPipeline{}
			nextCalled := false

			next := func(ctx StepContext) (StepContext, error) {
				nextCalled = true
				if tt.shouldPanic {
					panic("test panic")
				}
				return ctx, tt.nextError
			}

			nextFunc, err := recover.Bootstrap(pipeline, next)
			require.NoError(t, err)
			require.NotNil(t, nextFunc)

			inputCtx := StepContext{}
			resultCtx, resultErr := nextFunc(inputCtx)

			assert.True(t, nextCalled)

			if tt.expectError {
				assert.Error(t, resultErr)
				if tt.shouldPanic {
					assert.Contains(t, resultErr.Error(), "panic in step")
					assert.Contains(t, resultErr.Error(), tt.stepName)
					assert.Contains(t, resultErr.Error(), "test panic")
				} else {
					assert.Equal(t, tt.nextError, resultErr)
				}
			} else {
				assert.NoError(t, resultErr)
				assert.Equal(t, inputCtx, resultCtx)
			}
		})
	}
}

func TestRecoverPanicRecovery(t *testing.T) {
	recover := &Recover{stepName: "panic-step"}
	pipeline := &mockPipeline{}
	nextCalled := false

	next := func(ctx StepContext) (StepContext, error) {
		nextCalled = true
		// Simulate different types of panics
		panic("string panic")
	}

	nextFunc, err := recover.Bootstrap(pipeline, next)
	require.NoError(t, err)
	require.NotNil(t, nextFunc)

	inputCtx := StepContext{}
	resultCtx, resultErr := nextFunc(inputCtx)

	assert.True(t, nextCalled)
	assert.Error(t, resultErr)
	assert.Contains(t, resultErr.Error(), "panic in step `panic-step`")
	assert.Contains(t, resultErr.Error(), "string panic")
	assert.Contains(t, resultErr.Error(), "trace:")
	assert.Equal(t, inputCtx, resultCtx)
}

func TestRecoverContextPreservation(t *testing.T) {
	recover := &Recover{stepName: "context-step"}
	pipeline := &mockPipeline{}
	nextCalled := false

	next := func(ctx StepContext) (StepContext, error) {
		nextCalled = true
		// Modify context
		ctx.Dir = "/modified"
		ctx.Envs["MODIFIED"] = "true"
		return ctx, nil
	}

	nextFunc, err := recover.Bootstrap(pipeline, next)
	require.NoError(t, err)
	require.NotNil(t, nextFunc)

	inputCtx := StepContext{
		Dir: "/original",
		Envs: map[string]string{
			"ORIGINAL": "true",
		},
	}
	resultCtx, resultErr := nextFunc(inputCtx)

	assert.True(t, nextCalled)
	assert.NoError(t, resultErr)
	assert.Equal(t, "/modified", resultCtx.Dir)
	assert.Equal(t, "true", resultCtx.Envs["MODIFIED"])
	assert.Equal(t, "true", resultCtx.Envs["ORIGINAL"])
}
