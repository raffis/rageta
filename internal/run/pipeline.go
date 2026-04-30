package run

import (
	"github.com/raffis/rageta/internal/pipeline"
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/setup/flagset"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type PipelineOptions struct {
	SkipContainerLogs bool
	SkipSteps         []string
}

func (s PipelineOptions) Build() Step {
	return &Pipeline{opts: s}
}

func (s *PipelineOptions) BindFlags(flags flagset.Interface) {
	flags.BoolVar(&s.SkipContainerLogs, "skip-container-logs", s.SkipContainerLogs, "Do not store container output streams within the context directory")
	flags.StringSliceVar(&s.SkipSteps, "skip-steps", s.SkipSteps, "Skip steps")
}

type Pipeline struct {
	opts PipelineOptions
}

type PipelineContext struct {
	Builder processor.PipelineBuilder
}

func (s *Pipeline) Run(rc *RunContext, next Next) error {
	var builder processor.PipelineBuilder
	builder = pipeline.NewBuilder(
		pipeline.WithStepBuilder(s.stepPipeline(rc, &builder)),
		pipeline.WithLogger(rc.Logging.Logger),
		pipeline.WithTmpDir(rc.ContextDir.Path),
	)

	rc.Pipeline.Builder = builder
	return next(rc)
}

func (s *Pipeline) stepPipeline(rc *RunContext, pipeline *processor.PipelineBuilder) pipeline.StepBuilder {
	return func(spec v1beta1.Step) []processor.Bootstraper {
		processors := processor.Builder(&spec,
			processor.WithRecover(),
			processor.WithReport(rc.Report.Factory),
			processor.WithRetry(),
			processor.WithResult(),
			processor.WithTmpDir(),
			processor.WithInputVars(rc.CEL.Env),
			processor.WithEnvVars(osEnvMap(), rc.Envs.Envs),
			processor.WithSecretVars(osEnvMap(), rc.Secrets.Store),
			processor.WithOutputVars(),
			processor.WithTags(rc.Tags.Tags),
			processor.WithMatrix(),
			processor.WithOutput(rc.Output.Factory, rc.Output.InternalSteps, rc.Output.Expand),
			processor.WithEvents(rc.Events.Enabled, rc.Events.WaitUpdateInterval, rc.Events.Dev),
			processor.WithOtelTrace(rc.Logging.Logger, rc.Otel.Tracer),
			processor.WithLogger(rc.Logging.Logger, rc.Logging.Builder, rc.Logging.Detached),
			processor.WithOtelMetrics(rc.Otel.Meter),
			processor.WithSkipBlacklist(s.opts.SkipSteps),
			processor.WithAllowFailure(),
			processor.WithTimeout(),
			processor.WithWhen(rc.CEL.Env),
			processor.WithDependsOn(),
			//processor.WithContainerLogs(!s.opts.SkipContainerLogs, rc.Secrets.Store),
			processor.WithRun(rc.Buildkit.GatewayClient, rc.Buildkit.StatusRouter, rc.Buildkit.GWCacheImports, rc.Buildkit.NoCache),
			processor.WithService(rc.ImagePolicy.PullPolicy, rc.ContainerRuntime.Driver, rc.Teardown.Teardown),
			processor.WithInherit(*pipeline, rc.Provider.Provider),
		)

		return processor.WithDebug(rc.Logging.Logger, rc.Logging.Debug, &spec, processors...)
	}
}
