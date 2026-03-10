package runner

import (
	"os"
	"strings"

	"github.com/raffis/rageta/internal/pipeline"
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type BuilderStep struct {
	opts BuilderStepOptions
}

func WithBuilder(opts BuilderStepOptions) *BuilderStep {
	return &BuilderStep{opts: opts}
}

func (s *BuilderStep) Run(rc *RunContext, next Next) error {
	var b processor.PipelineBuilder
	b = pipeline.NewBuilder(
		pipeline.WithStepBuilder(s.stepBuilder(rc, &b)),
		pipeline.WithLogger(rc.Logger),
		pipeline.WithTmpDir(rc.ContextDir),
	)
	rc.Builder = b
	return next(rc)
}

func (s *BuilderStep) stepBuilder(rc *RunContext, builder *processor.PipelineBuilder) pipeline.StepBuilder {
	opts := s.opts
	return func(spec v1beta1.Step) []processor.Bootstraper {
		processors := processor.Builder(&spec,
			processor.WithRecover(),
			processor.WithReport(rc.ReportFactory),
			processor.WithRetry(),
			processor.WithResult(),
			processor.WithInputVars(rc.CelEnv),
			processor.WithEnvVars(s.osEnvMap(), rc.Envs),
			processor.WithSecretVars(s.osEnvMap(), rc.Secrets, rc.SecretStore),
			processor.WithOutputVars(),
			processor.WithTags(s.parseTags(opts.Tags)),
			processor.WithMatrix(rc.Pool),
			processor.WithOutput(rc.OutputFactory, opts.WithInternals, opts.Expand),
			processor.WithMonitor(!opts.NoStatus, opts.WaitUpdateInterval, rc.MonitorDev),
			processor.WithOtelTrace(rc.Logger, rc.Tracer),
			processor.WithLogger(rc.Logger, rc.LogBuilder, opts.LogDetached),
			processor.WithOtelMetrics(rc.Meter),
			processor.WithSkipBlacklist(opts.SkipSteps),
			processor.WithGarbageCollector(opts.NoGC, rc.Driver, rc.Teardown),
			processor.WithAllowFailure(),
			processor.WithTimeout(),
			processor.WithSkipDone(opts.SkipDone),
			processor.WithIf(rc.CelEnv),
			processor.WithTmpDir(),
			processor.WithTemplate(rc.Template),
			processor.WithNeeds(),
			processor.WithStdioRedirect(opts.Tee),
			processor.WithRun(rc.ImagePullPolicy, rc.Driver, rc.OutputFactory, rc.Teardown),
			processor.WithInherit(*builder, rc.Store),
			processor.WithAnd(),
			processor.WithConcurrent(rc.Pool),
			processor.WithPipe(opts.Tee),
		)
		debug := rc.Input.ZapConfig.Level.Level() <= -5
		return processor.WithDebug(rc.Logger, debug, &spec, processors...)
	}
}

func (s *BuilderStep) osEnvMap() map[string]string {
	envs := make(map[string]string)
	for _, v := range os.Environ() {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) == 2 {
			envs[parts[0]] = parts[1]
		}
	}
	return envs
}

func (s *BuilderStep) parseTags(tags []string) []processor.Tag {
	var result []processor.Tag
	for _, tag := range tags {
		v := strings.SplitN(tag, "=", 2)
		if len(v) != 2 {
			continue
		}
		t := processor.Tag{Key: v[0]}
		value := strings.SplitN(v[1], ":", 2)
		if len(value) == 2 {
			t.Value = value[0]
			t.Color = value[1]
		} else {
			t.Value = v[1]
		}
		result = append(result, t)
	}
	return result
}
