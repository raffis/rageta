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

	"github.com/moby/buildkit/client/llb"
	cruntime "github.com/raffis/rageta/internal/runtime"
	"github.com/tonistiigi/fsutil"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	LLBState        *llb.State             `json:"-"`
	LocalMounts     map[string]fsutil.FS   `json:"-"`
	Containers      map[string]cruntime.ContainerStatus
	Tags            TagsContext
	Streams         StreamsContext
	OutputVars      OutputVarsContext
	EnvVars         EnvVarsContext
	SecretVars      SecretVarsContext
	InputVars       InputVarsContext
	Matrix          MatrixContext
	Events          EventsContext
}

func (c StepContext) UniqueID() string {
	return c.uniqueID
}

func (c StepContext) UniqueName() string {
	return c.uniqueName
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
		Steps:       make(map[string]*StepContext),
		Containers:  make(map[string]cruntime.ContainerStatus),
		LocalMounts: make(map[string]fsutil.FS),
	}
}

func (c StepContext) DeepCopy() StepContext {
	copy := NewContext()
	copy.uniqueID = c.uniqueID
	copy.uniqueName = c.uniqueName
	copy.namespace = c.namespace
	copy.Context = c.Context
	copy.ContextDir = c.ContextDir
	copy.Streams.Stdout = c.Streams.Stdout
	copy.Streams.Stderr = c.Streams.Stderr
	copy.Streams.Stdin = c.Streams.Stdin
	copy.Streams.AdditionalStdout = append(copy.Streams.AdditionalStdout, c.Streams.AdditionalStdout...)
	copy.Streams.AdditionalStderr = append(copy.Streams.AdditionalStderr, c.Streams.AdditionalStderr...)
	copy.OutputVars.Outputs = append(copy.OutputVars.Outputs, c.OutputVars.Outputs...)
	copy.OutputVars.OutputVars = maps.Clone(c.OutputVars.OutputVars)
	copy.Steps = maps.Clone(c.Steps)
	copy.LLBState = c.LLBState
	copy.LocalMounts = maps.Clone(c.LocalMounts)
	copy.Tags.tags = append(copy.Tags.tags, c.Tags.tags...)
	copy.InputVars.Inputs = maps.Clone(c.InputVars.Inputs)
	copy.EnvVars.Envs = maps.Clone(c.EnvVars.Envs)
	copy.SecretVars.Secrets = maps.Clone(c.SecretVars.Secrets)
	copy.Containers = maps.Clone(c.Containers)
	copy.Matrix.Params = maps.Clone(c.Matrix.Params)

	return copy
}

func (t StepContext) Merge(c StepContext) StepContext {
	maps.Copy(t.EnvVars.Envs, c.EnvVars.Envs)
	maps.Copy(t.SecretVars.Secrets, c.SecretVars.Secrets)
	maps.Copy(t.InputVars.Inputs, c.InputVars.Inputs)
	maps.Copy(t.Steps, c.Steps)
	maps.Copy(t.Containers, c.Containers)
	maps.Copy(t.LocalMounts, c.LocalMounts)

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
			Outputs:   make(map[string]v1beta1.ParamValue),
			TmpDir:    path.Join(v.ContextDir, v.UniqueID(), "data"),
			StartedAt: metav1.Time{Time: v.StartedAt},
			EndedAt:   metav1.Time{Time: v.EndedAt},
		}

		if v.Error != nil {
			vars.Steps[k].Error = v.Error.Error()
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
