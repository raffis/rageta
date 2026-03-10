package run

import (
	"context"
	"io"

	"github.com/alitto/pond/v2"
	"github.com/go-logr/logr"
	"github.com/google/cel-go/cel"
	"github.com/raffis/rageta/internal/mask"
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/provider"
	cruntime "github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap/zapcore"

	"github.com/spf13/pflag"
)

// RunContext holds state built up through the run steps.
// Per-run input (Ctx, Ref, Entrypoint, Inputs, etc.) is in Input; step-specific config is on each step.
type RunContext struct {
	Input *RunInput

	// Logging & OTEL
	Logger        logr.Logger
	LogBuilder    processor.LogBuilder
	LogCoreFile   zapcore.Core
	SecretStore   *mask.SecretStore
	Tracer        trace.Tracer
	Meter         metric.Meter
	OtelLogger    log.Logger
	TraceShutdown func() error
	MeterShutdown func() error
	LogShutdown   func() error

	// IO
	Stdout    io.Writer
	Stderr    io.Writer
	ReportDev io.Writer
	Envs      map[string]string
	Secrets   map[string]string

	// Context & lifecycle
	Ctx    context.Context
	Cancel context.CancelFunc

	// Runtime
	Driver          cruntime.Interface
	ContextDir      string
	Template        v1beta1.Template
	ImagePullPolicy cruntime.PullImagePolicy

	// Pipeline resolution
	Store     provider.Interface
	Command   v1beta1.Pipeline
	PersistDB func() error

	// CEL
	CelEnv *cel.Env

	// Factories
	OutputFactory processor.OutputFactory
	ReportFactory reportFinalizer

	// Teardown & concurrency
	TeardownFuncs []processor.Teardown
	Teardown      chan processor.Teardown
	Pool          pond.Pool
	MonitorDev    io.Writer
	monitorFile   io.Closer

	// Builder
	Builder processor.PipelineBuilder

	// Execution
	Inputs       map[string]v1beta1.ParamValue
	InputFlagSet *pflag.FlagSet
	StepCtx      processor.StepContext
	Result       error

	// TUI
	TUIApp  interface{}
	TUIDone chan struct{}
}

type reportFinalizer interface {
	processor.Reporter
	Finalize() error
}
