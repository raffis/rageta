package provider

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/raffis/rageta/pkg/apis/package/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
)

// mockReader is a mock implementation of io.Reader that can return errors
type mockReader struct {
	data  []byte
	err   error
	index int
}

func (m *mockReader) Read(p []byte) (n int, err error) {
	if m.err != nil {
		return 0, m.err
	}
	if m.index >= len(m.data) {
		return 0, io.EOF
	}
	n = copy(p, m.data[m.index:])
	m.index += n
	return n, nil
}

// mockWriter is a mock implementation of io.Writer that can return errors
type mockWriter struct {
	buf []byte
	err error
}

func (m *mockWriter) Write(p []byte) (n int, err error) {
	if m.err != nil {
		return 0, m.err
	}
	m.buf = append(m.buf, p...)
	return len(p), nil
}

// mockDBGetter is a mock implementation of dbGetter
type mockDBGetter struct {
	getFunc func(name string) ([]byte, error)
}

func (m *mockDBGetter) Get(name string) ([]byte, error) {
	if m.getFunc != nil {
		return m.getFunc(name)
	}
	return nil, errors.New("not found")
}

func TestOpenDatabase(t *testing.T) {
	// Create a real decoder/encoder for testing
	scheme := kruntime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	factory := serializer.NewCodecFactory(scheme)
	decoder := factory.UniversalDeserializer()
	encoder := json.NewYAMLSerializer(json.DefaultMetaFactory, scheme, scheme)

	// Create a valid Store manifest for testing
	store := v1beta1.Store{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Store",
			APIVersion: v1beta1.GroupVersion.String(),
		},
		StoreSpec: v1beta1.StoreSpec{
			Apps: []v1beta1.App{},
		},
	}
	var storeManifest bytes.Buffer
	err := encoder.Encode(&store, &storeManifest)
	require.NoError(t, err)

	tests := []struct {
		name        string
		reader      io.Reader
		decoder     kruntime.Decoder
		encoder     kruntime.Serializer
		expectError bool
		errorMsg    string
	}{
		{
			name:        "successful database open",
			reader:      bytes.NewReader(storeManifest.Bytes()),
			decoder:     decoder,
			encoder:     encoder,
			expectError: false,
		},
		{
			name: "read error from reader",
			reader: &mockReader{
				err: errors.New("read error"),
			},
			decoder:     decoder,
			encoder:     encoder,
			expectError: true,
			errorMsg:    "read error",
		},
		{
			name:        "decode error with invalid data",
			reader:      bytes.NewReader([]byte("invalid yaml data")),
			decoder:     decoder,
			encoder:     encoder,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := OpenDatabase(tt.reader, tt.decoder, tt.encoder)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				if tt.errorMsg == "" {
					// For decode errors, we just check that there's an error
					assert.NotNil(t, err)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, db)
				assert.NotNil(t, db.encoder)
			}
		})
	}
}

func createTestEncoder() kruntime.Serializer {
	scheme := kruntime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	return json.NewYAMLSerializer(json.DefaultMetaFactory, scheme, scheme)
}

func TestDatabase_Add(t *testing.T) {
	encoder := createTestEncoder()
	tests := []struct {
		name        string
		initialApps []v1beta1.App
		addName     string
		addManifest []byte
		expectError bool
		expectedLen int
	}{
		{
			name:        "add new app to empty database",
			initialApps: []v1beta1.App{},
			addName:     "test-app",
			addManifest: []byte("manifest data"),
			expectError: false,
			expectedLen: 1,
		},
		{
			name: "add new app to existing database",
			initialApps: []v1beta1.App{
				{Name: "existing-app", Manifest: []byte("existing")},
			},
			addName:     "test-app",
			addManifest: []byte("manifest data"),
			expectError: false,
			expectedLen: 2,
		},
		{
			name: "replace existing app",
			initialApps: []v1beta1.App{
				{Name: "test-app", Manifest: []byte("old manifest")},
			},
			addName:     "test-app",
			addManifest: []byte("new manifest"),
			expectError: false,
			expectedLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &Database{
				store: v1beta1.Store{
					StoreSpec: v1beta1.StoreSpec{
						Apps: tt.initialApps,
					},
				},
				encoder: encoder,
			}

			err := db.Add(tt.addName, tt.addManifest)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedLen, len(db.store.Apps))
				found := false
				for _, app := range db.store.Apps {
					if app.Name == tt.addName {
						found = true
						assert.Equal(t, tt.addManifest, app.Manifest)
						assert.NotZero(t, app.InstalledAt)
					}
				}
				assert.True(t, found, "app should be found in database")
			}
		})
	}
}

func TestDatabase_Remove(t *testing.T) {
	encoder := createTestEncoder()
	tests := []struct {
		name        string
		initialApps []v1beta1.App
		removeName  string
		expectError bool
		errorMsg    string
		expectedLen int
	}{
		{
			name: "remove existing app",
			initialApps: []v1beta1.App{
				{Name: "app1", Manifest: []byte("manifest1")},
				{Name: "app2", Manifest: []byte("manifest2")},
			},
			removeName:  "app1",
			expectError: false,
			expectedLen: 1,
		},
		{
			name: "remove non-existent app",
			initialApps: []v1beta1.App{
				{Name: "app1", Manifest: []byte("manifest1")},
			},
			removeName:  "non-existent",
			expectError: true,
			errorMsg:    "no such pipeline found in local db store",
			expectedLen: 1,
		},
		{
			name:        "remove from empty database",
			initialApps: []v1beta1.App{},
			removeName:  "app1",
			expectError: true,
			errorMsg:    "no such pipeline found in local db store",
			expectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &Database{
				store: v1beta1.Store{
					StoreSpec: v1beta1.StoreSpec{
						Apps: tt.initialApps,
					},
				},
				encoder: encoder,
			}

			err := db.Remove(tt.removeName)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
					assert.Contains(t, err.Error(), tt.removeName)
				}
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectedLen, len(db.store.Apps))
		})
	}
}

func TestDatabase_Get(t *testing.T) {
	encoder := createTestEncoder()
	tests := []struct {
		name        string
		initialApps []v1beta1.App
		getName     string
		expectError bool
		errorMsg    string
		expected    []byte
	}{
		{
			name: "get existing app",
			initialApps: []v1beta1.App{
				{Name: "app1", Manifest: []byte("manifest1")},
				{Name: "app2", Manifest: []byte("manifest2")},
			},
			getName:     "app1",
			expectError: false,
			expected:    []byte("manifest1"),
		},
		{
			name: "get non-existent app",
			initialApps: []v1beta1.App{
				{Name: "app1", Manifest: []byte("manifest1")},
			},
			getName:     "non-existent",
			expectError: true,
			errorMsg:    "no such pipeline found in local db store",
		},
		{
			name: "get app with nil manifest",
			initialApps: []v1beta1.App{
				{Name: "app1", Manifest: nil},
			},
			getName:     "app1",
			expectError: true,
			errorMsg:    "no such pipeline found in local db store",
		},
		{
			name:        "get from empty database",
			initialApps: []v1beta1.App{},
			getName:     "app1",
			expectError: true,
			errorMsg:    "no such pipeline found in local db store",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &Database{
				store: v1beta1.Store{
					StoreSpec: v1beta1.StoreSpec{
						Apps: tt.initialApps,
					},
				},
				encoder: encoder,
			}

			result, err := db.Get(tt.getName)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
					assert.Contains(t, err.Error(), tt.getName)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestDatabase_Persist(t *testing.T) {
	encoder := createTestEncoder()
	tests := []struct {
		name        string
		store       v1beta1.Store
		encoder     kruntime.Serializer
		writer      io.Writer
		expectError bool
		errorMsg    string
	}{
		{
			name: "successful persist",
			store: v1beta1.Store{
				StoreSpec: v1beta1.StoreSpec{
					Apps: []v1beta1.App{
						{Name: "app1", Manifest: []byte("manifest1")},
					},
				},
			},
			encoder:     encoder,
			writer:      &bytes.Buffer{},
			expectError: false,
		},
		{
			name: "writer error",
			store: v1beta1.Store{
				StoreSpec: v1beta1.StoreSpec{
					Apps: []v1beta1.App{
						{Name: "app1", Manifest: []byte("manifest1")},
					},
				},
			},
			encoder: encoder,
			writer: &mockWriter{
				err: errors.New("write error"),
			},
			expectError: true,
			errorMsg:    "write error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &Database{
				store:   tt.store,
				encoder: tt.encoder,
			}

			err := db.Persist(tt.writer)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
func TestWithLocalDB(t *testing.T) {
	tests := []struct {
		name        string
		dbGetter    dbGetter
		ref         string
		expectError bool
		errorMsg    string
		expected    []byte
	}{
		{
			name: "successful get",
			dbGetter: &mockDBGetter{
				getFunc: func(name string) ([]byte, error) {
					return []byte("test data"), nil
				},
			},
			ref:         "test-ref",
			expectError: false,
			expected:    []byte("test data"),
		},
		{
			name: "get error",
			dbGetter: &mockDBGetter{
				getFunc: func(name string) ([]byte, error) {
					return nil, errors.New("not found")
				},
			},
			ref:         "test-ref",
			expectError: true,
			errorMsg:    "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := WithLocalDB(tt.dbGetter)
			require.NotNil(t, resolver)

			ctx := context.Background()
			reader, err := resolver(ctx, tt.ref)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				assert.Nil(t, reader)
			} else {
				require.NoError(t, err)
				require.NotNil(t, reader)
				data, err := io.ReadAll(reader)
				require.NoError(t, err)
				assert.Equal(t, tt.expected, data)
			}
		})
	}
}

func TestDatabase_Concurrency(t *testing.T) {
	encoder := createTestEncoder()
	db := &Database{
		store: v1beta1.Store{
			StoreSpec: v1beta1.StoreSpec{
				Apps: []v1beta1.App{},
			},
		},
		encoder: encoder,
	}

	// Test concurrent Add operations
	t.Run("concurrent add", func(t *testing.T) {
		const numGoroutines = 10
		done := make(chan bool, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer func() { done <- true }()
				err := db.Add("app-"+string(rune(id)), []byte("manifest"))
				assert.NoError(t, err)
			}(i)
		}

		for i := 0; i < numGoroutines; i++ {
			<-done
		}

		assert.Equal(t, numGoroutines, len(db.store.Apps))
	})

	// Test concurrent Get operations
	t.Run("concurrent get", func(t *testing.T) {
		const numGoroutines = 10
		done := make(chan bool, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer func() { done <- true }()
				_, err := db.Get("app-" + string(rune(id)))
				// Some may not exist, that's ok
				_ = err
			}(i)
		}

		for i := 0; i < numGoroutines; i++ {
			<-done
		}
	})
}
