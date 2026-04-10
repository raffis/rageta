package buildkitsetup

import (
	"context"
	"net/url"
	"os"
	"path/filepath"

	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/util/tracing/delegated"
	"github.com/pkg/errors"
	"github.com/raffis/rageta/internal/setup/flagset"
	"github.com/spf13/pflag"
	"go.opentelemetry.io/otel/trace"
)

type Options struct {
	Address       string `env:"BUILDKIT_HOST"`
	TLSServerName string
	TLSCACert     string
	TLSCert       string
	TLSKey        string
	TLSDir        string
	TLSVerify     bool
	Wait          bool
}

// BindFlags registers flags for connecting to buildkitd.
func (o *Options) BindFlags(flags flagset.Interface) {
	flags.StringVar(&o.Address, "buildkit-addr", o.Address, "BuildKitd address")
	flags.StringVar(&o.TLSServerName, "buildkit-tlsservername", "", "Server name for TLS certificate validation (default: hostname from address)")
	flags.StringVar(&o.TLSCACert, "buildkit-tlscacert", "", "CA certificate file for validating the server")
	flags.StringVar(&o.TLSCert, "buildkit-tlscert", "", "Client certificate file for mTLS")
	flags.StringVar(&o.TLSKey, "buildkit-tlskey", "", "Client key file for mTLS")
	flags.StringVar(&o.TLSDir, "buildkit-tlsdir", "", "Directory with ca.pem|ca.crt, cert.pem|tls.crt, key.pem|tls.key (mutually exclusive with individual TLS file flags)")
	flags.BoolVar(&o.Wait, "buildkit-wait", false, "Block until the BuildKit backend accepts RPCs")
	flags.BoolVar(&o.TLSVerify, "buildkit-tlsverify", false, "Verify server TLS using the system CA pool (sets server name from address; mutually exclusive with custom CA)")
}

// SetDefaultOptions runs after flag parsing: TLS directory resolution, validation, and defaults.
func (o *Options) SetDefaultOptions(flags *pflag.FlagSet) error {
	if o.TLSDir != "" {
		if flags.Changed("buildkit-tlscacert") || flags.Changed("buildkit-tlscert") || flags.Changed("buildkit-tlskey") {
			return errors.New("cannot use --buildkit-tlsdir together with --buildkit-tlscacert, --buildkit-tlscert, or --buildkit-tlskey")
		}
		ca, cert, key, err := resolveTLSFilesFromDir(o.TLSDir)
		if err != nil {
			return err
		}
		o.TLSCACert, o.TLSCert, o.TLSKey = ca, cert, key
	}

	if o.TLSVerify {
		if o.TLSCACert != "" {
			return errors.New("cannot combine --buildkit-tlsverify with a custom CA (--buildkit-tlscacert or --buildkit-tlsdir)")
		}
		if o.Address == "" {
			return errors.New("--buildkit-tlsverify requires --buildkit-host so the server name can be determined")
		}
		u, err := url.Parse(o.Address)
		if err != nil {
			return errors.Wrap(err, "parse buildkit address for TLS")
		}
		if o.TLSServerName == "" {
			o.TLSServerName = u.Hostname()
		}
	}

	return nil
}

func (o *Options) Build(ctx context.Context) (*client.Client, error) {
	serverName := o.TLSServerName
	if serverName == "" && o.Address != "" {
		if u, err := url.Parse(o.Address); err == nil {
			serverName = u.Hostname()
		}
	}

	var opts []client.ClientOpt
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		opts = append(opts,
			client.WithTracerProvider(span.TracerProvider()),
			client.WithTracerDelegate(delegated.DefaultExporter),
		)
	}

	if o.TLSVerify {
		opts = append(opts, client.WithServerConfigSystem(serverName))
	} else if o.TLSCACert != "" {
		opts = append(opts, client.WithServerConfig(serverName, o.TLSCACert))
	}

	if o.TLSCert != "" || o.TLSKey != "" {
		opts = append(opts, client.WithCredentials(o.TLSCert, o.TLSKey))
	}

	cl, err := client.New(ctx, o.Address, opts...)
	if err != nil {
		return nil, err
	}

	if o.Wait {
		if err := cl.Wait(ctx); err != nil {
			_ = cl.Close()
			return nil, err
		}
	}

	return cl, nil
}

// resolveTLSFilesFromDir scans a TLS directory for known cert/key filenames (same rules as buildctl).
func resolveTLSFilesFromDir(tlsDir string) (caCert, cert, key string, err error) {
	oneOf := func(either, or string) (string, error) {
		for _, name := range []string{either, or} {
			fpath := filepath.Join(tlsDir, name)
			if _, err := os.Stat(fpath); err == nil {
				return fpath, nil
			} else if !os.IsNotExist(err) {
				return "", err
			}
		}
		return "", errors.Errorf("directory did not contain one of the needed files: %s or %s", either, or)
	}

	if caCert, err = oneOf("ca.pem", "ca.crt"); err != nil {
		return "", "", "", err
	}
	if cert, err = oneOf("cert.pem", "tls.crt"); err != nil {
		return "", "", "", err
	}
	if key, err = oneOf("key.pem", "tls.key"); err != nil {
		return "", "", "", err
	}
	return caCert, cert, key, nil
}
