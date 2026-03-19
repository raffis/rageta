package run

import (
	"context"
	"io"

	"github.com/spf13/pflag"
)

type Step interface {
	Run(rc *RunContext, next Next) error
}

type Next func(rc *RunContext) error

type Runner struct {
	steps []Step
}

func Builder(steps ...Step) *Runner {
	result := &Runner{}
	result.steps = steps
	return result
}

func (r *Runner) Run(ctx context.Context, args []string, _ io.Reader, stdout, stderr io.Writer) (rc *RunContext, err error) {
	rc = NewContext()
	rc.Context = ctx
	rc.Output.Stdout = stdout
	rc.Output.Stderr = stderr
	rc.Provider.Args = args

	noop := func(rc *RunContext) error {
		return nil
	}

	chain := noop

	for i := len(r.steps) - 1; i >= 0; i-- {
		step := r.steps[i]
		next := chain
		chain = func(rc *RunContext) error {
			return step.Run(rc, next)
		}
	}
	err = chain(rc)
	return rc, err
}

type Options struct {
	EnvOptions              EnvsOptions
	SecretOptions           SecretsOptions
	ImagePolicyOptions      ImagePolicyOptions
	OutputOptions           OutputOptions
	ReportOptions           ReportOptions
	TemplateOptions         TemplateOptions
	TeardownOptions         TeardownOptions
	EventsOptions           EventsOptions
	ForkOptions             ForkOptions
	ContainerRuntimeOptions ContainerRuntimeOptions
	LifecycleOptions        LifecycleOptions
	OtelOptions             OtelOptions
	LoggingOptions          LoggingOptions
	ProviderOptions         ProviderOptions
	ExecuteOptions          ExecuteOptions
	CELOptions              CELOptions
	PipelineOptions         PipelineOptions
	InputsOptions           InputsOptions
	ContextDirOptions       ContextDirOptions
	TagsOptions             TagsOptions
	ErrorOptions            ErrorOptions
}

func (s *Options) BindFlags(flags *pflag.FlagSet) {
	s.ImagePolicyOptions.BindFlags(flags)
	s.OutputOptions.BindFlags(flags)
	s.ReportOptions.BindFlags(flags)
	s.TemplateOptions.BindFlags(flags)
	s.TeardownOptions.BindFlags(flags)
	s.EventsOptions.BindFlags(flags)
	s.ForkOptions.BindFlags(flags)
	s.ContainerRuntimeOptions.BindFlags(flags)
	s.LifecycleOptions.BindFlags(flags)
	s.OtelOptions.BindFlags(flags)
	s.LoggingOptions.BindFlags(flags)
	s.TagsOptions.BindFlags(flags)
	s.EnvOptions.BindFlags(flags)
	s.SecretOptions.BindFlags(flags)
	s.ProviderOptions.BindFlags(flags)
	s.ExecuteOptions.BindFlags(flags)
	s.InputsOptions.BindFlags(flags)
}

func DefaultOptions() Options {
	return Options{
		ContainerRuntimeOptions: NewContainerRuntimeOptions(),
		ImagePolicyOptions:      NewImagePolicyOptions(),
		OutputOptions:           NewOutputOptions(),
		LoggingOptions:          NewLoggingOptions(),
		ProviderOptions:         NewProviderOptions(),
		EventsOptions:           NewEventsOptions(),
		ReportOptions:           NewReportOptions(),
	}
}

func (o Options) Build() *Runner {
	return Builder(
		o.ErrorOptions.Build(),
		o.ContextDirOptions.Build(),
		o.ReportOptions.Build(),
		o.EnvOptions.Build(),
		o.SecretOptions.Build(),
		o.OutputOptions.Build(),
		o.LoggingOptions.Build(),
		o.ForkOptions.Build(),
		o.ImagePolicyOptions.Build(),
		o.TemplateOptions.Build(),
		o.TeardownOptions.Build(),
		o.EventsOptions.Build(),
		o.CELOptions.Build(),
		o.TagsOptions.Build(),
		o.ContainerRuntimeOptions.Build(),
		o.LifecycleOptions.Build(),
		o.OtelOptions.Build(),
		o.ProviderOptions.Build(),
		o.PipelineOptions.Build(),
		o.InputsOptions.Build(),
		o.ExecuteOptions.Build(),
	)
}
