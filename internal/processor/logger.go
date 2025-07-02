package processor

import (
	"fmt"
	"io"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func WithLogger(logger logr.Logger, zapConfig *zap.Config, detached bool) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if zapConfig == nil && logger.IsZero() {
			return nil
		}

		return &Logger{
			stepName:  spec.Name,
			zapConfig: zapConfig,
			logger:    logger,
			detached:  detached,
		}
	}
}

type Logger struct {
	stepName  string
	zapConfig *zap.Config
	logger    logr.Logger
	detached  bool
}

func (s *Logger) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:      "time",
		LevelKey:     "level",
		MessageKey:   "msg",
		EncodeTime:   zapcore.ISO8601TimeEncoder,
		EncodeLevel:  zapcore.CapitalLevelEncoder,
		EncodeCaller: zapcore.ShortCallerEncoder,
	}

	var encoder zapcore.Encoder
	switch s.zapConfig.Encoding {
	case "json":
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	case "console":
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	default:
		return nil, fmt.Errorf("failed setup step logger: no such log encoder `%s`", s.zapConfig.Encoding)
	}

	return func(ctx StepContext) (StepContext, error) {
		logger := s.logger

		if ctx.Stderr != nil && ctx.Stderr != io.Discard && !s.detached {
			core := zapcore.NewCore(
				encoder,
				zapcore.AddSync(ctx.Stderr),
				s.zapConfig.Level,
			)

			zapLogger := zap.New(core)
			logger = zapr.NewLogger(zapLogger)

			for _, tag := range ctx.Tags() {
				logger = logger.WithValues(tag.Key, tag.Value)
			}
		}

		ctx.Context = logr.NewContext(ctx, logger)
		logger.V(2).Info("step context input", "context", ctx)
		ctx, err := next(ctx)
		logger.V(2).Info("step done", "err", err, "context", ctx)

		return ctx, err
	}, nil
}
