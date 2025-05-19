package processor

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"runtime"
	"time"

	cruntime "github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type StepContext struct {
	context.Context
	Dir              string
	DataDir          string
	Matrix           map[string]string
	Inputs           map[string]v1beta1.ParamValue
	Steps            map[string]*StepContext
	Envs             map[string]string
	Containers       map[string]cruntime.ContainerStatus
	Tags             []Tag
	NamePrefix       string
	Env              string
	Outputs          []OutputParam
	Stdin            io.Reader
	Stdout           io.Writer
	Stderr           io.Writer
	AdditionalStdout []io.Writer
	AdditionalStderr []io.Writer
	Template         *v1beta1.Template
	StartedAt        time.Time
	EndedAt          time.Time
	OutputVars       map[string]v1beta1.ParamValue
	Error            error
}

type Tag struct {
	Key   string
	Value string
	Color string
}

type OutputParam struct {
	Name string
	Path string
}

func NewContext() StepContext {
	return StepContext{
		Envs:       make(map[string]string),
		Steps:      make(map[string]*StepContext),
		Inputs:     make(map[string]v1beta1.ParamValue),
		Containers: make(map[string]cruntime.ContainerStatus),
		Matrix:     make(map[string]string),
		OutputVars: make(map[string]v1beta1.ParamValue),
	}
}

func (c StepContext) DeepCopy() StepContext {
	copy := NewContext()
	copy.Context = c.Context
	copy.NamePrefix = c.NamePrefix
	copy.Dir = c.Dir
	copy.DataDir = c.DataDir
	copy.Stdout = c.Stdout
	copy.Stderr = c.Stderr
	copy.Stdin = c.Stdin
	copy.AdditionalStdout = append(copy.AdditionalStdout, c.AdditionalStdout...)
	copy.AdditionalStderr = append(copy.AdditionalStderr, c.AdditionalStderr...)

	/*for k, v := range c.Steps {
		copy.Steps[k] = &StepResult{
			StartedAt: v.StartedAt,
			EndedAt:   v.EndedAt,
			Outputs:   maps.Clone(v.Outputs),
			Error:     v.Error,
			DataDir:   v.DataDir,
		}
	}*/

	copy.OutputVars = maps.Clone(c.OutputVars)
	copy.Steps = maps.Clone(c.Steps)
	copy.Tags = append(copy.Tags, c.Tags...)
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

	/*for k, v := range vars.Steps {
		t.Steps[k] = &StepResult{
			DataDir: v.TmpDir,
		}
	}*/
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
		Uid:        fmt.Sprintf("%d", os.Getuid()),
		Guid:       fmt.Sprintf("%d", os.Getgid()),
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

		maps.Copy(vars.Steps[k].Outputs, v.OutputVars)
	}

	for _, v := range t.Outputs {
		vars.Outputs[v.Name] = &v1beta1.Output{
			Path: v.Path,
		}
	}
	return vars
}
