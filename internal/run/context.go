package run

import (
	"context"
)

type RunContext struct {
	context.Context
	CEL              CELContext
	Buildkit         BuildkitContext
	ContainerRuntime ContainerRuntimeContext
	ContextDir       ContextDirContext
	Envs             EnvsContext
	Inputs           InputsContext
	Secrets          SecretsContext
	Labels           LabelsContext
	Events           EventsContext
	ImagePolicy      ImagePolicyContext
	Otel             OtelContext
	Logging          LoggingContext
	Report           ReportContext
	Display          DisplayContext
	ImagePullPolicy  ImagePolicyContext
	Teardown         TeardownContext
	Provider         ProviderContext
	Pipeline         PipelineContext
}

func NewContext() *RunContext {
	return &RunContext{}
}
