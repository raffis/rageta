package processor

import (
	"context"
	"math/rand"

	"github.com/go-logr/logr"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

/*
const name = "go.opentelemetry.io/otel/example/dice"


var (
	tracer  = otel.Tracer(name)
	meter   = otel.Meter(name)
	logger  = otelslog.NewLogger(name)
	rollCnt metric.Int64Counter
)

func init() {
	var err error
	rollCnt, err = meter.Int64Counter("dice.rolls",
		metric.WithDescription("The number of rolls by roll value"),
		metric.WithUnit("{roll}"))
	if err != nil {
		panic(err)
	}
}
*/

func WithOtel(logger logr.Logger, tracer trace.Tracer, meter metric.Meter) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		return &Otel{
			stepName: spec.Name,
			logger:   logger,
			tracer:   tracer,
			meter:    meter,
		}
	}
}

type Otel struct {
	stepName string
	logger   logr.Logger
	tracer   trace.Tracer
	meter    metric.Meter
}

func (s *Otel) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx context.Context, stepContext StepContext) (StepContext, error) {
		ctx, span := s.tracer.Start(ctx, s.stepName)
		defer span.End()

		logger := s.logger.WithValues(
			"step", s.stepName,
			"span-id", span.SpanContext().SpanID(),
			"trace-id", span.SpanContext().TraceID())

		logger.Info("process step")
		logger.V(1).Info("step context input", "context", stepContext)

		roll := 1 + rand.Intn(6)

		/*var msg string
		if player := r.PathValue("player"); player != "" {
			msg = fmt.Sprintf("%s is rolling the dice", player)
		} else {
			msg = "Anonymous player is rolling the dice"
		}
		logger.InfoContext(ctx, msg, "result", roll)
		*/

		rollValueAttr := attribute.Int("roll.value", roll)
		span.SetAttributes(rollValueAttr)
		//rollCnt.Add(ctx, 1, metric.WithAttributes(rollValueAttr))

		stepContext, err := next(ctx, stepContext)
		logger.V(1).Info("step done", "err", err, "context", stepContext)

		return stepContext, err
	}, nil
}
