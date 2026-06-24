package v1beta1

import (
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TaskResult struct {
	Outputs   map[string]ParamValue `cel:"outputs"`
	Error     string                `cel:"error"`
	StartedAt metav1.Time           `cel:"startedAt"`
	EndedAt   metav1.Time           `cel:"endedAt"`
}

type Context struct {
	Inputs  map[string]ParamValue  `cel:"inputs"`
	Envs    map[string]string      `cel:"envs"`
	Secrets map[string]string      `cel:"secrets"`
	Tasks   map[string]*TaskResult `cel:"steps"`
	Matrix  map[string]string      `cel:"matrix"`
	Secret  string                 `cel:"secret"`
	Env     string                 `cel:"env"`
	Os      string                 `cel:"os"`
	Arch    string                 `cel:"arch"`
	Uid     string                 `cel:"uid"`
	Guid    string                 `cel:"guid"`
}

func (v *Context) Index() map[string]string {
	vars := map[string]string{
		"context.os":     v.Os,
		"context.arch":   v.Arch,
		"context.uid":    v.Uid,
		"context.guid":   v.Guid,
		"context.env":    v.Env,
		"context.secret": v.Secret,
	}

	for k, v := range v.Inputs {
		switch v.Type {
		case ParamTypeString:
			vars[fmt.Sprintf("context.inputs.%s", k)] = v.StringVal
		case ParamTypeArray:
			b, _ := json.Marshal(v.ArrayVal)
			vars[fmt.Sprintf("context.inputs.%s", k)] = string(b)
		case ParamTypeObject:
			b, _ := json.Marshal(v.ObjectVal)
			vars[fmt.Sprintf("context.inputs.%s", k)] = string(b)
		}
	}

	for k, v := range v.Secrets {
		vars[fmt.Sprintf("context.secrets.%s", k)] = v
	}

	for k, v := range v.Envs {
		vars[fmt.Sprintf("context.envs.%s", k)] = v
	}

	for k, v := range v.Matrix {
		vars[fmt.Sprintf("context.matrix.%s", k)] = v
	}

	for k, v := range v.Tasks {
		vars[fmt.Sprintf("context.steps.%s.error", k)] = v.Error
		vars[fmt.Sprintf("context.steps.%s.startedAt", k)] = fmt.Sprintf("%d", v.StartedAt.Unix())
		vars[fmt.Sprintf("context.steps.%s.endedAt", k)] = fmt.Sprintf("%d", v.EndedAt.Unix())
		for outputName, v := range v.Outputs {
			switch v.Type {
			case ParamTypeString:
				vars[fmt.Sprintf("context.steps.%s.outputs.%s", k, outputName)] = v.StringVal
			case ParamTypeArray:
				b, _ := json.Marshal(v.ArrayVal)
				vars[fmt.Sprintf("context.steps.%s.outputs.%s", k, outputName)] = string(b)
			case ParamTypeObject:
				b, _ := json.Marshal(v.ObjectVal)
				vars[fmt.Sprintf("context.steps.%s.outputs.%s", k, outputName)] = string(b)
			}
		}
	}

	return vars
}
