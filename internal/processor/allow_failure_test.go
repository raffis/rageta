package processor

import (
	"errors"
	"testing"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllowFailureBuilder(t *testing.T) {
	tests := []struct {
		name      string
		spec      *v1beta1.Step
		expectNil bool
	}{
		{
			name: "AllowFailure false returns nil",
			spec: &v1beta1.Step{
				StepOptions: v1beta1.StepOptions{
					AllowFailure: false,
				},
			},
			expectNil: true,
		},
		{
			name: "AllowFailure true returns AllowFailure struct",
			spec: &v1beta1.Step{
				StepOptions: v1beta1.StepOptions{
					AllowFailure: true,
				},
			},
			expectNil: false,
		},
		{
			name:      "AllowFailure not set returns nil",
			spec:      &v1beta1.Step{},
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := WithAllowFailure()
			bootstraper := builder(tt.spec)

			if tt.expectNil {
				assert.Nil(t, bootstraper)
			} else {
				assert.NotNil(t, bootstraper)
				_, ok := bootstraper.(*AllowFailure)
				assert.True(t, ok)
			}
		})
	}
}

func TestAllowFailureBootstrap(t *testing.T) {
	tests := []struct {
		name        string
		inputError  error
		expectError bool
		errorType   error
	}{
		{
			name:        "no error from next function",
			inputError:  nil,
			expectError: false,
		},
		{
			name:        "error from next function is wrapped",
			inputError:  errors.New("test error"),
			expectError: true,
			errorType:   ErrAllowFailure,
		},
		{
			name:        "custom error from next function is wrapped",
			inputError:  errors.New("custom error message"),
			expectError: true,
			errorType:   ErrAllowFailure,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowFailure := &AllowFailure{}
			pipeline := &mockPipeline{}
			nextCalled := false

			next := func(ctx StepContext) (StepContext, error) {
				nextCalled = true
				return ctx, tt.inputError
			}

			nextFunc, err := allowFailure.Bootstrap(pipeline, next)
			require.NoError(t, err)
			require.NotNil(t, nextFunc)

			ctx := StepContext{}
			resultCtx, resultErr := nextFunc(ctx)

			assert.True(t, nextCalled)
			assert.Equal(t, ctx, resultCtx)

			if tt.expectError {
				assert.Error(t, resultErr)
				assert.ErrorIs(t, resultErr, tt.errorType)
				if tt.inputError != nil {
					assert.ErrorIs(t, resultErr, tt.inputError)
				}
			} else {
				assert.NoError(t, resultErr)
			}
		})
	}
}
