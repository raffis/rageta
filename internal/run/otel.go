package runner

import (
	"context"
	"fmt"
	"io"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/raffis/rageta/internal/logbridge"
	"github.com/raffis/rageta/internal/otelsetup"
	"github.com/raffis/rageta/internal/processor"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const otelName = "github.com/raffis/rageta"

type OtelStep struct {
	otelOpts  otelsetup.Options
	output    string
	zapConfig zap.Config
}

func WithOtel(otelOpts otelsetup.Options, output string, zapConfig zap.Config) *OtelStep {
	return &OtelStep{otelOpts: otelOpts, output: output, zapConfig: zapConfig}
}

func (s *OtelStep) Run(rc *RunContext, next Next) error {
	ctx := context.Background()

	traceProvider, err := s.otelOpts.BuildTraceProvider(ctx)
	if err != nil {
		return err
	}
	if traceProvider != nil {
		rc.Tracer = traceProvider.Tracer(otelName)
		rc.TraceShutdown = func() error { return traceProvider.Shutdown(context.Background()) }
		defer func() {
			if rc.TraceShutdown != nil {
				_ = rc.TraceShutdown()
			}
		}()
	}

	meterProvider, err := s.otelOpts.BuildMeterProvider(ctx)
	if err != nil {
		return err
	}
	if meterProvider != nil {
		rc.Meter = meterProvider.Meter(otelName)
		rc.MeterShutdown = func() error { return meterProvider.Shutdown(context.Background()) }
		defer func() {
			if rc.MeterShutdown != nil {
				_ = rc.MeterShutdown()
			}
		}()
	}

	logProvider, err := s.otelOpts.BuildLoggerProvider(ctx)
	if err != nil {
		return err
	}
	if logProvider != nil {
		rc.OtelLogger = logProvider.Logger(otelName)
		rc.LogShutdown = func() error { return logProvider.Shutdown(context.Background()) }
		defer func() {
			if rc.LogShutdown != nil {
				_ = rc.LogShutdown()
			}
		}()
	}

	defaultLog := rc.LogCoreFile
	if rc.OtelLogger != nil {
		defaultLog = zapcore.NewTee(rc.LogCoreFile, logbridge.OtelCore(rc.OtelLogger))
	}

	rc.LogBuilder = s.logBuilder(defaultLog)

	if s.output == "ui" {
		rc.Logger = zapr.NewLogger(zap.New(defaultLog))
	} else {
		var err error
		rc.Logger, err = rc.LogBuilder(rc.Stderr)
		if err != nil {
			return err
		}
	}

	return next(rc)
}

func (s *OtelStep) logBuilder(defaultLog zapcore.Core) processor.LogBuilder {
	return func(w io.Writer) (logr.Logger, error) {
		log, err := s.buildZapCore(w)
		if err != nil {
			return logr.Discard(), err
		}
		zapLogger := zap.New(zapcore.NewTee(defaultLog, log))
		return zapr.NewLogger(zapLogger), nil
	}
}

func (s *OtelStep) buildZapCore(w io.Writer) (zapcore.Core, error) {
	config := s.zapConfig
	var encoder zapcore.Encoder
	switch config.Encoding {
	case "json":
		encoder = zapcore.NewJSONEncoder(config.EncoderConfig)
	case "console":
		encoder = zapcore.NewConsoleEncoder(config.EncoderConfig)
	default:
		return nil, fmt.Errorf("no such log encoder %q", config.Encoding)
	}
	return zapcore.NewCore(encoder, zapcore.AddSync(w), config.Level), nil
}
