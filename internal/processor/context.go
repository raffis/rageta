package processor

import (
	"io"
	"maps"
	"runtime"
	"time"

	cruntime "github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type StepContext struct {
	Dir              string
	DataDir          string
	Matrix           map[string]string
	Inputs           map[string]v1beta1.ParamValue
	Steps            map[string]*StepResult
	Envs             map[string]string
	Containers       map[string]cruntime.ContainerStatus
	Tags             map[string]string
	NamePrefix       string
	Env              string
	Outputs          []OutputParam
	Stdin            io.Reader
	Stdout           io.Writer
	Stderr           io.Writer
	AdditionalStdout []io.Writer
	AdditionalStderr []io.Writer
	Template         *v1beta1.Template
}

type OutputParam struct {
	Name string
	Path string
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

func NewContext() StepContext {
	return StepContext{
		Envs:       make(map[string]string),
		Steps:      make(map[string]*StepResult),
		Inputs:     make(map[string]v1beta1.ParamValue),
		Containers: make(map[string]cruntime.ContainerStatus),
		Matrix:     make(map[string]string),
		Tags:       make(map[string]string),
	}
}

func (c StepContext) DeepCopy() StepContext {
	copy := NewContext()
	copy.NamePrefix = c.NamePrefix
	copy.Dir = c.Dir
	copy.DataDir = c.DataDir
	copy.Stdout = c.Stdout
	copy.Stderr = c.Stderr
	copy.Stdin = c.Stdin
	copy.AdditionalStdout = append(copy.AdditionalStdout, c.AdditionalStdout...)
	copy.AdditionalStderr = append(copy.AdditionalStderr, c.AdditionalStderr...)

	for k, v := range c.Steps {
		copy.Steps[k] = &StepResult{
			StartedAt: v.StartedAt,
			EndedAt:   v.EndedAt,
			Outputs:   maps.Clone(v.Outputs),
			Error:     v.Error,
			DataDir:   v.DataDir,
		}
	}

	copy.Tags = maps.Clone(c.Tags)
	copy.Inputs = maps.Clone(c.Inputs)
	copy.Envs = maps.Clone(c.Envs)
	copy.Containers = maps.Clone(c.Containers)
	copy.Matrix = maps.Clone(c.Matrix)
	if c.Template != nil {
		copy.Template = c.Template.DeepCopy()
	}

	return copy
}

func (t StepContext) Merge(c StepContext) StepContext {
	maps.Copy(t.Envs, c.Envs)
	maps.Copy(t.Inputs, c.Inputs)
	maps.Copy(t.Steps, c.Steps)
	maps.Copy(t.Containers, c.Containers)

	_ = mergeTemplate(t.Template, c.Template)

	return t
}

func (t StepContext) FromV1Beta1(vars *v1beta1.Context) {
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

func (t StepContext) ToV1Beta1() *v1beta1.Context {
	vars := &v1beta1.Context{
		TmpDir:     t.DataDir,
		Steps:      make(map[string]*v1beta1.StepResult),
		Containers: make(map[string]*v1beta1.ContainerStatus),
		Matrix:     maps.Clone(t.Matrix),
		Envs:       maps.Clone(t.Envs),
		Inputs:     maps.Clone(t.Inputs),
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
			Path: v.Path,
		}
	}
	return vars
}
