package ocisetup

import (
	"context"
	"fmt"
	"time"

	"github.com/fluxcd/pkg/oci"
	"github.com/fluxcd/pkg/oci/auth/login"
	"github.com/fluxcd/pkg/oci/client"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/spf13/pflag"
)

type Options struct {
	Creds   string
	URL     string
	Timeout time.Duration
}

// BindFlags will parse the given pflag.FlagSet
func (o *Options) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.Creds, "creds", "", "credentials for OCI registry in the format <username>[:<password>] if --provider is generic")
	//pushCmd.Flags().Var(&pushArtifactArgs.provider, "provider", pushArtifactArgs.provider)
}

func (o *Options) Build(ctx context.Context) (*client.Client, error) {

	/*url, err := client.ParseArtifactURL(o.URL)
	if err != nil {
		return nil, err
	}*/

	ref, err := name.ParseReference(o.URL)
	if err != nil {
		return nil, err
	}

	var auth authn.Authenticator
	opts := client.DefaultOptions()
	/*
		if pushArtifactArgs.provider.String() == sourcev1.GenericOCIProvider && pushArtifactArgs.creds != "" {
			logger.Info("logging in to registry with credentials")
			auth, err = client.GetAuthFromCredentials(pushArtifactArgs.creds)
			if err != nil {
				return fmt.Errorf("could not login with credentials: %w", err)
			}
			opts = append(opts, crane.WithAuth(auth))
		}

		if pushArtifactArgs.provider.String() != sourcev1.GenericOCIProvider {
			logger.Info("logging in to registry with provider credentials")
			ociProvider, err := pushArtifactArgs.provider.ToOCIProvider()
			if err != nil {
				return fmt.Errorf("provider not supported: %w", err)
			}

			auth, err = login.NewManager().Login(ctx, url, ref, getProviderLoginOption(ociProvider))
			if err != nil {
				return fmt.Errorf("error during login with provider: %w", err)
			}
			opts = append(opts, crane.WithAuth(auth))
		}*/

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

		transportOpts, err := client.WithRetryTransport(ctx, ref, auth, backoff, []string{ref.Context().Scope(transport.PushScope)})
		if err != nil {
			return nil, fmt.Errorf("error setting up transport: %w", err)
		}
		opts = append(opts, transportOpts, client.WithRetryBackOff(backoff))
	}

	return client.NewClient(opts), nil
}

func getProviderLoginOption(provider oci.Provider) login.ProviderOptions {
	var opts login.ProviderOptions
	switch provider {
	case oci.ProviderAzure:
		opts.AzureAutoLogin = true
	case oci.ProviderAWS:
		opts.AwsAutoLogin = true
	case oci.ProviderGCP:
		opts.GcpAutoLogin = true
	}
	return opts
}
