package run

import (
	"fmt"
	"reflect"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type CELOptions struct{}

func (s CELOptions) Build() Step {
	return &CEL{opts: s}
}

type CEL struct {
	opts CELOptions
}

type CELContext struct {
	Env *cel.Env
}

func (s *CEL) Run(rc *RunContext, next Next) error {
	celEnv, err := cel.NewEnv(
		ext.Strings(),
		ext.Math(),
		ext.Lists(),
		ext.Encoders(),
		ext.Sets(),
		ext.NativeTypes(ext.ParseStructTags(true),
			reflect.TypeOf(&v1beta1.Context{}),
			reflect.TypeOf(&v1beta1.StepResult{}),
			reflect.TypeOf(&v1beta1.ParamValue{}),
			reflect.TypeOf(&v1beta1.Output{}),
			reflect.TypeOf(&v1beta1.ContainerStatus{}),
		),
		cel.Variable("context", cel.ObjectType("v1beta1.Context")),
	)
	if err != nil {
		return fmt.Errorf("setup cel env failed: %w", err)
	}

	rc.CEL.Env = celEnv
	return next(rc)
}
