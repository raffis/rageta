package run

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/gofrs/flock"
	"github.com/raffis/rageta/internal/provider"
	cruntime "github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/internal/setup/flagset"
	"github.com/raffis/rageta/internal/setup/ocisetup"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"github.com/spf13/pflag"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
)

type ProviderOptions struct {
	OCI    *ocisetup.Options
	DBPath string
}

func (s *ProviderOptions) BindFlags(flags flagset.Interface) {
	ociFlags := pflag.NewFlagSet("oci", pflag.ExitOnError)
	s.OCI.BindFlags(ociFlags)
	flags.AddFlagSet(ociFlags)
}

func (s ProviderOptions) Build() Step {
	return &Provider{opts: s}
}

func NewProviderOptions() ProviderOptions {
	return ProviderOptions{
		OCI: ocisetup.DefaultOptions(),
	}
}

type Provider struct {
	opts ProviderOptions
}

type ProviderContext struct {
	Provider provider.Interface
	Pipeline v1beta1.Pipeline
	Args     []string
	Ref      string
}

func (s *Provider) Run(rc *RunContext, next Next) error {
	store, persistDB := CreateProvider(
		rc.ImagePolicy.PullPolicy,
		s.opts.DBPath,
		s.opts.OCI,
	)
	rc.Provider.Provider = store
	defer func() {
		if err := persistDB(); err != nil {
			rc.Logging.Logger.V(1).Error(err, "failed to persist database")
		}
	}()

	var ref string
	if len(rc.Provider.Args) > 0 && !strings.HasPrefix(rc.Provider.Args[0], "--") {
		ref = rc.Provider.Args[0]
		rc.Provider.Ref = ref
	}

	rc.Logging.Logger.V(3).Info("resolve pipeline reference", "source", ref)
	pipeline, err := store.Resolve(rc.Context, ref)
	if err != nil {
		return err
	}

	rc.Provider.Pipeline = pipeline
	return next(rc)
}

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
			return nil, fmt.Errorf("failed to open local database: %w", err)
		}
		return provider.WithLocalDB(localDB)(ctx, ref)
	}

	ociProviderWrapper := func(ctx context.Context, ref string) (io.Reader, error) {
		ociOptions.URL = ref
		ociClient, err := ociOptions.Build(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to build oci client: %w", err)
		}

		r, err := provider.WithOCI(ociClient)(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("failed to pull image from oci registry: %w", err)
		}

		localDB, err := openDB()
		if err != nil {
			return r, fmt.Errorf("failed to open local database: %w", err)
		}

		manifest, err := io.ReadAll(r)
		if err != nil {
			return r, fmt.Errorf("failed to read manifest: %w", err)
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
