package run

import (
	"github.com/raffis/rageta/internal/pipeline"
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/setup/flagset"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type PipelineOptions struct {
	SkipContainerLogs bool
	SkipTasks         []string
}

func (s PipelineOptions) Build() Task {
	return &Pipeline{opts: s}
}

func (s *PipelineOptions) BindFlags(flags flagset.Interface) {
	flags.BoolVar(&s.SkipContainerLogs, "skip-container-logs", s.SkipContainerLogs, "Do not store container output streams within the context directory")
	flags.StringSliceVar(&s.SkipTasks, "skip-steps", s.SkipTasks, "Skip steps")
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
		pipeline.WithTaskBuilder(s.stepPipeline(rc, &builder)),
		pipeline.WithLogger(rc.Logging.Logger),
	)

	rc.Pipeline.Builder = builder
	return next(rc)
}

func (s *Pipeline) stepPipeline(rc *RunContext, pipeline *processor.PipelineBuilder) pipeline.TaskBuilder {
	return func(spec v1beta1.Task) []processor.Bootstraper {
		processors := processor.Builder(&spec,
			processor.WithRecover(),
			processor.WithReport(rc.Report.Factory),
			processor.WithRetry(),
			processor.WithResult(),
			processor.WithImage(),
			processor.WithWorkdir(),
			processor.WithStyle(),
			processor.WithDisplay(rc.Display.Factory),
			processor.WithStats(),
			processor.WithEvents(rc.Events.Enabled, rc.Events.WaitUpdateInterval, rc.Events.Dev),
			processor.WithMatrix(),
			processor.WithDependsOn(),
			processor.WithOtelTrace(rc.Logging.Logger, rc.Otel.Tracer),
			processor.WithLogger(rc.Logging.Logger, rc.Logging.Builder, rc.Logging.Detached),
			processor.WithOtelMetrics(rc.Otel.Meter),
			processor.WithSkipBlacklist(s.opts.SkipTasks),
			processor.WithAllowFailure(),
			processor.WithTimeout(),
			processor.WithWhen(rc.CEL.Env),
			processor.WithArtifacts(rc.Buildkit.GatewayClient),
			processor.WithInputVars(rc.CEL.Env),
			processor.WithLabels(rc.Labels.Labels),
			processor.WithEnvVars(osEnvMap(), rc.Envs.Envs),
			processor.WithSecretVars(osEnvMap(), rc.Secrets.Store),
			processor.WithService(rc.ImagePolicy.PullPolicy, rc.ContainerRuntime.Driver, rc.Teardown.Teardown),
			processor.WithSteps(rc.Secrets.Store),
			processor.WithSources(),
			processor.WithCaches(),
			processor.WithBuild(rc.Buildkit.GatewayClient, rc.Buildkit.StatusRouter, rc.Buildkit.GWCacheImports, rc.Buildkit.NoCache, &rc.Buildkit.BuiltRefs),
			processor.WithGroup(rc.Display.GroupBy),
			processor.WithInherit(*pipeline, rc.Provider.Provider),
		)

		return processor.WithDebug(rc.Logging.Logger, rc.Logging.Debug, &spec, processors...)
	}
}
