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
	Tags             TagsContext
	Events           EventsContext
	ImagePolicy      ImagePolicyContext
	Otel             OtelContext
	Logging          LoggingContext
	Report           ReportContext
	Output           OutputContext
	ImagePullPolicy  ImagePolicyContext
	Teardown         TeardownContext
	Provider         ProviderContext
	Pipeline         PipelineContext
	Execution        ExecutionContext
}

func NewContext() *RunContext {
	return &RunContext{}
}
