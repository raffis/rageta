package processor

import (
	"testing"
	"time"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSkipDoneBuilder(t *testing.T) {
	tests := []struct {
		name      string
		skipDone  bool
		spec      *v1beta1.Step
		expectNil bool
	}{
		{
			name:      "skipDone false returns nil",
			skipDone:  false,
			spec:      &v1beta1.Step{Name: "test-step"},
			expectNil: true,
		},
		{
			name:      "skipDone true returns SkipDone struct",
			skipDone:  true,
			spec:      &v1beta1.Step{Name: "test-step"},
			expectNil: false,
		},
		{
			name:      "skipDone true with empty step name",
			skipDone:  true,
			spec:      &v1beta1.Step{Name: ""},
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := WithSkipDone(tt.skipDone)
			bootstraper := builder(tt.spec)

			if tt.expectNil {
				assert.Nil(t, bootstraper)
			} else {
				assert.NotNil(t, bootstraper)
				skipDone, ok := bootstraper.(*SkipDone)
				assert.True(t, ok)
				assert.Equal(t, tt.spec.Name, skipDone.stepName)
			}
		})
	}
}

func TestSkipDoneBootstrap(t *testing.T) {
	tests := []struct {
		name          string
		stepName      string
		steps         map[string]*StepContext
		expectSkip    bool
		expectError   bool
		errorType     error
		expectedCalls int
	}{
		{
			name:     "no matching step in context",
			stepName: "test-step",
			steps: map[string]*StepContext{
				"other-step": {},
			},
			expectSkip:    false,
			expectError:   false,
			expectedCalls: 1,
		},
		{
			name:     "matching step with error but ended",
			stepName: "test-step",
			steps: map[string]*StepContext{
				"test-step": {
					Error:   assert.AnError,
					EndedAt: time.Now(),
				},
			},
			expectSkip:    false,
			expectError:   false,
			expectedCalls: 1,
		},
		{
			name:     "matching step with error and not ended",
			stepName: "test-step",
			steps: map[string]*StepContext{
				"test-step": {
					Error:   assert.AnError,
					EndedAt: time.Time{}, // Zero time
				},
			},
			expectSkip:    true,
			expectError:   true,
			errorType:     ErrSkipDone,
			expectedCalls: 0,
		},
		{
			name:     "matching step without error",
			stepName: "test-step",
			steps: map[string]*StepContext{
				"test-step": {
					Error:   nil,
					EndedAt: time.Time{}, // Zero time
				},
			},
			expectSkip:    false,
			expectError:   false,
			expectedCalls: 1,
		},
		{
			name:     "empty step name",
			stepName: "",
			steps: map[string]*StepContext{
				"": {
					Error:   assert.AnError,
					EndedAt: time.Time{},
				},
			},
			expectSkip:    true,
			expectError:   true,
			errorType:     ErrSkipDone,
			expectedCalls: 0,
		},
		{
			name:     "multiple steps, one matching with error",
			stepName: "test-step",
			steps: map[string]*StepContext{
				"step1": {
					Error:   assert.AnError,
					EndedAt: time.Time{},
				},
				"test-step": {
					Error:   assert.AnError,
					EndedAt: time.Time{},
				},
				"step2": {
					Error:   nil,
					EndedAt: time.Time{},
				},
			},
			expectSkip:    true,
			expectError:   true,
			errorType:     ErrSkipDone,
			expectedCalls: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skipDone := &SkipDone{stepName: tt.stepName}
			pipeline := &mockPipeline{}
			callCount := 0

			next := func(ctx StepContext) (StepContext, error) {
				callCount++
				return ctx, nil
			}

			nextFunc, err := skipDone.Bootstrap(pipeline, next)
			require.NoError(t, err)
			require.NotNil(t, nextFunc)

			inputCtx := StepContext{
				Steps: tt.steps,
			}
			resultCtx, resultErr := nextFunc(inputCtx)

			assert.Equal(t, tt.expectedCalls, callCount)

			if tt.expectError {
				assert.Error(t, resultErr)
				assert.ErrorIs(t, resultErr, tt.errorType)
				assert.Equal(t, inputCtx, resultCtx)
			} else {
				assert.NoError(t, resultErr)
				assert.Equal(t, inputCtx, resultCtx)
			}
		})
	}
}

func TestSkipDoneErrorPropagation(t *testing.T) {
	skipDone := &SkipDone{stepName: "test-step"}
	pipeline := &mockPipeline{}
	nextCalled := false

	next := func(ctx StepContext) (StepContext, error) {
		nextCalled = true
		return ctx, assert.AnError
	}

	nextFunc, err := skipDone.Bootstrap(pipeline, next)
	require.NoError(t, err)
	require.NotNil(t, nextFunc)

	// Test case where step should not be skipped
	inputCtx := StepContext{
		Steps: map[string]*StepContext{
			"test-step": {
				Error:   nil, // No error, so shouldn't skip
				EndedAt: time.Time{},
			},
		},
	}
	resultCtx, resultErr := nextFunc(inputCtx)

	assert.True(t, nextCalled)
	assert.Error(t, resultErr)
	assert.ErrorIs(t, resultErr, assert.AnError)
	assert.Equal(t, inputCtx, resultCtx)
}
