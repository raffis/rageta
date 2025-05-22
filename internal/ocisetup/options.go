package ocisetup

import (
	"context"
	"fmt"
	"time"

	authutils "github.com/fluxcd/pkg/auth/utils"
	"github.com/fluxcd/pkg/oci"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/spf13/pflag"
)

type Options struct {
	Creds    string
	Provider string
	URL      string
	Timeout  time.Duration
}

func DefaultOptions() *Options {
	return &Options{
		Provider: "generic",
	}
}

// BindFlags will parse the given pflag.FlagSet
func (o *Options) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.Creds, "creds", "", "credentials for OCI registry in the format <username>[:<password>] if --provider is generic")
	fs.StringVar(&o.Provider, "provider", "", "OCI provider type")
}

func (o *Options) Build(ctx context.Context) (*oci.Client, error) {
	ref, err := name.ParseReference(o.URL)
	if err != nil {
		return nil, err
	}

	var auth authn.Authenticator
	opts := oci.DefaultOptions()

	if o.Provider == "generic" && o.Creds != "" {
		auth, err = oci.GetAuthFromCredentials(o.Creds)
		if err != nil {
			return nil, fmt.Errorf("could not login with credentials: %w", err)
		}
		opts = append(opts, crane.WithAuth(auth))
	}

	if o.Provider != "generic" {
		auth, err = authutils.GetArtifactRegistryCredentials(ctx, o.Provider, o.URL)
		if err != nil {
			return nil, fmt.Errorf("error during login with provider: %w", err)
		}

		opts = append(opts, crane.WithAuth(auth))
	}

	if o.Timeout != 0 {
		backoff := remote.Backoff{
			Duration: 1.0 * time.Second,
			Factor:   3,
			Jitter:   0.1,
			// timeout happens when the cap is exceeded or number of steps is reached
			// 10 steps is big enough that most reasonable cap(under 30min) will be exceeded before
			// the number of steps are completed.
			Steps: 10,
			Cap:   o.Timeout,
		}

		if auth == nil {
			auth, err = authn.DefaultKeychain.Resolve(ref.Context())
			if err != nil {
				return nil, err
			}
		}

		transportOpts, err := oci.WithRetryTransport(ctx, ref, auth, backoff, []string{ref.Context().Scope(transport.PushScope)})
		if err != nil {
			return nil, fmt.Errorf("error setting up transport: %w", err)
		}
		opts = append(opts, transportOpts, oci.WithRetryBackOff(backoff))
	}

	return oci.NewClient(opts), nil
}
