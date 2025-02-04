package ocisetup

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/fluxcd/pkg/oci"
	"github.com/fluxcd/pkg/oci/auth/login"
	"github.com/fluxcd/pkg/oci/client"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/spf13/pflag"
)

type Options struct {
	Creds    string
	Provider SourceOCIProvider
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
	fs.Var(&o.Provider, "provider", "OCI provider type")
}

func (o *Options) Build(ctx context.Context) (*client.Client, error) {
	ref, err := name.ParseReference(o.URL)
	if err != nil {
		return nil, err
	}

	var auth authn.Authenticator
	opts := client.DefaultOptions()

	if o.Provider == "generic" && o.Creds != "" {
		auth, err = client.GetAuthFromCredentials(o.Creds)
		if err != nil {
			return nil, fmt.Errorf("could not login with credentials: %w", err)
		}
		opts = append(opts, crane.WithAuth(auth))
	}

	if o.Provider != "generic" {
		ociProvider, err := o.Provider.ToOCIProvider()
		if err != nil {
			return nil, fmt.Errorf("provider not supported: %w", err)
		}

		auth, err = login.NewManager().Login(ctx, o.URL, ref, getProviderLoginOption(ociProvider))
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

var supportedSourceOCIProviders = []string{
	"generic",
	"aws",
	"azure",
	"gcr",
}

var sourceOCIProvidersToOCIProvider = map[string]oci.Provider{
	"generic": oci.ProviderGeneric,
	"aws":     oci.ProviderAWS,
	"azure":   oci.ProviderAzure,
	"gcr":     oci.ProviderGCP,
}

type SourceOCIProvider string

func (p *SourceOCIProvider) String() string {
	return string(*p)
}

func (p *SourceOCIProvider) Set(str string) error {
	if strings.TrimSpace(str) == "" {
		return fmt.Errorf("no source OCI provider given, please specify %s",
			p.Description())
	}
	if !slices.Contains(supportedSourceOCIProviders, str) {
		return fmt.Errorf("source OCI provider '%s' is not supported, must be one of: %v",
			str, strings.Join(supportedSourceOCIProviders, ", "))
	}
	*p = SourceOCIProvider(str)
	return nil
}

func (p *SourceOCIProvider) Type() string {
	return "sourceOCIProvider"
}

func (p *SourceOCIProvider) Description() string {
	return fmt.Sprintf(
		"the OCI provider name, available options are: (%s)",
		strings.Join(supportedSourceOCIProviders, ", "),
	)
}

func (p *SourceOCIProvider) ToOCIProvider() (oci.Provider, error) {
	value, ok := sourceOCIProvidersToOCIProvider[p.String()]
	if !ok {
		return 0, fmt.Errorf("no mapping between source OCI provider %s and OCI provider", p.String())
	}

	return value, nil
}
