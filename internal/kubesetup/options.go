package kubesetup

import (
	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	flagClusterName        = "kube-cluster"
	flagAuthInfoName       = "kube-user"
	flagContext            = "kube-context"
	flagNamespace          = "kube-namespace"
	flagAPIServer          = "kube-server"
	flagTLSServerName      = "kube-tls-server-name"
	flagInsecure           = "kube-insecure-skip-tls-verify"
	flagCertFile           = "kube-client-certificate"
	flagKeyFile            = "kube-client-key"
	flagCAFile             = "kube-certificate-authority"
	flagBearerToken        = "kube-token"
	flagImpersonate        = "kube-as"
	flagImpersonateUID     = "kube-as-uid"
	flagImpersonateGroup   = "kube-as-group"
	flagUsername           = "kube-username"
	flagPassword           = "kube-password"
	flagTimeout            = "kube-request-timeout"
	flagCacheDir           = "kube-cache-dir"
	flagDisableCompression = "kube-disable-compression"
)

// Options are the options used to configure the client cli.
type Options struct {
	*genericclioptions.ConfigFlags
}

// BindFlags adds flags for the common options on the FlagSet
func (o *Options) BindFlags(flags *pflag.FlagSet) {
	if o.KubeConfig != nil {
		flags.StringVar(o.KubeConfig, "kubeconfig", *o.KubeConfig, "Path to the kubeconfig file to use for CLI requests.")
	}
	if o.CacheDir != nil {
		flags.StringVar(o.CacheDir, flagCacheDir, *o.CacheDir, "Default cache directory")
	}

	if o.CertFile != nil {
		flags.StringVar(o.CertFile, flagCertFile, *o.CertFile, "Path to a client certificate file for TLS")
	}
	if o.KeyFile != nil {
		flags.StringVar(o.KeyFile, flagKeyFile, *o.KeyFile, "Path to a client key file for TLS")
	}
	if o.BearerToken != nil {
		flags.StringVar(o.BearerToken, flagBearerToken, *o.BearerToken, "Bearer token for authentication to the API server")
	}
	if o.Impersonate != nil {
		flags.StringVar(o.Impersonate, flagImpersonate, *o.Impersonate, "Username to impersonate for the operation. User could be a regular user or a service account in a namespace.")
	}
	if o.ImpersonateUID != nil {
		flags.StringVar(o.ImpersonateUID, flagImpersonateUID, *o.ImpersonateUID, "UID to impersonate for the operation.")
	}
	if o.ImpersonateGroup != nil {
		flags.StringArrayVar(o.ImpersonateGroup, flagImpersonateGroup, *o.ImpersonateGroup, "Group to impersonate for the operation, this flag can be repeated to specify multiple groups.")
	}
	if o.Username != nil {
		flags.StringVar(o.Username, flagUsername, *o.Username, "Username for basic authentication to the API server")
	}
	if o.Password != nil {
		flags.StringVar(o.Password, flagPassword, *o.Password, "Password for basic authentication to the API server")
	}
	if o.ClusterName != nil {
		flags.StringVar(o.ClusterName, flagClusterName, *o.ClusterName, "The name of the kubeconfig cluster to use")
	}
	if o.AuthInfoName != nil {
		flags.StringVar(o.AuthInfoName, flagAuthInfoName, *o.AuthInfoName, "The name of the kubeconfig user to use")
	}
	if o.Namespace != nil {
		flags.StringVarP(o.Namespace, flagNamespace, "n", *o.Namespace, "If present, the namespace scope for this CLI request")
	}
	if o.Context != nil {
		flags.StringVar(o.Context, flagContext, *o.Context, "The name of the kubeconfig context to use")
	}

	if o.APIServer != nil {
		flags.StringVarP(o.APIServer, flagAPIServer, "s", *o.APIServer, "The address and port of the Kubernetes API server")
	}
	if o.TLSServerName != nil {
		flags.StringVar(o.TLSServerName, flagTLSServerName, *o.TLSServerName, "Server name to use for server certificate validation. If it is not provided, the hostname used to contact the server is used")
	}
	if o.Insecure != nil {
		flags.BoolVar(o.Insecure, flagInsecure, *o.Insecure, "If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure")
	}
	if o.CAFile != nil {
		flags.StringVar(o.CAFile, flagCAFile, *o.CAFile, "Path to a cert file for the certificate authority")
	}
	if o.Timeout != nil {
		flags.StringVar(o.Timeout, flagTimeout, *o.Timeout, "The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests.")
	}
	if o.DisableCompression != nil {
		flags.BoolVar(o.DisableCompression, flagDisableCompression, *o.DisableCompression, "If true, opt-out of response compression for all requests to the server")
	}
}

func DefaultOptions() *Options {
	return &Options{
		genericclioptions.NewConfigFlags(false),
	}
}
