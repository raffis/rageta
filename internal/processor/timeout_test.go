package processor

import (
	"context"
	"testing"
	"time"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTimeoutBuilder(t *testing.T) {
	tests := []struct {
		name      string
		spec      *v1beta1.Step
		expectNil bool
	}{
		{
			name: "timeout duration zero returns nil",
			spec: &v1beta1.Step{
				StepOptions: v1beta1.StepOptions{
					Timeout: metav1.Duration{Duration: 0},
				},
			},
			expectNil: true,
		},
		{
			name: "timeout duration set returns Timeout struct",
			spec: &v1beta1.Step{
				StepOptions: v1beta1.StepOptions{
					Timeout: metav1.Duration{Duration: 5 * time.Second},
				},
			},
			expectNil: false,
		},
		{
			name:      "timeout not set returns nil",
			spec:      &v1beta1.Step{},
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := WithTimeout()
			bootstraper := builder(tt.spec)

			if tt.expectNil {
				assert.Nil(t, bootstraper)
			} else {
				assert.NotNil(t, bootstraper)
				timeout, ok := bootstraper.(*Timeout)
				assert.True(t, ok)
				assert.NotNil(t, timeout.timer)
			}
		})
	}
}

func TestTimeoutBootstrap(t *testing.T) {
	tests := []struct {
		name        string
		timeout     time.Duration
		nextDelay   time.Duration
		expectError bool
	}{
		{
			name:        "next function completes within timeout",
			timeout:     100 * time.Millisecond,
			nextDelay:   10 * time.Millisecond,
			expectError: false,
		},
		{
			name:        "next function completes immediately",
			timeout:     100 * time.Millisecond,
			nextDelay:   0,
			expectError: false,
		},
		{
			name:        "next function exceeds timeout",
			timeout:     10 * time.Millisecond,
			nextDelay:   10000 * time.Millisecond,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timeout := &Timeout{timer: time.NewTimer(tt.timeout)}
			pipeline := &mockPipeline{}
			nextCalled := false

			next := func(ctx StepContext) (StepContext, error) {
				if tt.nextDelay > 0 {
					time.Sleep(tt.nextDelay)
				}
				nextCalled = true

				return ctx, nil
			}

			nextFunc, err := timeout.Bootstrap(pipeline, next)
			require.NoError(t, err)
			require.NotNil(t, nextFunc)

			ctx := StepContext{Context: context.Background()}
			resultCtx, resultErr := nextFunc(ctx)

			if tt.expectError {
				assert.False(t, nextCalled)
				assert.Error(t, resultErr)
				assert.Contains(t, resultErr.Error(), "operation timed out")
			} else {
				assert.True(t, nextCalled)
				assert.NoError(t, resultErr)
				assert.Equal(t, ctx, resultCtx)
			}
		})
	}
}
