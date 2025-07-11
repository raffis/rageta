package processor

import (
	"testing"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTagsBuilder(t *testing.T) {
	tests := []struct {
		name       string
		globalTags []Tag
		spec       *v1beta1.Step
		expectNil  bool
	}{
		{
			name:       "no global tags and no spec tags returns nil",
			globalTags: []Tag{},
			spec:       &v1beta1.Step{},
			expectNil:  true,
		},
		{
			name: "global tags only returns Tags struct",
			globalTags: []Tag{
				{Key: "env", Value: "prod", Color: "#FF0000"},
			},
			spec:      &v1beta1.Step{},
			expectNil: false,
		},
		{
			name:       "spec tags only returns Tags struct",
			globalTags: []Tag{},
			spec: &v1beta1.Step{
				StepOptions: v1beta1.StepOptions{
					Tags: []v1beta1.Tag{
						{Name: "service", Value: "api", Color: "#00FF00"},
					},
				},
			},
			expectNil: false,
		},
		{
			name: "both global and spec tags returns Tags struct",
			globalTags: []Tag{
				{Key: "env", Value: "prod", Color: "#FF0000"},
			},
			spec: &v1beta1.Step{
				StepOptions: v1beta1.StepOptions{
					Tags: []v1beta1.Tag{
						{Name: "service", Value: "api", Color: "#00FF00"},
					},
				},
			},
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := WithTags(tt.globalTags)
			bootstraper := builder(tt.spec)

			if tt.expectNil {
				assert.Nil(t, bootstraper)
			} else {
				assert.NotNil(t, bootstraper)
				tags, ok := bootstraper.(*Tags)
				assert.True(t, ok)
				assert.Equal(t, tt.globalTags, tags.globalTags)
				if tt.spec != nil {
					assert.Equal(t, tt.spec.StepOptions.Tags, tags.tags)
				}
			}
		})
	}
}

func TestTagsBootstrap(t *testing.T) {
	tests := []struct {
		name          string
		specTags      []v1beta1.Tag
		globalTags    []Tag
		inputContext  StepContext
		expectedNext  StepContext
		expectedAfter StepContext
		shouldError   bool
	}{
		{
			name:          "empty tags and global tags",
			specTags:      []v1beta1.Tag{},
			globalTags:    []Tag{},
			inputContext:  StepContext{},
			expectedNext:  StepContext{},
			expectedAfter: StepContext{},
			shouldError:   false,
		},
		{
			name: "only spec tags",
			specTags: []v1beta1.Tag{
				{Name: "service", Value: "api", Color: "#00FF00"},
				{Name: "version", Value: "v1.0.0", Color: "#0000FF"},
			},
			globalTags:   []Tag{},
			inputContext: StepContext{},
			expectedNext: StepContext{
				tags: []Tag{
					{Key: "service", Value: "api", Color: "#00FF00"},
					{Key: "version", Value: "v1.0.0", Color: "#0000FF"},
				},
			},
			expectedAfter: StepContext{},
			shouldError:   false,
		},
		{
			name:     "only global tags",
			specTags: []v1beta1.Tag{},
			globalTags: []Tag{
				{Key: "env", Value: "prod", Color: "#FF0000"},
				{Key: "region", Value: "us-west", Color: "#FFFF00"},
			},
			inputContext: StepContext{},
			expectedNext: StepContext{
				tags: []Tag{
					{Key: "env", Value: "prod", Color: "#FF0000"},
					{Key: "region", Value: "us-west", Color: "#FFFF00"},
				},
			},
			expectedAfter: StepContext{},
			shouldError:   false,
		},
		{
			name: "both spec and global tags",
			specTags: []v1beta1.Tag{
				{Name: "service", Value: "api", Color: "#00FF00"},
			},
			globalTags: []Tag{
				{Key: "env", Value: "prod", Color: "#FF0000"},
			},
			inputContext: StepContext{},
			expectedNext: StepContext{
				tags: []Tag{
					{Key: "service", Value: "api", Color: "#00FF00"},
					{Key: "env", Value: "prod", Color: "#FF0000"},
				},
			},
			expectedAfter: StepContext{},
			shouldError:   false,
		},
		{
			name: "existing context tags are preserved after execution",
			specTags: []v1beta1.Tag{
				{Name: "service", Value: "api", Color: "#00FF00"},
			},
			globalTags: []Tag{},
			inputContext: StepContext{
				tags: []Tag{
					{Key: "existing", Value: "tag", Color: "#CCCCCC"},
				},
			},
			expectedNext: StepContext{
				tags: []Tag{
					{Key: "existing", Value: "tag", Color: "#CCCCCC"},
					{Key: "service", Value: "api", Color: "#00FF00"},
				},
			},
			expectedAfter: StepContext{
				tags: []Tag{
					{Key: "existing", Value: "tag", Color: "#CCCCCC"},
				},
			},
			shouldError: false,
		},
		{
			name: "error handling - error propagation",
			specTags: []v1beta1.Tag{
				{Name: "service", Value: "api", Color: "#00FF00"},
			},
			globalTags:    []Tag{},
			inputContext:  StepContext{},
			expectedNext:  StepContext{},
			expectedAfter: StepContext{},
			shouldError:   true,
		},
		{
			name: "tag overwriting - same key overwrites existing",
			specTags: []v1beta1.Tag{
				{Name: "service", Value: "new-api", Color: "#00FF00"},
			},
			globalTags: []Tag{},
			inputContext: StepContext{
				tags: []Tag{
					{Key: "service", Value: "old-api", Color: "#CCCCCC"},
					{Key: "env", Value: "prod", Color: "#FF0000"},
				},
			},
			expectedNext: StepContext{
				tags: []Tag{
					{Key: "service", Value: "new-api", Color: "#00FF00"},
					{Key: "env", Value: "prod", Color: "#FF0000"},
				},
			},
			expectedAfter: StepContext{
				tags: []Tag{
					{Key: "service", Value: "old-api", Color: "#CCCCCC"},
					{Key: "env", Value: "prod", Color: "#FF0000"},
				},
			},
			shouldError: false,
		},
		{
			name:     "empty spec tags with global tags",
			specTags: []v1beta1.Tag{},
			globalTags: []Tag{
				{Key: "env", Value: "prod", Color: "#FF0000"},
			},
			inputContext: StepContext{},
			expectedNext: StepContext{
				tags: []Tag{
					{Key: "env", Value: "prod", Color: "#FF0000"},
				},
			},
			expectedAfter: StepContext{},
			shouldError:   false,
		},
		{
			name: "empty global tags with spec tags",
			specTags: []v1beta1.Tag{
				{Name: "service", Value: "api", Color: "#00FF00"},
			},
			globalTags:   []Tag{},
			inputContext: StepContext{},
			expectedNext: StepContext{
				tags: []Tag{
					{Key: "service", Value: "api", Color: "#00FF00"},
				},
			},
			expectedAfter: StepContext{},
			shouldError:   false,
		},
		{
			name: "complex tag mapping - multiple tags and proper mapping",
			specTags: []v1beta1.Tag{
				{Name: "service", Value: "api", Color: "#00FF00"},
				{Name: "version", Value: "v1.0.0", Color: "#0000FF"},
				{Name: "component", Value: "backend", Color: "#FF00FF"},
			},
			globalTags: []Tag{
				{Key: "env", Value: "prod", Color: "#FF0000"},
				{Key: "region", Value: "us-west", Color: "#FFFF00"},
			},
			inputContext: StepContext{
				tags: []Tag{
					{Key: "existing", Value: "tag", Color: "#CCCCCC"},
				},
			},
			expectedNext: StepContext{
				tags: []Tag{
					{Key: "existing", Value: "tag", Color: "#CCCCCC"},
					{Key: "service", Value: "api", Color: "#00FF00"},
					{Key: "version", Value: "v1.0.0", Color: "#0000FF"},
					{Key: "component", Value: "backend", Color: "#FF00FF"},
					{Key: "env", Value: "prod", Color: "#FF0000"},
					{Key: "region", Value: "us-west", Color: "#FFFF00"},
				},
			},
			expectedAfter: StepContext{
				tags: []Tag{
					{Key: "existing", Value: "tag", Color: "#CCCCCC"},
				},
			},
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags := &Tags{
				tags:       tt.specTags,
				globalTags: tt.globalTags,
			}

			ctx := tt.inputContext

			// Create a mock pipeline and next function
			pipeline := &mockPipeline{}
			nextCalled := false

			next := func(ctx StepContext) (StepContext, error) {
				nextCalled = true

				if tt.shouldError {
					return ctx, assert.AnError
				}

				// Verify that tags were added during execution
				actualTags := ctx.Tags()
				expectedTags := tt.expectedNext.Tags()
				assert.ElementsMatch(t, expectedTags, actualTags)
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
			expectedTags := tt.expectedAfter.Tags()
			assert.Equal(t, expectedTags, resultCtx.Tags())

		})
	}
}
