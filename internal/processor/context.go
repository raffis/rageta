package processor

import (
	"io"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/raffis/rageta/internal/ioext"
	cruntime "github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type StepContext struct {
	dir        string
	dataDir    string
	Matrix     map[string]string
	inputs     map[string]v1beta1.ParamValue
	Steps      map[string]*StepResult
	Envs       map[string]string
	Containers map[string]cruntime.ContainerStatus
	NamePrefix string
	Env        string
	Parent     string
	Outputs    []OutputParam
	Stdin      io.Reader
	Stdout     *ioext.MultiWriter
	Stderr     *ioext.MultiWriter
}

type OutputParam struct {
	Name string
	file *os.File
}

type StepResult struct {
	StartedAt time.Time
	EndedAt   time.Time
	Outputs   map[string]v1beta1.ParamValue
	Error     error
	DataDir   string
}

func (t *StepResult) Duration() time.Duration {
	return t.EndedAt.Sub(t.StartedAt)
}

func NewContext(dir string, inputs map[string]v1beta1.ParamValue) StepContext {
	return StepContext{
		dir:        dir,
		dataDir:    filepath.Join(dir, "_data"),
		inputs:     inputs,
		Steps:      make(map[string]*StepResult),
		Envs:       make(map[string]string),
		Containers: make(map[string]cruntime.ContainerStatus),
		Matrix:     make(map[string]string),
		Stderr:     ioext.New(),
		Stdout:     ioext.New(),
	}
}

func (c StepContext) DeepCopy() StepContext {
	copy := NewContext(c.dir, maps.Clone(c.inputs))
	copy.NamePrefix = c.NamePrefix
	copy.dataDir = c.dataDir
	copy.Stdout.Add(c.Stdout.Unpack()...)
	copy.Stderr.Add(c.Stderr.Unpack()...)
	copy.Steps = maps.Clone(c.Steps)
	copy.Envs = maps.Clone(c.Envs)
	copy.Containers = maps.Clone(c.Containers)
	copy.Matrix = maps.Clone(c.Matrix)
	return copy
}

func (t StepContext) Merge(c StepContext) StepContext {
	for k, v := range c.Envs {
		t.Envs[k] = v
	}

	for k, v := range c.inputs {
		t.inputs[k] = v
	}

	for k, v := range c.Steps {
		t.Steps[k] = v
	}

	for k, v := range c.Containers {
		t.Containers[k] = v
	}

	return t
}

func (t StepContext) TmpDir() string {
	return t.dir
}

func (t StepContext) Child() StepContext {
	return StepContext{
		dir:        t.dir,
		dataDir:    t.dataDir,
		inputs:     maps.Clone(t.inputs),
		Steps:      maps.Clone(t.Steps),
		Envs:       maps.Clone(t.Envs),
		Containers: maps.Clone(t.Containers),
		Matrix:     maps.Clone(t.Matrix),
	}
}

func (t StepContext) FromV1Beta1(vars *v1beta1.RuntimeVars) {
	for k, v := range vars.Containers {
		t.Containers[k] = cruntime.ContainerStatus{
			ContainerID: v.ContainerID,
			ContainerIP: v.ContainerIP,
			Name:        v.Name,
			Ready:       v.Ready,
			Started:     v.Started,
			ExitCode:    int(v.ExitCode),
		}
	}

	for k, v := range vars.Steps {
		t.Steps[k] = &StepResult{
			//Outputs: v.Outputs,
			DataDir: v.TmpDir,
		}
	}
}

func (t StepContext) ToV1Beta1() *v1beta1.RuntimeVars {
	vars := &v1beta1.RuntimeVars{
		TmpDir:     t.dataDir,
		Steps:      make(map[string]*v1beta1.StepResult),
		Containers: make(map[string]*v1beta1.ContainerStatus),
		Matrix:     maps.Clone(t.Matrix),
		Envs:       maps.Clone(t.Envs),
		Inputs:     maps.Clone(t.inputs),
		Env:        t.Env,
		Outputs:    make(map[string]*v1beta1.Output),
		Os:         runtime.GOOS,
		Arch:       runtime.GOARCH,
	}

	for k, v := range t.Containers {
		vars.Containers[k] = &v1beta1.ContainerStatus{
			ContainerID: v.ContainerID,
			ContainerIP: v.ContainerIP,
			Name:        v.Name,
			Ready:       v.Ready,
			Started:     v.Started,
			ExitCode:    int32(v.ExitCode),
		}
	}

	for k, v := range t.Steps {
		vars.Steps[k] = &v1beta1.StepResult{
			Outputs: make(map[string]v1beta1.ParamValue),
			TmpDir:  v.DataDir,
		}

		for outputKey, outputValue := range v.Outputs {
			vars.Steps[k].Outputs[outputKey] = outputValue
		}
	}

	for _, v := range t.Outputs {
		vars.Outputs[v.Name] = &v1beta1.Output{
			Path: v.file.Name(),
		}
	}
	return vars
}

func (t StepContext) RuntimeVars() map[string]interface{} {
	vars := t.ToV1Beta1()
	mappedVars := map[string]interface{}{
		"tmpDir":     vars.TmpDir,
		"steps":      make(map[string]interface{}),
		"inputs":     make(map[string]interface{}),
		"containers": make(map[string]interface{}),
		"matrix":     vars.Matrix,
		"envs":       vars.Envs,
		"env":        vars.Env,
		"outputs":    make(map[string]interface{}),
		"os":         vars.Os,
		"Arch":       vars.Arch,
	}

	for k, v := range vars.Outputs {
		mappedVars["outputs"].(map[string]interface{})[k] = map[string]interface{}{
			"path": v.Path,
		}
	}

	for k, v := range vars.Containers {
		mappedVars["containers"].(map[string]interface{})[k] = map[string]interface{}{
			"containerID": v.ContainerID,
			"containerIP": v.ContainerIP,
			"name":        v.Name,
			"ready":       v.Ready,
			"started":     v.Started,
			"exitCode":    v.ExitCode,
		}
	}
	for k, v := range vars.Inputs {
		mappedVars["inputs"].(map[string]interface{})[k] = paramValueToAny(v)
	}

	for k, v := range vars.Steps {
		mappedVars["steps"].(map[string]interface{})[k] = map[string]interface{}{
			"tmpDir":  v.TmpDir,
			"outputs": make(map[string]interface{}),
		}

		for ok, ov := range v.Outputs {
			mappedVars["steps"].(map[string]interface{})[k].(map[string]interface{})["outputs"].(map[string]interface{})[ok] = paramValueToAny(ov)
		}
	}

	return map[string]interface{}{
		"context": mappedVars,
	}
}

func paramValueToAny(v v1beta1.ParamValue) interface{} {
	switch v.Type {
	case v1beta1.ParamTypeString:
		return v.StringVal
	case v1beta1.ParamTypeArray:
		return v.ArrayVal
	case v1beta1.ParamTypeObject:
		return v.ObjectVal
	}

	return v.StringVal
}
