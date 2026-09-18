package logbridge

import (
	"context"
	"fmt"
	"math"
	"time"

	otellog "go.opentelemetry.io/otel/log"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// OtelCore returns a zapcore.Core that forwards log entries to an OpenTelemetry Logger.
// If logger is nil, returns a no-op core.
func OtelCore(logger otellog.Logger) zapcore.Core {
	if logger == nil {
		return zapcore.NewNopCore()
	}
	return &otelCore{logger: logger}
}

type otelCore struct {
	logger otellog.Logger
	fields []zapcore.Field
}

func (c *otelCore) Enabled(level zapcore.Level) bool {
	return true
}

func (c *otelCore) With(fields []zapcore.Field) zapcore.Core {
	clone := &otelCore{logger: c.logger}
	clone.fields = make([]zapcore.Field, 0, len(c.fields)+len(fields))
	clone.fields = append(clone.fields, c.fields...)
	clone.fields = append(clone.fields, fields...)
	return clone
}

func (c *otelCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	return ce.AddCore(ent, c)
}

func (c *otelCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	var r otellog.Record
	r.SetTimestamp(ent.Time)
	r.SetObservedTimestamp(time.Now())
	r.SetSeverity(zapLevelToOtel(ent.Level))
	r.SetSeverityText(ent.Level.CapitalString())
	r.SetBody(otellog.StringValue(ent.Message))

	allFields := make([]zapcore.Field, 0, len(c.fields)+len(fields)+4)
	allFields = append(allFields, c.fields...)
	allFields = append(allFields, fields...)
	if ent.Caller.Defined {
		allFields = append(allFields,
			zap.String("caller", ent.Caller.TrimmedPath()),
			zap.String("function", ent.Caller.Function),
		)
	}
	if ent.Stack != "" {
		allFields = append(allFields, zap.String("stack", ent.Stack))
	}

	for _, f := range allFields {
		if kv := zapFieldToOtel(f); kv.Key != "" {
			r.AddAttributes(kv)
		}
	}
	c.logger.Emit(context.Background(), r)
	return nil
}

func (c *otelCore) Sync() error {
	return nil
}

func zapLevelToOtel(l zapcore.Level) otellog.Severity {
	switch l {
	case zapcore.DebugLevel:
		return otellog.SeverityDebug1
	case zapcore.InfoLevel:
		return otellog.SeverityInfo1
	case zapcore.WarnLevel:
		return otellog.SeverityWarn1
	case zapcore.ErrorLevel:
		return otellog.SeverityError1
	case zapcore.DPanicLevel, zapcore.PanicLevel, zapcore.FatalLevel:
		return otellog.SeverityError2
	default:
		return otellog.SeverityInfo1
	}
}

func zapFieldToOtel(f zapcore.Field) otellog.KeyValue {
	key := f.Key
	if key == "" {
		return otellog.KeyValue{}
	}
	switch f.Type {
	case zapcore.StringType:
		return otellog.String(key, f.String)
	case zapcore.Int64Type, zapcore.Int32Type, zapcore.Int16Type, zapcore.Int8Type:
		return otellog.Int64(key, f.Integer)
	case zapcore.Uint64Type, zapcore.Uint32Type, zapcore.Uint16Type, zapcore.Uint8Type:
		return otellog.Int64(key, int64(f.Integer))
	case zapcore.Float64Type:
		return otellog.Float64(key, math.Float64frombits(uint64(f.Integer)))
	case zapcore.Float32Type:
		return otellog.Float64(key, float64(math.Float32frombits(uint32(f.Integer))))
	case zapcore.BoolType:
		return otellog.Bool(key, f.Integer == 1)
	case zapcore.ErrorType:
		if err, ok := f.Interface.(error); ok {
			return otellog.String(key, err.Error())
		}
		return otellog.String(key, fmt.Sprint(f.Interface))
	case zapcore.DurationType, zapcore.TimeType, zapcore.TimeFullType:
		return otellog.Int64(key, f.Integer)
	default:
		return otellog.String(key, fmt.Sprint(f.Interface))
	}
}
