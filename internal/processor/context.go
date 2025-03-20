package processor

import (
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/raffis/rageta/internal/ioext"
	cruntime "github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
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
		Inputs:     make(map[string]*anypb.Any),
		Matrix:     maps.Clone(t.Matrix),
		Envs:       maps.Clone(t.Envs),
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
			Outputs: make(map[string]*anypb.Any),
			TmpDir:  v.DataDir,
		}

		for outputKey, outputValue := range v.Outputs {
			_v, _ := paramValueToAny(outputValue)
			vars.Steps[k].Outputs[outputKey] = _v
		}
	}

	for _, v := range t.Outputs {
		vars.Outputs[v.Name] = &v1beta1.Output{
			Path: v.file.Name(),
		}
	}

	for k, v := range t.inputs {
		_v, _ := paramValueToAny(v)
		vars.Inputs[k] = _v
	}

	return vars
}

func (t StepContext) RuntimeVars() map[string]interface{} {
	return map[string]interface{}{
		"context": t.ToV1Beta1(),
	}
}

func paramValueToAny(v v1beta1.ParamValue) (*anypb.Any, error) {
	switch v.Type {
	case v1beta1.ParamTypeString:
		return anypb.New(wrapperspb.String(v.StringVal))
	case v1beta1.ParamTypeArray:
		list := &structpb.ListValue{}
		for _, item := range v.ArrayVal {
			list.Values = append(list.Values, structpb.NewStringValue(item))
		}
		return anypb.New(list)
	case v1beta1.ParamTypeObject:
		obj := &structpb.Struct{}
		for k, v := range v.ObjectVal {
			obj.Fields[k] = structpb.NewStringValue(v)
		}

		return anypb.New(obj)
	}

	return nil, errors.New("can not convert unsupported param")
}
