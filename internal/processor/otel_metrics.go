package processor

import (
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"go.opentelemetry.io/otel/metric"
)

/*
var (
	tracer  = OtelMetrics.Tracer(name)
	meter   = OtelMetrics.Meter(name)
	logger  = OtelMetricsslog.NewLogger(name)
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

func WithOtelMetrics(meter metric.Meter) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if meter == nil {
			return nil
		}

		return &OtelMetrics{
			stepName: spec.Name,
			meter:    meter,
		}
	}
}

type OtelMetrics struct {
	stepName string
	meter    metric.Meter
}

func (s *OtelMetrics) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		//roll := 1 + rand.Intn(6)

		/*var msg string
		if player := r.PathValue("player"); player != "" {
			msg = fmt.Sprintf("%s is rolling the dice", player)
		} else {
			msg = "Anonymous player is rolling the dice"
		}
		logger.InfoContext(ctx, msg, "result", roll)
		*/

		//rollCnt.Add(ctx, 1, metric.WithAttributes(rollValueAttr))

		ctx, err := next(ctx)
		return ctx, err
	}, nil
}
