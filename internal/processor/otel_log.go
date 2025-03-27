package processor

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func WithLogger(logger logr.Logger, zapConfig *zap.Config) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if zapConfig == nil && logger.IsZero() {
			return nil
		}

		return &Logger{
			stepName:  spec.Name,
			zapConfig: zapConfig,
			logger:    logger,
		}
	}
}

type Logger struct {
	stepName  string
	zapConfig *zap.Config
	logger    logr.Logger
}

func (s *Logger) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		/*s.zapConfig.
			logger, err := s.zapConfig.Build()
		if err != nil {
			return stepContext, fmt.Errorf("failed setup step logger %w", err)
		}*/

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
			return stepContext, fmt.Errorf("failed setup step logger: no such log encoder `%s`", s.zapConfig.Encoding)
		}

		core := zapcore.NewCore(
			encoder,
			zapcore.AddSync(stepContext.Stderr),
			s.zapConfig.Level,
		)

		zapLogger := zap.New(core)
		logger := zapr.NewLogger(zapLogger)
		ctx = logr.NewContext(ctx, logger)

		logger.Info("process step")
		logger.V(1).Info("step context input", "context", stepContext)
		stepContext, err := next(ctx, stepContext)
		logger.Info("process step done", "err", err)
		logger.V(1).Info("step done", "err", err, "context", stepContext)

		return stepContext, err
	}, nil
}
