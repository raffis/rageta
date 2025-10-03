package processor

import (
	"io"

	"github.com/go-logr/logr"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type LogBuilder func(w io.Writer) (logr.Logger, error)

func WithLogger(defaultLogger logr.Logger, logBuilder LogBuilder, detached bool) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if logBuilder == nil && defaultLogger.IsZero() {
			return nil
		}

		return &Logger{
			stepName:   spec.Name,
			logger:     defaultLogger,
			logBuilder: logBuilder,
			detached:   detached,
		}
	}
}

type Logger struct {
	stepName   string
	logBuilder LogBuilder
	logger     logr.Logger
	detached   bool
}

func (s *Logger) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		logger := s.logger

		if ctx.Stderr != nil && ctx.Stderr != io.Discard && !s.detached {
			var err error
			logger, err = s.logBuilder(ctx.Stderr)
			if err != nil {
				return ctx, err
			}

			for _, tag := range ctx.Tags() {
				logger = logger.WithValues(tag.Key, tag.Value)
			}
		}

		logger = logger.WithValues("step", s.stepName)
		ctx.Context = logr.NewContext(ctx, logger)
		logger.V(2).Info("step context input", "context", ctx)
		ctx, err := next(ctx)
		logger.V(2).Info("step done", "err", err, "context", ctx)

		return ctx, err
	}, nil
}
