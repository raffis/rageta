package run

import (
	"context"
	"io"
)

type RunContext struct {
	context.Context
	Checklist io.Closer
	Cancel    context.CancelFunc
	CEL       CELContext
	Buildkit  BuildkitContext
	Envs      EnvsContext
	Inputs    InputsContext
	Secrets   SecretsContext
	Labels    LabelsContext
	Otel      OtelContext
	Logging   LoggingContext
	Report    ReportContext
	Display   DisplayContext
	//ImagePullPolicy  ImagePolicyContext
	Teardown TeardownContext
	Provider ProviderContext
	Pipeline PipelineContext
	Execute  ExecuteContext
}

func NewContext() *RunContext {
	return &RunContext{}
}
