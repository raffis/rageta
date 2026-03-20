package run

import (
	"context"

	"github.com/raffis/rageta/internal/logbridge"
	"github.com/raffis/rageta/internal/otelsetup"
	"github.com/spf13/pflag"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const otelName = "github.com/raffis/rageta"

type OtelOptions struct {
	OtelOpts  otelsetup.Options
	Output    string
	ZapConfig zap.Config
}

func (s *OtelOptions) BindFlags(flags *pflag.FlagSet) {
	otelFlags := pflag.NewFlagSet("otel", pflag.ExitOnError)
	s.OtelOpts.BindFlags(otelFlags)
	flags.AddFlagSet(otelFlags)
}

func (s OtelOptions) Build() Step {
	return &Otel{opts: s}
}

type Otel struct {
	opts OtelOptions
}

type OtelContext struct {
	Tracer trace.Tracer
	Meter  metric.Meter
	Logger log.Logger
}

func (s *Otel) Run(rc *RunContext, next Next) error {
	ctx := context.Background()

	traceProvider, err := s.opts.OtelOpts.BuildTraceProvider(ctx)
	if err != nil {
		return err
	}
	if traceProvider != nil {
		rc.Otel.Tracer = traceProvider.Tracer(otelName)
		defer func() {
			traceProvider.Shutdown(context.Background())
		}()
	}

	meterProvider, err := s.opts.OtelOpts.BuildMeterProvider(ctx)
	if err != nil {
		return err
	}
	if meterProvider != nil {
		rc.Otel.Meter = meterProvider.Meter(otelName)
		defer func() {
			meterProvider.Shutdown(context.Background())
		}()
	}

	logProvider, err := s.opts.OtelOpts.BuildLoggerProvider(ctx)
	if err != nil {
		return err
	}

	if logProvider != nil {
		rc.Otel.Logger = logProvider.Logger(otelName)
		defer func() {
			logProvider.Shutdown(context.Background())
		}()

		rc.Logging.MainLog = logbridge.OtelCore(rc.Otel.Logger)
	}

	return next(rc)
}
