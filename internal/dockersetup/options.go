package dockersetup

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/docker/cli/cli/config"
	"github.com/docker/docker/client"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/go-connections/sockets"
	"github.com/docker/go-connections/tlsconfig"
	"github.com/spf13/pflag"
)

const (
	// EnvEnableTLS is the name of the environment variable that can be used
	// to enable TLS for client connections. When set to a non-empty value, TLS
	// is enabled for API connections using TCP. For backward-compatibility, this
	// environment-variable can only be used to enable TLS, not to disable.
	//
	// Note that TLS is always enabled implicitly if the "--tls-verify" option
	// or "DOCKER_TLS_VERIFY" ([github.com/docker/docker/client.EnvTLSVerify])
	// env var is set to, which could be to either enable or disable TLS certification
	// validation. In both cases, TLS is enabled but, depending on the setting,
	// with verification disabled.
	EnvEnableTLS = "DOCKER_TLS"

	// DefaultCaFile is the default filename for the CA pem file
	DefaultCaFile = "ca.pem"
	// DefaultKeyFile is the default filename for the key pem file
	DefaultKeyFile = "key.pem"
	// DefaultCertFile is the default filename for the cert pem file
	DefaultCertFile = "cert.pem"
	// FlagTLSVerify is the flag name for the TLS verification option
	FlagTLSVerify = "tlsverify"
	// FormatHelp describes the --format flag behavior for list commands
	FormatHelp = `Format output using a custom template:
'table':            Print output in table format with column headers (default)
'table TEMPLATE':   Print output in table format using the given Go template
'json':             Print in JSON format
'TEMPLATE':         Print output using the given Go template.
Refer to https://docs.docker.com/go/formatting/ for more information about formatting output with templates`
	// InspectFormatHelp describes the --format flag behavior for inspect commands
	InspectFormatHelp = `Format output using a custom template:
'json':             Print in JSON format
'TEMPLATE':         Print output using the given Go template.
Refer to https://docs.docker.com/go/formatting/ for more information about formatting output with templates`
)

var (
	dockerCertPath  = os.Getenv(client.EnvOverrideCertPath)
	dockerTLSVerify = os.Getenv(client.EnvTLSVerify) != ""
	dockerTLS       = os.Getenv(EnvEnableTLS) != ""
)

// ClientOptions are the options used to configure the client cli.
type Options struct {
	Hosts      []string `env:"DOCKER_HOST"`
	TLS        bool
	TLSVerify  bool
	TLSOptions *tlsconfig.Options
	Context    string
	ConfigDir  string
}

// InstallFlags adds flags for the common options on the FlagSet
func (o *Options) BindFlags(flags *pflag.FlagSet) {
	configDir := config.Dir()
	if dockerCertPath == "" {
		dockerCertPath = configDir
	}

	flags.StringVar(&o.ConfigDir, "docker-config", configDir, "Location of client config files")
	flags.BoolVar(&o.TLS, "docker-tls", dockerTLS, "Use TLS; implied by --tlsverify")
	flags.BoolVar(&o.TLSVerify, FlagTLSVerify, dockerTLSVerify, "Use TLS and verify the remote")

	o.TLSOptions = &tlsconfig.Options{
		CAFile:   filepath.Join(dockerCertPath, DefaultCaFile),
		CertFile: filepath.Join(dockerCertPath, DefaultCertFile),
		KeyFile:  filepath.Join(dockerCertPath, DefaultKeyFile),
	}
	tlsOptions := o.TLSOptions
	flags.StringVar(&tlsOptions.CAFile, "docker-tlscacert", "", "Trust certs signed only by this CA")
	flags.StringVar(&tlsOptions.CertFile, "docker-tlscert", "", "Path to TLS certificate file")
	flags.StringVar(&tlsOptions.KeyFile, "docker-tlskey", "", "Path to TLS key file")

	flags.StringSliceVarP(&o.Hosts, "docker-host", "", []string{dockerclient.DefaultDockerHost}, "Daemon socket to connect to")
	flags.StringVarP(&o.Context, "docker-context", "", "",
		`Name of the context to use to connect to the daemon (overrides `+client.EnvOverrideHost+` env var and default context set with "docker context use")`)
}

// SetDefaultOptions sets default values for options after flag parsing is
// complete
func (o *Options) SetDefaultOptions(flags *pflag.FlagSet) {
	// Regardless of whether the user sets it to true or false, if they
	// specify --tlsverify at all then we need to turn on TLS
	// TLSVerify can be true even if not set due to DOCKER_TLS_VERIFY env var, so we need
	// to check that here as well
	if flags.Changed(FlagTLSVerify) || o.TLSVerify {
		o.TLS = true
	}

	if !o.TLS {
		o.TLSOptions = nil
	} else {
		tlsOptions := o.TLSOptions
		tlsOptions.InsecureSkipVerify = !o.TLSVerify

		// Reset CertFile and KeyFile to empty string if the user did not specify
		// the respective flags and the respective default files were not found.
		if !flags.Changed("tlscert") {
			if _, err := os.Stat(tlsOptions.CertFile); os.IsNotExist(err) {
				tlsOptions.CertFile = ""
			}
		}
		if !flags.Changed("tlskey") {
			if _, err := os.Stat(tlsOptions.KeyFile); os.IsNotExist(err) {
				tlsOptions.KeyFile = ""
			}
		}
	}
}

func (o *Options) Build() (*dockerclient.Client, error) {
	host := dockerclient.DefaultDockerHost
	if len(o.Hosts) > 0 {
		host = o.Hosts[0]
	}

	hostURL, err := dockerclient.ParseHostURL(host)
	if err != nil {
		return nil, err
	}

	client, err := o.defaultDockerHTTPClient(hostURL)
	if err != nil {
		return nil, err
	}

	var opts []dockerclient.Opt = []dockerclient.Opt{
		dockerclient.WithHTTPClient(client),
		dockerclient.WithUserAgent("rageta"),
		dockerclient.WithAPIVersionNegotiation(),
	}

	if o.TLS {
		opts = append(opts, dockerclient.WithTLSClientConfig(o.TLSOptions.CAFile, o.TLSOptions.CertFile, o.TLSOptions.KeyFile))
	}

	return dockerclient.NewClientWithOpts(opts...)
}

func (o *Options) defaultDockerHTTPClient(hostURL *url.URL) (*http.Client, error) {
	transport := &http.Transport{}

	if o.TLS || o.TLSVerify {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: o.TLSVerify,
		}
	}

	err := sockets.ConfigureTransport(transport, hostURL.Scheme, hostURL.Host)
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Transport:     transport,
		CheckRedirect: dockerclient.CheckRedirect,
	}, nil
}
