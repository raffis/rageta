package processor

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func WithOtelLog(logger logr.Logger, zapConfig *zap.Config) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if zapConfig == nil && logger.IsZero() {
			return nil
		}

		return &OtelLog{
			stepName:  spec.Name,
			zapConfig: zapConfig,
			logger:    logger,
		}
	}
}

type OtelLog struct {
	stepName  string
	zapConfig *zap.Config
	logger    logr.Logger
}

func (s *OtelLog) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {

		logger := []logr.Logger{s.logger}

		delegatedLogger := logr.NewDelegatingLogger()

		encoderConfig := zapcore.EncoderConfig{
			TimeKey:      "time",
			LevelKey:     "level",
			MessageKey:   "msg",
			EncodeTime:   zapcore.ISO8601TimeEncoder,
			EncodeLevel:  zapcore.CapitalLevelEncoder,
			EncodeCaller: zapcore.ShortCallerEncoder,
		}

		core := zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderConfig),
			zapcore.AddSync(stepContext.Stderr),
			zapcore.DebugLevel,
		)

		zapLogger := zap.New(core)
		logger := zapr.NewLogger(zapLogger)
		ctx = logr.NewContext(ctx, logger)

		logger.
			logger.Info("process step")
		logger.V(1).Info("step context input", "context", stepContext)
		stepContext, err := next(ctx, stepContext)
		logger.Info("process step done", "err", err)
		logger.V(1).Info("step done", "err", err, "context", stepContext)

		return stepContext, err
	}, nil
}
