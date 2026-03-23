package run

import (
	"github.com/raffis/rageta/internal/pipeline"
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"github.com/spf13/pflag"
)

type PipelineOptions struct {
	SkipDone      bool
	MaxConcurrent int
	SkipSteps     []string
}

func (s PipelineOptions) Build() Step {
	return &Pipeline{opts: s}
}

func (s *PipelineOptions) BindFlags(flags *pflag.FlagSet) {
	flags.BoolVar(&s.SkipDone, "skip-done", false, "skip already done steps")
	flags.IntVar(&s.MaxConcurrent, "max-concurrent", 0, "Max concurrent container steps")
	flags.StringSliceVar(&s.SkipSteps, "skip-steps", nil, "skip steps")
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
	var pool chan struct{}

	if s.opts.MaxConcurrent > 0 {
		pool = make(chan struct{}, s.opts.MaxConcurrent)
	}

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
			processor.WithMatrix(),
			processor.WithOutput(rc.Output.Factory, rc.Output.InternalSteps, rc.Output.Expand),
			processor.WithMonitor(rc.Events.Enabled, rc.Events.WaitUpdateInterval, rc.Events.Dev),
			processor.WithOtelTrace(rc.Logging.Logger, rc.Otel.Tracer),
			processor.WithLogger(rc.Logging.Logger, rc.Logging.Builder, rc.Logging.Detached),
			processor.WithOtelMetrics(rc.Otel.Meter),
			processor.WithSkipBlacklist(s.opts.SkipSteps),
			processor.WithGarbageCollector(rc.Teardown.Enabled, rc.ContainerRuntime.Driver, rc.Teardown.Teardown),
			processor.WithAllowFailure(),
			processor.WithTimeout(),
			processor.WithSkipDone(s.opts.SkipDone),
			processor.WithIf(rc.CEL.Env),
			processor.WithTmpDir(),
			processor.WithTemplate(rc.Template.Container),
			processor.WithNeeds(),
			processor.WithStdioRedirect(false),
			processor.WithMaxConcurrent(pool),
			processor.WithRun(rc.ImagePolicy.PullPolicy, rc.ContainerRuntime.Driver, rc.Output.Factory, rc.Teardown.Teardown),
			processor.WithInherit(*pipeline, rc.Provider.Provider),
			processor.WithAnd(),
			processor.WithConcurrent(),
			processor.WithPipe(false),
		)

		return processor.WithDebug(rc.Logging.Logger, rc.Logging.Debug, &spec, processors...)
	}
}
