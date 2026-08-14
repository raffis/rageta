package run

import (
	"github.com/raffis/rageta/internal/pipeline"
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type PipelineOptions struct{}

func (s PipelineOptions) Build() Task {
	return &Pipeline{}
}

type Pipeline struct {
}

type PipelineContext struct {
	Builder processor.PipelineBuilder
}

func (s *Pipeline) Label() string {
	return "Preparing pipeline"
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
			processor.WithImage(rc.Buildkit.GatewayClient),
			processor.WithWorkdir(),
			processor.WithStyle(),
			processor.WithDisplay(rc.Display.Factory),
			processor.WithStats(),
			processor.WithMatrix(),
			processor.WithDependsOn(),
			processor.WithOtelTrace(rc.Logging.Logger, rc.Otel.Tracer),
			processor.WithLogger(rc.Logging.Logger, rc.Logging.Builder, rc.Logging.Detached),
			processor.WithOtelMetrics(rc.Otel.Meter),
			processor.WithAllowFailure(),
			processor.WithTimeout(),
			processor.WithWhen(rc.CEL.Env),
			processor.WithArtifacts(rc.Buildkit.GatewayClient),
			processor.WithInputVars(rc.CEL.Env),
			processor.WithLabels(rc.Labels.Labels),
			processor.WithEnvVars(osEnvMap(), rc.Envs.Envs),
			processor.WithSecretVars(osEnvMap(), rc.Secrets.Store),
			processor.WithSources(),
			processor.WithServiceBinding(),
			processor.WithService(rc.Buildkit.GatewayClient, rc.Teardown.Teardown),
			processor.WithVolumes(),
			processor.WithSteps(rc.Secrets.Store, rc.Buildkit.NoCache),
			processor.WithBuild(rc.Buildkit.GatewayClient, rc.Buildkit.VertexRouter, rc.Buildkit.GWCacheImports, rc.Buildkit.NoCache, &rc.Buildkit.BuiltRefs),
			processor.WithGroup(rc.Display.GroupBy),
			processor.WithInherit(*pipeline, rc.Provider.Provider),
		)

		return processor.WithDebug(rc.Logging.Logger, rc.Logging.Debug, &spec, processors...)
	}
}
