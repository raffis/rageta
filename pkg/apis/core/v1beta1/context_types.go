package v1beta1

import (
	"encoding/json"
	"fmt"
)

type StepResult struct {
	Outputs map[string]ParamValue `cel:"outputs"`
	TmpDir  string                `cel:"tmpDir"`
}

type ContainerStatus struct {
	ContainerID string
	ContainerIP string
	Name        string
	Ready       bool
	Started     bool
	ExitCode    int32
}

type Output struct {
	Path string `cel:"path"`
}

type Context struct {
	Inputs     map[string]ParamValue       `cel:"inputs"`
	Envs       map[string]string           `cel:"envs"`
	Containers map[string]*ContainerStatus `cel:"containers"`
	Steps      map[string]*StepResult      `cel:"steps"`
	TmpDir     string                      `cel:"tmpDir"`
	Matrix     map[string]string           `cel:"matrix"`
	Env        string                      `cel:"env"`
	Outputs    map[string]*Output          `cel:"outputs"`
	Os         string                      `cel:"os"`
	Arch       string                      `cel:"arch"`
	Uid        string                      `cel:"uid"`
	Guid       string                      `cel:"guid"`
}

func (v *Context) Index() map[string]string {
	vars := map[string]string{
		"context.os":     v.Os,
		"context.arch":   v.Arch,
		"context.uid":    v.Uid,
		"context.guid":   v.Guid,
		"context.env":    v.Env,
		"context.tmpDir": v.TmpDir,
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

	for k, v := range v.Envs {
		vars[fmt.Sprintf("context.envs.%s", k)] = v
	}

	for k, v := range v.Matrix {
		vars[fmt.Sprintf("context.matrix.%s", k)] = v
	}

	for k, v := range v.Containers {
		vars[fmt.Sprintf("context.containers.%s.containerID", k)] = v.ContainerID
		vars[fmt.Sprintf("context.containers.%s.containerIP", k)] = v.ContainerIP
		vars[fmt.Sprintf("context.containers.%s.name", k)] = v.Name
	}

	for k, v := range v.Outputs {
		vars[fmt.Sprintf("context.outputs.%s.path", k)] = v.Path
	}

	for k, v := range v.Steps {
		vars[fmt.Sprintf("context.steps.%s.tmpDir", k)] = v.TmpDir
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
