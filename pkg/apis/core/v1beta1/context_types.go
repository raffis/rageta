package v1beta1

import (
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TaskResult struct {
	Error     string      `cel:"error"`
	StartedAt metav1.Time `cel:"startedAt"`
	EndedAt   metav1.Time `cel:"endedAt"`
}

type Context struct {
	Inputs  map[string]ParamValue  `cel:"inputs"`
	Secrets map[string]string      `cel:"secrets"`
	Tasks   map[string]*TaskResult `cel:"steps"`
	Matrix  map[string]string      `cel:"matrix"`
}

func (v *Context) Index() map[string]string {
	vars := map[string]string{}

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

	for k, v := range v.Matrix {
		vars[fmt.Sprintf("context.matrix.%s", k)] = v
	}

	for k, v := range v.Tasks {
		vars[fmt.Sprintf("context.steps.%s.error", k)] = v.Error
		vars[fmt.Sprintf("context.steps.%s.startedAt", k)] = fmt.Sprintf("%d", v.StartedAt.Unix())
		vars[fmt.Sprintf("context.steps.%s.endedAt", k)] = fmt.Sprintf("%d", v.EndedAt.Unix())
	}

	return vars
}
