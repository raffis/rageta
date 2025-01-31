package processor

import (
	"io"
	"maps"
	"runtime"
	"time"

	cruntime "github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type StepContext struct {
	tmpDir     string
	Matrix     map[string]interface{}
	inputs     map[string]interface{}
	Steps      map[string]*StepResult
	Envs       map[string]string
	Containers map[string]cruntime.ContainerStatus
	NamePrefix string
	Env        string
	Parent     string
	Output     string
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
}

type StepResult struct {
	StartedAt  time.Time
	EndedAt    time.Time
	Outputs    map[string]string
	Envs       map[string]string
	Containers map[string]cruntime.ContainerStatus
	Error      error
}

func (t *StepResult) Duration() time.Duration {
	return t.EndedAt.Sub(t.StartedAt)
}

func NewContext(tmpDir string, inputs map[string]interface{}) StepContext {
	return StepContext{
		tmpDir:     tmpDir,
		inputs:     inputs,
		Steps:      make(map[string]*StepResult),
		Envs:       make(map[string]string),
		Containers: make(map[string]cruntime.ContainerStatus),
		Matrix:     make(map[string]interface{}),
	}
}

func (c StepContext) DeepCopy() StepContext {
	copy := NewContext(c.tmpDir, c.inputs)
	copy.NamePrefix = c.NamePrefix

	for k, v := range c.Envs {
		copy.Envs[k] = v
	}

	for k, v := range c.Steps {
		copy.Steps[k] = v
	}

	for k, v := range c.Matrix {
		copy.Matrix[k] = v
	}

	for k, v := range c.Containers {
		copy.Containers[k] = v
	}

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
	return t.tmpDir
}

func (t StepContext) Child() StepContext {
	return StepContext{
		tmpDir:     t.tmpDir,
		inputs:     maps.Clone(t.inputs),
		Steps:      maps.Clone(t.Steps),
		Envs:       maps.Clone(t.Envs),
		Containers: maps.Clone(t.Containers),
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
			Outputs: v.Outputs,
		}
	}
}

func (t StepContext) ToV1Beta1() *v1beta1.RuntimeVars {
	vars := &v1beta1.RuntimeVars{
		RootDir:    "/__rootfs",
		TmpDir:     t.tmpDir,
		Envs:       t.Envs,
		Steps:      make(map[string]*v1beta1.StepResult),
		Containers: make(map[string]*v1beta1.ContainerStatus),
		Inputs:     make(map[string]*anypb.Any),
		Matrix:     make(map[string]*anypb.Any),
		Env:        t.Env,
		Output:     t.Output,
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
			Outputs: v.Outputs,
		}
	}

	for k, v := range t.inputs {
		vars.Inputs[k] = protoWrap(v)
	}

	for k, v := range t.Matrix {
		vars.Matrix[k] = protoWrap(v)
	}

	return vars
}

func (t StepContext) RuntimeVars() map[string]interface{} {
	return map[string]interface{}{
		"context": t.ToV1Beta1(),
	}
}

func protoWrap(v interface{}) *anypb.Any {
	switch v := v.(type) {
	case []string:
		values := []interface{}{}
		for _, listV := range v {
			values = append(values, listV)
		}

		list, _ := structpb.NewList(values)
		val, _ := anypb.New(list)
		return val
	case string:
		strWrapper := wrapperspb.String(v)
		val, _ := anypb.New(strWrapper)
		return val
	case bool:
		boolWrapper := wrapperspb.Bool(v)
		val, _ := anypb.New(boolWrapper)
		return val
	}

	return nil
}
