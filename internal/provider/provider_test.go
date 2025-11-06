package provider

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
)

func TestNew(t *testing.T) {
	scheme := kruntime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	factory := serializer.NewCodecFactory(scheme)
	decoder := factory.UniversalDeserializer()

	tests := []struct {
		name     string
		decoder  kruntime.Decoder
		handlers []Resolver
	}{
		{
			name:     "create provider with decoder and handlers",
			decoder:  decoder,
			handlers: []Resolver{},
		},
		{
			name:    "create provider with multiple handlers",
			decoder: decoder,
			handlers: []Resolver{
				func(ctx context.Context, ref string) (io.Reader, error) {
					return nil, nil
				},
				func(ctx context.Context, ref string) (io.Reader, error) {
					return nil, nil
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(tt.decoder, tt.handlers...)

			require.NotNil(t, p)
			assert.Equal(t, tt.decoder, p.decoder)
			assert.Equal(t, len(tt.handlers), len(p.handlers))
		})
	}
}

func TestProvider_Resolve(t *testing.T) {
	scheme := kruntime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	factory := serializer.NewCodecFactory(scheme)
	decoder := factory.UniversalDeserializer()
	encoder := json.NewYAMLSerializer(json.DefaultMetaFactory, scheme, scheme)

	createValidManifest := func(name string) []byte {
		pipeline := v1beta1.Pipeline{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Pipeline",
				APIVersion: v1beta1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		}
		var buf bytes.Buffer
		err := encoder.Encode(&pipeline, &buf)
		require.NoError(t, err)
		return buf.Bytes()
	}

	tests := []struct {
		name          string
		handlers      []Resolver
		ref           string
		ctxSetup      func() context.Context
		expectError   bool
		errorMsg      string
		errorContains []string
		expectedName  string
	}{
		{
			name: "successful resolve with first handler",
			handlers: []Resolver{
				func(ctx context.Context, ref string) (io.Reader, error) {
					return bytes.NewReader(createValidManifest("test-pipeline")), nil
				},
			},
			ref:          "test-ref",
			expectError:  false,
			expectedName: "test-pipeline",
		},
		{
			name: "successful resolve with second handler",
			handlers: []Resolver{
				func(ctx context.Context, ref string) (io.Reader, error) {
					return nil, errors.New("first handler error")
				},
				func(ctx context.Context, ref string) (io.Reader, error) {
					return bytes.NewReader(createValidManifest("test-pipeline")), nil
				},
			},
			ref:          "test-ref",
			expectError:  false,
			expectedName: "test-pipeline",
		},
		{
			name: "all handlers fail",
			handlers: []Resolver{
				func(ctx context.Context, ref string) (io.Reader, error) {
					return nil, errors.New("first handler error")
				},
				func(ctx context.Context, ref string) (io.Reader, error) {
					return nil, errors.New("second handler error")
				},
			},
			ref:           "test-ref",
			expectError:   true,
			errorContains: []string{"could not lookup ref", "test-ref", "first handler error", "second handler error"},
		},
		{
			name: "read error from reader",
			handlers: []Resolver{
				func(ctx context.Context, ref string) (io.Reader, error) {
					return &mockReader{err: errors.New("read error")}, nil
				},
			},
			ref:         "test-ref",
			expectError: true,
			errorMsg:    "read error",
		},
		{
			name: "decode error with invalid data",
			handlers: []Resolver{
				func(ctx context.Context, ref string) (io.Reader, error) {
					return bytes.NewReader([]byte("invalid yaml data")), nil
				},
			},
			ref:         "test-ref",
			expectError: true,
		},
		{
			name: "handler order - second handler succeeds after first fails",
			handlers: []Resolver{
				func(ctx context.Context, ref string) (io.Reader, error) {
					return nil, errors.New("first error")
				},
				func(ctx context.Context, ref string) (io.Reader, error) {
					return nil, errors.New("second error")
				},
				func(ctx context.Context, ref string) (io.Reader, error) {
					return bytes.NewReader(createValidManifest("test-pipeline")), nil
				},
			},
			ref:          "test-ref",
			expectError:  false,
			expectedName: "test-pipeline",
		},
		{
			name: "error joining with multiple handlers",
			handlers: []Resolver{
				func(ctx context.Context, ref string) (io.Reader, error) {
					return nil, errors.New("first error")
				},
				func(ctx context.Context, ref string) (io.Reader, error) {
					return nil, errors.New("second error")
				},
				func(ctx context.Context, ref string) (io.Reader, error) {
					return nil, errors.New("third error")
				},
			},
			ref:           "test-ref",
			expectError:   true,
			errorContains: []string{"could not lookup ref", "test-ref", "first error", "second error", "third error"},
		},
		{
			name:          "empty handlers",
			handlers:      []Resolver{},
			ref:           "test-ref",
			expectError:   true,
			errorContains: []string{"could not lookup ref"},
		},
		{
			name: "context cancellation",
			handlers: []Resolver{
				func(ctx context.Context, ref string) (io.Reader, error) {
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					default:
						return bytes.NewReader(createValidManifest("test-pipeline")), nil
					}
				},
			},
			ref: "test-ref",
			ctxSetup: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			expectError:   true,
			errorContains: []string{"context canceled"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(decoder, tt.handlers...)

			ctx := context.Background()
			if tt.ctxSetup != nil {
				ctx = tt.ctxSetup()
			}

			result, err := p.Resolve(ctx, tt.ref)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				if len(tt.errorContains) > 0 {
					for _, msg := range tt.errorContains {
						assert.Contains(t, err.Error(), msg)
					}
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				if tt.expectedName != "" {
					assert.Equal(t, tt.expectedName, result.Name)
				}
			}
		})
	}
}
