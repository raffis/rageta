package buildkitsetup

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "github.com/moby/buildkit/client/connhelper/dockercontainer"
	_ "github.com/moby/buildkit/client/connhelper/kubepod"
	_ "github.com/moby/buildkit/client/connhelper/nerdctlcontainer"
	_ "github.com/moby/buildkit/client/connhelper/npipe"
	_ "github.com/moby/buildkit/client/connhelper/podmancontainer"
	_ "github.com/moby/buildkit/client/connhelper/ssh"

	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/util/tracing/delegated"
	"github.com/pkg/errors"
	"github.com/raffis/rageta/internal/setup/flagset"
	"github.com/spf13/pflag"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

// maxGRPCMessageSize raises the default 16MiB gRPC message limit so local
// artifacts larger than that (e.g. built binaries) can be transferred over
// the BuildKit session without hitting ResourceExhausted.
const maxGRPCMessageSize = 512 * 1024 * 1024

type Options struct {
	Host           string `env:"BUILDKIT_HOST"`
	TLSServerName  string
	TLSCACert      string
	TLSCert        string
	TLSKey         string
	TLSDir         string
	TLSVerify      bool
	Wait           bool
	ConnectTimeout time.Duration
}

// BindFlags registers flags for connecting to buildkitd.
func (o *Options) BindFlags(flags flagset.Interface) {
	flags.StringVar(&o.Host, "buildkit-host", o.Host, "BuildKitd host")
	flags.StringVar(&o.TLSServerName, "buildkit-tlsservername", "", "Server name for TLS certificate validation (default: hostname from address)")
	flags.StringVar(&o.TLSCACert, "buildkit-tlscacert", "", "CA certificate file for validating the server")
	flags.StringVar(&o.TLSCert, "buildkit-tlscert", "", "Client certificate file for mTLS")
	flags.StringVar(&o.TLSKey, "buildkit-tlskey", "", "Client key file for mTLS")
	flags.StringVar(&o.TLSDir, "buildkit-tlsdir", "", "Directory with ca.pem|ca.crt, cert.pem|tls.crt, key.pem|tls.key (mutually exclusive with individual TLS file flags)")
	flags.BoolVar(&o.Wait, "buildkit-wait", o.Wait, "Block until the BuildKit backend accepts RPCs")
	flags.BoolVar(&o.TLSVerify, "buildkit-tlsverify", o.TLSVerify, "Verify server TLS using the system CA pool (sets server name from address; mutually exclusive with custom CA)")
	flags.DurationVar(&o.ConnectTimeout, "buildkit-connect-timeout", o.ConnectTimeout, "Timeout for connecting to buildkitd")
}

func NewOptions() Options {
	return Options{
		Host:           "docker-container://rageta-buildkitd",
		Wait:           true,
		ConnectTimeout: 60 * time.Second,
	}
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
		if o.Host == "" {
			return errors.New("--buildkit-tlsverify requires --buildkit-host so the server name can be determined")
		}
		u, err := url.Parse(o.Host)
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
	if serverName == "" && o.Host != "" {
		if u, err := url.Parse(o.Host); err == nil {
			serverName = u.Hostname()
		}
	}

	opts := []client.ClientOpt{
		client.WithGRPCDialOption(grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxGRPCMessageSize),
			grpc.MaxCallSendMsgSize(maxGRPCMessageSize),
		)),
	}
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

	bkClient, err := client.New(ctx, o.Host, opts...)
	if err != nil {
		return nil, err
	}

	if o.Wait {
		waitCtx, cancel := context.WithTimeout(ctx, o.ConnectTimeout)
		defer cancel()

		if bkClient.Wait(waitCtx) != nil {
			_ = bkClient.Close()
			return nil, fmt.Errorf("timed out waiting for buildkitd: %w", err)
		}
	}

	return bkClient, nil
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
