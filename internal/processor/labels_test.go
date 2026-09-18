package processor

import (
	"testing"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLabelsBuilder(t *testing.T) {
	tests := []struct {
		name         string
		globalLabels []Label
		spec         *v1beta1.Task
		expectNil    bool
	}{
		{
			name:         "no global tags and no spec tags returns nil",
			globalLabels: []Label{},
			spec:         &v1beta1.Task{},
			expectNil:    true,
		},
		{
			name: "global tags only returns Labels struct",
			globalLabels: []Label{
				{Key: "env", Value: "prod", HEXColor: "#FF0000"},
			},
			spec:      &v1beta1.Task{},
			expectNil: false,
		},
		{
			name:         "spec tags only returns Labels struct",
			globalLabels: []Label{},
			spec: &v1beta1.Task{
				TaskOptions: v1beta1.TaskOptions{
					Labels: []v1beta1.Label{
						{Name: "service", Value: "api", HEXColor: "#00FF00"},
					},
				},
			},
			expectNil: false,
		},
		{
			name: "both global and spec tags returns Labels struct",
			globalLabels: []Label{
				{Key: "env", Value: "prod", HEXColor: "#FF0000"},
			},
			spec: &v1beta1.Task{
				TaskOptions: v1beta1.TaskOptions{
					Labels: []v1beta1.Label{
						{Name: "service", Value: "api", HEXColor: "#00FF00"},
					},
				},
			},
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := WithLabels(tt.globalLabels)
			bootstraper := builder(tt.spec)

			if tt.expectNil {
				assert.Nil(t, bootstraper)
			} else {
				assert.NotNil(t, bootstraper)
				tags, ok := bootstraper.(*Labels)
				assert.True(t, ok)
				assert.Equal(t, tt.globalLabels, tags.globalLabels)
				if tt.spec != nil {
					assert.Equal(t, tt.spec.Labels, tags.tags)
				}
			}
		})
	}
}

func TestLabelsBootstrap(t *testing.T) {
	tests := []struct {
		name          string
		specLabels    []v1beta1.Label
		globalLabels  []Label
		inputContext  TaskContext
		expectedNext  TaskContext
		expectedAfter TaskContext
		shouldError   bool
	}{
		{
			name:          "empty tags and global tags",
			specLabels:    []v1beta1.Label{},
			globalLabels:  []Label{},
			inputContext:  TaskContext{},
			expectedNext:  TaskContext{},
			expectedAfter: TaskContext{},
			shouldError:   false,
		},
		{
			name: "only spec tags",
			specLabels: []v1beta1.Label{
				{Name: "service", Value: "api", HEXColor: "#00FF00"},
				{Name: "version", Value: "v1.0.0", HEXColor: "#0000FF"},
			},
			globalLabels: []Label{},
			inputContext: TaskContext{},
			expectedNext: TaskContext{
				Labels: LabelsContext{tags: []Label{
					{Key: "service", Value: "api", HEXColor: "#00FF00"},
					{Key: "version", Value: "v1.0.0", HEXColor: "#0000FF"},
				}},
			},
			expectedAfter: TaskContext{},
			shouldError:   false,
		},
		{
			name:       "only global tags",
			specLabels: []v1beta1.Label{},
			globalLabels: []Label{
				{Key: "env", Value: "prod", HEXColor: "#FF0000"},
				{Key: "region", Value: "us-west", HEXColor: "#FFFF00"},
			},
			inputContext: TaskContext{},
			expectedNext: TaskContext{
				Labels: LabelsContext{tags: []Label{
					{Key: "env", Value: "prod", HEXColor: "#FF0000"},
					{Key: "region", Value: "us-west", HEXColor: "#FFFF00"},
				}},
			},
			expectedAfter: TaskContext{},
			shouldError:   false,
		},
		{
			name: "both spec and global tags",
			specLabels: []v1beta1.Label{
				{Name: "service", Value: "api", HEXColor: "#00FF00"},
			},
			globalLabels: []Label{
				{Key: "env", Value: "prod", HEXColor: "#FF0000"},
			},
			inputContext: TaskContext{},
			expectedNext: TaskContext{
				Labels: LabelsContext{tags: []Label{
					{Key: "service", Value: "api", HEXColor: "#00FF00"},
					{Key: "env", Value: "prod", HEXColor: "#FF0000"},
				}},
			},
			expectedAfter: TaskContext{},
			shouldError:   false,
		},
		{
			name: "existing context tags are preserved after execution",
			specLabels: []v1beta1.Label{
				{Name: "service", Value: "api", HEXColor: "#00FF00"},
			},
			globalLabels: []Label{},
			inputContext: TaskContext{
				Labels: LabelsContext{tags: []Label{
					{Key: "existing", Value: "tag", HEXColor: "#CCCCCC"},
				}},
			},
			expectedNext: TaskContext{
				Labels: LabelsContext{tags: []Label{
					{Key: "existing", Value: "tag", HEXColor: "#CCCCCC"},
					{Key: "service", Value: "api", HEXColor: "#00FF00"},
				}},
			},
			expectedAfter: TaskContext{
				Labels: LabelsContext{tags: []Label{
					{Key: "existing", Value: "tag", HEXColor: "#CCCCCC"},
				}},
			},
			shouldError: false,
		},
		{
			name: "error handling - error propagation",
			specLabels: []v1beta1.Label{
				{Name: "service", Value: "api", HEXColor: "#00FF00"},
			},
			globalLabels:  []Label{},
			inputContext:  TaskContext{},
			expectedNext:  TaskContext{},
			expectedAfter: TaskContext{},
			shouldError:   true,
		},
		{
			name: "tag overwriting - same key overwrites existing",
			specLabels: []v1beta1.Label{
				{Name: "service", Value: "new-api", HEXColor: "#00FF00"},
			},
			globalLabels: []Label{},
			inputContext: TaskContext{
				Labels: LabelsContext{tags: []Label{
					{Key: "service", Value: "old-api", HEXColor: "#CCCCCC"},
					{Key: "env", Value: "prod", HEXColor: "#FF0000"},
				}},
			},
			expectedNext: TaskContext{
				Labels: LabelsContext{tags: []Label{
					{Key: "service", Value: "new-api", HEXColor: "#00FF00"},
					{Key: "env", Value: "prod", HEXColor: "#FF0000"},
				}},
			},
			expectedAfter: TaskContext{
				Labels: LabelsContext{tags: []Label{
					{Key: "service", Value: "old-api", HEXColor: "#CCCCCC"},
					{Key: "env", Value: "prod", HEXColor: "#FF0000"},
				}},
			},
			shouldError: false,
		},
		{
			name:       "empty spec tags with global tags",
			specLabels: []v1beta1.Label{},
			globalLabels: []Label{
				{Key: "env", Value: "prod", HEXColor: "#FF0000"},
			},
			inputContext: TaskContext{},
			expectedNext: TaskContext{
				Labels: LabelsContext{tags: []Label{
					{Key: "env", Value: "prod", HEXColor: "#FF0000"},
				}},
			},
			expectedAfter: TaskContext{},
			shouldError:   false,
		},
		{
			name: "empty global tags with spec tags",
			specLabels: []v1beta1.Label{
				{Name: "service", Value: "api", HEXColor: "#00FF00"},
			},
			globalLabels: []Label{},
			inputContext: TaskContext{},
			expectedNext: TaskContext{
				Labels: LabelsContext{tags: []Label{
					{Key: "service", Value: "api", HEXColor: "#00FF00"},
				}},
			},
			expectedAfter: TaskContext{},
			shouldError:   false,
		},
		{
			name: "complex tag mapping - multiple tags and proper mapping",
			specLabels: []v1beta1.Label{
				{Name: "service", Value: "api", HEXColor: "#00FF00"},
				{Name: "version", Value: "v1.0.0", HEXColor: "#0000FF"},
				{Name: "component", Value: "backend", HEXColor: "#FF00FF"},
			},
			globalLabels: []Label{
				{Key: "env", Value: "prod", HEXColor: "#FF0000"},
				{Key: "region", Value: "us-west", HEXColor: "#FFFF00"},
			},
			inputContext: TaskContext{
				Labels: LabelsContext{tags: []Label{
					{Key: "existing", Value: "tag", HEXColor: "#CCCCCC"},
				}},
			},
			expectedNext: TaskContext{
				Labels: LabelsContext{tags: []Label{
					{Key: "existing", Value: "tag", HEXColor: "#CCCCCC"},
					{Key: "service", Value: "api", HEXColor: "#00FF00"},
					{Key: "version", Value: "v1.0.0", HEXColor: "#0000FF"},
					{Key: "component", Value: "backend", HEXColor: "#FF00FF"},
					{Key: "env", Value: "prod", HEXColor: "#FF0000"},
					{Key: "region", Value: "us-west", HEXColor: "#FFFF00"},
				}},
			},
			expectedAfter: TaskContext{
				Labels: LabelsContext{tags: []Label{
					{Key: "existing", Value: "tag", HEXColor: "#CCCCCC"},
				}},
			},
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags := &Labels{
				tags:         tt.specLabels,
				globalLabels: tt.globalLabels,
			}

			ctx := tt.inputContext

			// Create a mock pipeline and next function
			pipeline := &mockPipeline{}
			nextCalled := false

			next := func(ctx TaskContext) (TaskContext, error) {
				nextCalled = true

				if tt.shouldError {
					return ctx, assert.AnError
				}

				// Verify that tags were added during execution
				actualLabels := ctx.Labels.Labels()
				expectedLabels := tt.expectedNext.Labels.Labels()
				assert.ElementsMatch(t, expectedLabels, actualLabels)
				return ctx, nil
			}

			nextFunc, err := tags.Bootstrap(pipeline, next)
			require.NoError(t, err)
			require.NotNil(t, nextFunc)

			resultCtx, err := nextFunc(ctx)

			if tt.shouldError {
				assert.Error(t, err)
				assert.Equal(t, assert.AnError, err)
			} else {
				require.NoError(t, err)
			}

			assert.True(t, nextCalled)
			// Verify that original tags are restored after execution
			expectedLabels := tt.expectedAfter.Labels.Labels()
			assert.Equal(t, expectedLabels, resultCtx.Labels.Labels())

		})
	}
}
