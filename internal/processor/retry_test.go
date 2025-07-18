package processor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRetryBuilder(t *testing.T) {
	tests := []struct {
		name      string
		spec      *v1beta1.Step
		expectNil bool
	}{
		{
			name:      "retry nil returns nil",
			spec:      &v1beta1.Step{},
			expectNil: true,
		},
		{
			name: "retry with exponential backoff returns Retry struct",
			spec: &v1beta1.Step{
				StepOptions: v1beta1.StepOptions{
					Retry: &v1beta1.Retry{
						Exponential: metav1.Duration{Duration: 1 * time.Second},
						MaxRetries:  3,
					},
				},
			},
			expectNil: false,
		},
		{
			name: "retry with constant backoff returns Retry struct",
			spec: &v1beta1.Step{
				StepOptions: v1beta1.StepOptions{
					Retry: &v1beta1.Retry{
						Constant:   metav1.Duration{Duration: 2 * time.Second},
						MaxRetries: 5,
					},
				},
			},
			expectNil: false,
		},
		{
			name: "retry with both exponential and constant uses exponential",
			spec: &v1beta1.Step{
				StepOptions: v1beta1.StepOptions{
					Retry: &v1beta1.Retry{
						Exponential: metav1.Duration{Duration: 1 * time.Second},
						Constant:    metav1.Duration{Duration: 2 * time.Second},
						MaxRetries:  3,
					},
				},
			},
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := WithRetry()
			bootstraper := builder(tt.spec)

			if tt.expectNil {
				assert.Nil(t, bootstraper)
			} else {
				assert.NotNil(t, bootstraper)
				retry, ok := bootstraper.(*Retry)
				assert.True(t, ok)
				if tt.spec.Retry != nil {
					assert.Equal(t, uint64(tt.spec.Retry.MaxRetries), retry.max)
					assert.Equal(t, tt.spec.Retry.Exponential.Duration, retry.exponential)
					assert.Equal(t, tt.spec.Retry.Constant.Duration, retry.constant)
				}
			}
		})
	}
}

func TestRetryBootstrap(t *testing.T) {
	tests := []struct {
		name          string
		maxRetries    uint64
		exponential   time.Duration
		constant      time.Duration
		nextError     error
		expectedCalls int
		expectError   bool
		expectRetry   bool
	}{
		{
			name:          "no error from next function",
			maxRetries:    3,
			exponential:   10 * time.Millisecond,
			nextError:     nil,
			expectedCalls: 1,
			expectError:   false,
			expectRetry:   false,
		},
		{
			name:          "error from next function, retries exhausted",
			maxRetries:    2,
			exponential:   10 * time.Millisecond,
			nextError:     errors.New("test error"),
			expectedCalls: 3, // 1 initial + 2 retries
			expectError:   true,
			expectRetry:   true,
		},
		{
			name:          "error from next function, succeeds on retry",
			maxRetries:    3,
			exponential:   10 * time.Millisecond,
			nextError:     errors.New("test error"),
			expectedCalls: 2, // 1 initial + 1 retry, then succeeds
			expectError:   false,
			expectRetry:   true,
		},
		{
			name:          "constant backoff with error",
			maxRetries:    2,
			constant:      10 * time.Millisecond,
			nextError:     errors.New("test error"),
			expectedCalls: 3, // 1 initial + 2 retries
			expectError:   true,
			expectRetry:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retry := &Retry{
				max:         tt.maxRetries,
				exponential: tt.exponential,
				constant:    tt.constant,
			}
			pipeline := &mockPipeline{}
			callCount := 0

			next := func(ctx StepContext) (StepContext, error) {
				callCount++
				// Simulate success after first failure for some tests
				if tt.name == "error from next function, succeeds on retry" && callCount > 1 {
					return ctx, nil
				}
				return ctx, tt.nextError
			}

			nextFunc, err := retry.Bootstrap(pipeline, next)
			require.NoError(t, err)
			require.NotNil(t, nextFunc)

			ctx := StepContext{Context: context.Background()}
			resultCtx, resultErr := nextFunc(ctx)

			assert.Equal(t, tt.expectedCalls, callCount)

			if tt.expectError {
				assert.Error(t, resultErr)
				if tt.nextError != nil {
					assert.ErrorIs(t, resultErr, tt.nextError)
				}
			} else {
				assert.NoError(t, resultErr)
				// Don't compare contexts directly as they may be modified
				assert.NotNil(t, resultCtx)
			}
		})
	}
}

func TestRetryBackoffStrategies(t *testing.T) {
	tests := []struct {
		name        string
		exponential time.Duration
		constant    time.Duration
		expectType  string
	}{
		{
			name:        "exponential backoff strategy",
			exponential: 1 * time.Millisecond,
			constant:    0,
		},
		{
			name:        "constant backoff strategy",
			exponential: 0,
			constant:    2 * time.Millisecond,
		},
		{
			name:        "exponential takes precedence over constant",
			exponential: 1 * time.Millisecond,
			constant:    2 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retry := &Retry{
				max:         1, // Limit to 1 retry to avoid infinite loops
				exponential: tt.exponential,
				constant:    tt.constant,
			}
			pipeline := &mockPipeline{}
			callCount := 0

			next := func(ctx StepContext) (StepContext, error) {
				callCount++
				return ctx, errors.New("test error")
			}

			nextFunc, err := retry.Bootstrap(pipeline, next)
			require.NoError(t, err)
			require.NotNil(t, nextFunc)

			ctx := StepContext{Context: context.Background()}
			start := time.Now()
			_, resultErr := nextFunc(ctx)
			duration := time.Since(start)

			assert.Error(t, resultErr)
			assert.Equal(t, 2, callCount) // 1 initial + 1 retry

			switch {
			case tt.exponential > 0 && tt.constant > 0:
				assert.Greater(t, duration, tt.exponential, "Should have some delay")
				assert.Less(t, duration, tt.exponential+tt.constant, "Should have some delay")
			case tt.exponential > 0:
				assert.Greater(t, duration, tt.exponential, "Should have some delay")
			default:
				assert.Greater(t, duration, tt.constant-tt.exponential, "Should have some delay")
			}
		})
	}
}
