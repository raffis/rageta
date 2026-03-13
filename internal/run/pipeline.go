package run

import (
	"github.com/alitto/pond/v2"
	"github.com/raffis/rageta/internal/pipeline"
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type PipelineOptions struct {
}

func (s PipelineOptions) Build() Step {
	return &Pipeline{opts: s}
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

	pool := pond.NewPool(29)

	return func(spec v1beta1.Step) []processor.Bootstraper {
		processors := processor.Builder(&spec,
			processor.WithRecover(),
			processor.WithReport(rc.Report.Factory),
			processor.WithRetry(),
			processor.WithResult(),
			processor.WithInputVars(rc.CEL.Env),
			processor.WithEnvVars(osEnvMap(), rc.Envs.Envs),
			processor.WithSecretVars(osEnvMap(), rc.Secrets.Secrets, rc.Secrets.Store),
			processor.WithOutputVars(),
			processor.WithTags(rc.Tags.Tags),
			processor.WithMatrix(pool),
			processor.WithOutput(rc.Output.Factory, rc.Output.InternalSteps, rc.Output.Expand),
			processor.WithMonitor(rc.Events.Enabled, rc.Events.WaitUpdateInterval, rc.Events.Dev),
			processor.WithOtelTrace(rc.Logging.Logger, rc.Otel.Tracer),
			processor.WithLogger(rc.Logging.Logger, rc.Logging.Builder, rc.Logging.Detached),
			processor.WithOtelMetrics(rc.Otel.Meter),
			//processor.WithSkipBlacklist(opts.SkipSteps),
			//processor.WithGarbageCollector(opts.NoGC, rc.Driver, rc.Teardown),
			processor.WithAllowFailure(),
			processor.WithTimeout(),
			//processor.WithSkipDone(opts.SkipDone),
			processor.WithIf(rc.CEL.Env),
			processor.WithTmpDir(),
			processor.WithTemplate(rc.Template.Container),
			processor.WithNeeds(),
			processor.WithStdioRedirect(false),
			processor.WithRun(rc.ImagePolicy.PullPolicy, rc.ContainerRuntime.Driver, rc.Output.Factory, rc.Teardown.Teardown),
			processor.WithInherit(*pipeline, rc.Provider.Provider),
			processor.WithAnd(),
			processor.WithConcurrent(pool),
			processor.WithPipe(false),
		)

		return processors
		//debug := rc.Input.ZapConfig.Level.Level() <= -5
		//return processor.WithDebug(rc.Logger, debug, &spec, processors...)
	}
}
