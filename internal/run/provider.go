package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/gofrs/flock"
	"github.com/raffis/rageta/internal/ocisetup"
	"github.com/raffis/rageta/internal/provider"
	cruntime "github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
)

type ProviderStep struct {
	ociOpts *ocisetup.Options
}

func WithProvider(ociOpts *ocisetup.Options) *ProviderStep {
	return &ProviderStep{ociOpts: ociOpts}
}

func (s *ProviderStep) Run(rc *RunContext, next Next) error {
	store, persistDB := CreateProvider(
		rc.ImagePullPolicy,
		rc.Input.DBPath,
		s.ociOpts,
	)
	rc.Store = store
	rc.PersistDB = persistDB

	rc.Logger.V(3).Info("resolve pipeline reference", "source", rc.Input.Ref)
	command, err := store.Resolve(rc.Ctx, rc.Input.Ref)
	if err != nil {
		return err
	}
	rc.Command = command
	return next(rc)
}

// CreateProvider builds a pipeline provider and a persist function.
func CreateProvider(
	imagePullPolicy cruntime.PullImagePolicy,
	dbPath string,
	ociOptions *ocisetup.Options,
) (provider.Interface, func() error) {
	scheme := kruntime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)

	factory := serializer.NewCodecFactory(scheme)
	decoder := factory.UniversalDeserializer()
	encoder := json.NewYAMLSerializer(json.DefaultMetaFactory, scheme, scheme)
	var localDB *provider.Database
	var mu sync.Mutex

	openDB := func() (*provider.Database, error) {
		mu.Lock()
		defer mu.Unlock()
		if localDB == nil {
			dbFile, err := os.OpenFile(dbPath, os.O_RDONLY|os.O_CREATE, 0644)
			if err != nil {
				return nil, err
			}
			localDB, err = provider.OpenDatabase(dbFile, decoder, encoder)
			if err != nil {
				return nil, err
			}
		}
		return localDB, nil
	}

	localDBProviderWrapper := func(ctx context.Context, ref string) (io.Reader, error) {
		localDB, err := openDB()
		if err != nil {
			return nil, err
		}
		return provider.WithLocalDB(localDB)(ctx, ref)
	}

	ociProviderWrapper := func(ctx context.Context, ref string) (io.Reader, error) {
		ociOptions.URL = ref
		ociClient, err := ociOptions.Build(ctx)
		if err != nil {
			return nil, err
		}
		r, err := provider.WithOCI(ociClient)(ctx, ref)
		if err != nil {
			return nil, err
		}
		localDB, err := openDB()
		if err != nil {
			return r, err
		}
		manifest, err := io.ReadAll(r)
		if err != nil {
			return r, err
		}
		r = bytes.NewReader(manifest)
		err = localDB.Add(ref, manifest)
		return r, err
	}

	providers := []provider.Resolver{
		provider.WithFile(),
		provider.WithRagetafile(),
	}
	if imagePullPolicy == cruntime.PullImagePolicyAlways {
		providers = append(providers, ociProviderWrapper, localDBProviderWrapper)
	} else {
		providers = append(providers, localDBProviderWrapper, ociProviderWrapper)
	}

	return provider.New(decoder, providers...), func() error {
		if localDB == nil {
			return nil
		}
		return PersistDatabase(dbPath, localDB)
	}
}

// PersistDatabase writes the in-memory database to the given path.
func PersistDatabase(dbPath string, db *provider.Database) error {
	scheme := kruntime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	factory := serializer.NewCodecFactory(scheme)
	decoder := factory.UniversalDeserializer()
	encoder := json.NewYAMLSerializer(json.DefaultMetaFactory, scheme, scheme)

	fileLock := flock.New(fmt.Sprintf("%s.lock", dbPath))
	locked, err := fileLock.TryLock()
	if !locked || err != nil {
		return fmt.Errorf("failed to lock database: %w", err)
	}
	defer func() { _ = fileLock.Unlock() }()

	dbFile, err := os.OpenFile(dbPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer func() { _ = dbFile.Close() }()

	localDB, err := provider.OpenDatabase(dbFile, decoder, encoder)
	if err != nil {
		return err
	}
	if err := localDB.Merge(db); err != nil {
		return err
	}
	return localDB.Persist(dbFile)
}
