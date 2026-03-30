package processor

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path"
	"runtime"
	"sync"
	"time"

	cruntime "github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

var (
	tagColors = make(map[Tag]string)
	tagMutex  = sync.Mutex{}
)

type StepContext struct {
	context.Context `json:"-"`
	uniqueID        string
	uniqueName      string
	namespace       string
	Error           error
	StartedAt       time.Time
	EndedAt         time.Time
	ContextDir      string
	Steps           map[string]*StepContext `json:"-"`
	Containers      map[string]cruntime.ContainerStatus
	Tags            TagsContext
	Streams         StreamsContext
	OutputVars      OutputVarsContext
	EnvVars         EnvVarsContext
	SecretVars      SecretVarsContext
	InputVars       InputVarsContext
	Template        TemplateContext
	Matrix          MatrixContext
	Events          EventsContext
}

func (c StepContext) UniqueID() string {
	return c.uniqueID
}

func (c StepContext) UniqueName() string {
	return c.uniqueID
}

func (c StepContext) WithNamespace(name string) StepContext {
	copy := c

	if copy.namespace == "" {
		copy.namespace = name
		return copy
	}

	copy.namespace = fmt.Sprintf("%s-%s", copy.namespace, name)
	return copy
}

func NewContext() StepContext {
	return StepContext{
		EnvVars:    newEnvVarsContext(),
		SecretVars: newSecretVarsContext(),
		InputVars:  newInputVarsContext(),
		Matrix:     newMatrixContext(),
		OutputVars: newOutputVarsContext(),
		Events:     newEventsContext(),
		Steps:      make(map[string]*StepContext),
		Containers: make(map[string]cruntime.ContainerStatus),
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
	copy.Outputs = append(copy.Outputs, c.Outputs...)
	copy.OutputVars = maps.Clone(c.OutputVars)
	copy.Steps = maps.Clone(c.Steps)
	copy.tags = append(copy.tags, c.tags...)
	copy.Inputs = maps.Clone(c.Inputs)
	copy.Envs = maps.Clone(c.Envs)
	copy.Secrets = maps.Clone(c.Secrets)
	copy.Containers = maps.Clone(c.Containers)
	copy.Matrix = maps.Clone(c.Matrix)
	if c.Template != nil {
		copy.Template = c.Template.DeepCopy()
	}

	return copy
}

func (t StepContext) Merge(c StepContext) StepContext {
	maps.Copy(t.Envs, c.Envs)
	maps.Copy(t.Secrets, c.Secrets)
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
		TmpDir:     path.Join(t.ContextDir, t.UniqueID(), "data"),
		Steps:      make(map[string]*v1beta1.StepResult),
		Containers: make(map[string]*v1beta1.ContainerStatus),
		Matrix:     maps.Clone(t.Matrix.Params),
		Envs:       maps.Clone(t.EnvVars.Envs),
		Secrets:    maps.Clone(t.SecretVars.Secrets),
		Inputs:     maps.Clone(t.InputVars.Inputs),
		Env:        t.EnvVars.OutputPath,
		Secret:     t.SecretVars.OutputPath,
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
			TmpDir:  path.Join(v.ContextDir, v.UniqueID(), "data"),
		}

		maps.Copy(vars.Steps[k].Outputs, v.OutputVars.OutputVars)
	}

	for _, v := range t.OutputVars.Outputs {
		vars.Outputs[v.Name] = &v1beta1.Output{
			Path: v.Path,
		}
	}
	return vars
}
