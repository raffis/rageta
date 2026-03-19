package run

import (
	"context"

	"github.com/raffis/rageta/internal/mask"
)

type RunContext struct {
	context.Context
	CEL              CELContext
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
	Template         TemplateContext
}

func NewContext() *RunContext {
	return &RunContext{
		Secrets: SecretsContext{
			Store: mask.NewSecretStore(mask.DefaultMask),
		},
	}
}
