package run

import (
	"context"
	"io"

	"github.com/raffis/rageta/internal/setup/flagset"
	"github.com/spf13/pflag"
)

type Task interface {
	Run(rc *RunContext, next Next) error
}

type Next func(rc *RunContext) error

type Runner struct {
	steps []Task
}

func Builder(steps ...Task) *Runner {
	result := &Runner{}
	result.steps = steps
	return result
}

func (r *Runner) Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) (rc *RunContext, err error) {
	rc = NewContext()
	rc.Context = ctx
	rc.Display.Stdout = stdout
	rc.Display.Stderr = stderr
	rc.Display.Stdin = stdin
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
	DisplayOptions          DisplayOptions
	ReportOptions           ReportOptions
	TeardownOptions         TeardownOptions
	EventsOptions           EventsOptions
	ForkOptions             ForkOptions
	ContainerRuntimeOptions ContainerRuntimeOptions
	BuildkitOptions         BuildkitOptions
	LifecycleOptions        LifecycleOptions
	OtelOptions             OtelOptions
	LoggingOptions          LoggingOptions
	ProviderOptions         ProviderOptions
	ExecuteOptions          ExecuteOptions
	CELOptions              CELOptions
	PipelineOptions         PipelineOptions
	InputsOptions           InputsOptions
	LabelsOptions           LabelsOptions
	SummaryOptions          SummaryOptions
	InteractiveOptions      InteractiveOptions
	ExitCodeOptions         ExitCodeOptions
}

func (s *Options) BindFlags(flags flagset.Interface) {
	pipelineFlags := pflag.NewFlagSet("Pipeline", pflag.ExitOnError)
	s.ImagePolicyOptions.BindFlags(flags)
	s.DisplayOptions.BindFlags(flags)
	s.ReportOptions.BindFlags(flags)
	s.TeardownOptions.BindFlags(flags)
	s.EventsOptions.BindFlags(flags)
	s.ForkOptions.BindFlags(flags)
	s.BuildkitOptions.BindFlags(flags)
	s.ContainerRuntimeOptions.BindFlags(flags)
	s.OtelOptions.BindFlags(flags)
	s.LoggingOptions.BindFlags(flags)
	s.ProviderOptions.BindFlags(flags)
	s.InteractiveOptions.BindFlags(flags)
	s.ExitCodeOptions.BindFlags(flags)
	s.SummaryOptions.BindFlags(flags)
	s.LabelsOptions.BindFlags(pipelineFlags)
	s.EnvOptions.BindFlags(pipelineFlags)
	s.SecretOptions.BindFlags(pipelineFlags)
	s.ExecuteOptions.BindFlags(pipelineFlags)
	s.InputsOptions.BindFlags(pipelineFlags)
	s.PipelineOptions.BindFlags(pipelineFlags)
	flags.AddFlagSet(pipelineFlags)
}

func DefaultOptions() Options {
	return Options{
		ContainerRuntimeOptions: NewContainerRuntimeOptions(),
		ImagePolicyOptions:      NewImagePolicyOptions(),
		DisplayOptions:          NewDisplayOptions(),
		LoggingOptions:          NewLoggingOptions(),
		ProviderOptions:         NewProviderOptions(),
		EventsOptions:           NewEventsOptions(),
		ReportOptions:           NewReportOptions(),
		BuildkitOptions:         NewBuildkitOptions(),
		InteractiveOptions:      NewInteractiveOptions(),
	}
}

func (o Options) Build() *Runner {
	return Builder(
		o.ExitCodeOptions.Build(),
		o.TeardownOptions.Build(),
		o.SecretOptions.Build(),
		o.ReportOptions.Build(),
		o.OtelOptions.Build(),
		o.LoggingOptions.Build(),
		o.EnvOptions.Build(),
		o.ImagePolicyOptions.Build(),
		o.EventsOptions.Build(),
		o.CELOptions.Build(),
		o.LabelsOptions.Build(),
		o.ContainerRuntimeOptions.Build(),
		o.ForkOptions.Build(),
		o.LifecycleOptions.Build(),
		o.BuildkitOptions.Build(),
		o.SummaryOptions.Build(),
		o.InteractiveOptions.Build(),
		o.ProviderOptions.Build(),
		o.PipelineOptions.Build(),
		o.InputsOptions.Build(),
		o.DisplayOptions.Build(),
		o.ExecuteOptions.Build(),
	)
}
