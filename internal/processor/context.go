package processor

import (
	"fmt"
	"io"
	"maps"
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
	Stdout     *ioext.MultiWriter
	Stderr     *ioext.MultiWriter
}

type StepResult struct {
	StartedAt time.Time
	EndedAt   time.Time
	Outputs   map[string]string
	Error     error
	DataDir   string
}

func (t *StepResult) Duration() time.Duration {
	return t.EndedAt.Sub(t.StartedAt)
}

func NewContext(dir string, inputs map[string]interface{}) StepContext {
	return StepContext{
		dir:        dir,
		dataDir:    filepath.Join(dir, "_data"),
		inputs:     inputs,
		Steps:      make(map[string]*StepResult),
		Envs:       make(map[string]string),
		Containers: make(map[string]cruntime.ContainerStatus),
		Matrix:     make(map[string]interface{}),
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
			DataDir: v.TmpDir,
		}
	}
}

func (t StepContext) ToV1Beta1() *v1beta1.RuntimeVars {
	vars := &v1beta1.RuntimeVars{
		RootDir:    "/__rootfs",
		TmpDir:     t.dataDir,
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
			TmpDir:  v.DataDir,
		}
	}

	for k, v := range t.inputs {
		_v, _ := InterfaceToAny(v)
		vars.Inputs[k] = _v
	}

	for k, v := range t.Matrix {
		_v, _ := InterfaceToAny(v)
		vars.Matrix[k] = _v
	}

	return vars
}

func (t StepContext) RuntimeVars() map[string]interface{} {
	return map[string]interface{}{
		"context": t.ToV1Beta1(),
	}
}

// Convert interface{} to *anypb.Any (packing)
func InterfaceToAny(i interface{}) (*anypb.Any, error) {
	switch v := i.(type) {
	case string:
		return anypb.New(wrapperspb.String(v))
	case int:
		return anypb.New(wrapperspb.Int64(int64(v)))
	case float64:
		return anypb.New(wrapperspb.Double(v))
	case bool:
		return anypb.New(wrapperspb.Bool(v))

	// Handle list types explicitly
	case []string:
		list := &structpb.ListValue{}
		for _, item := range v {
			list.Values = append(list.Values, structpb.NewStringValue(item))
		}
		return anypb.New(list)

	case []int:
		list := &structpb.ListValue{}
		for _, item := range v {
			list.Values = append(list.Values, structpb.NewNumberValue(float64(item)))
		}
		return anypb.New(list)

	case []float64:
		list := &structpb.ListValue{}
		for _, item := range v {
			list.Values = append(list.Values, structpb.NewNumberValue(item))
		}
		return anypb.New(list)

	case []bool:
		list := &structpb.ListValue{}
		for _, item := range v {
			list.Values = append(list.Values, structpb.NewBoolValue(item))
		}
		return anypb.New(list)
	}

	// Fallback: use structpb.NewValue for generic cases
	val, err := structpb.NewValue(i)
	if err != nil {
		return nil, fmt.Errorf("failed to convert interface{} to Value: %w", err)
	}
	return anypb.New(val)
}
