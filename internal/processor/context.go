package processor

import (
	"context"
	"fmt"
	"maps"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TaskContext struct {
	context.Context `json:"-"`
	uniqueID        string
	uniqueName      string
	namespace       string
	Error           error
	StartedAt       time.Time
	EndedAt         time.Time
	Tasks           map[string]*TaskContext `json:"-"`
	Labels          LabelsContext
	Display         DisplayContext
	Style           StyleContext
	EnvVars         EnvVarsContext
	SecretVars      SecretVarsContext
	InputVars       InputVarsContext
	Matrix          MatrixContext
	Build           BuildContext
	Workdir         WorkdirContext
	Service         ServiceContext
	Stats           StatsContext
	Parent          ParentContext
	mu              *sync.Mutex
}

func (c TaskContext) UniqueID() string {
	return c.uniqueID
}

func (c TaskContext) UniqueName() string {
	return c.uniqueName
}

// Namespace identifies the pipeline-instance scope a context belongs to
// (e.g. a specific matrix combination or inherited pipeline invocation).
// Two contexts sharing the same namespace refer to the same logical run of
// a task; different namespaces are independent runs.
func (c TaskContext) Namespace() string {
	return c.namespace
}

func (c TaskContext) WithNamespace(name string) TaskContext {
	copy := c

	if copy.namespace == "" {
		copy.namespace = name
		return copy
	}

	copy.namespace = fmt.Sprintf("%s-%s", copy.namespace, name)
	return copy
}

func NewContext() TaskContext {
	return TaskContext{
		EnvVars:    newEnvVarsContext(),
		SecretVars: newSecretVarsContext(),
		Build:      newBuildContext(),
		InputVars:  newInputVarsContext(),
		Matrix:     newMatrixContext(),
		Stats:      newStatsContext(),
		Tasks:      make(map[string]*TaskContext),
		mu:         &sync.Mutex{},
	}
}

func (c TaskContext) DeepCopy() TaskContext {
	copy := NewContext()
	c.mu.Lock()
	defer c.mu.Unlock()

	copy.uniqueID = c.uniqueID
	copy.uniqueName = c.uniqueName
	copy.namespace = c.namespace
	copy.Context = c.Context
	copy.Display.Stdout = c.Display.Stdout
	copy.Display.Stderr = c.Display.Stderr
	copy.Display.Demuxer = c.Display.Demuxer
	copy.Display.Events = c.Display.Events
	copy.Display.WriteStats = c.Display.WriteStats
	copy.Display.WritePullProgress = c.Display.WritePullProgress
	copy.Tasks = maps.Clone(c.Tasks)
	copy.Labels.labels = append(copy.Labels.labels, c.Labels.labels...)
	copy.InputVars.Inputs = maps.Clone(c.InputVars.Inputs)
	copy.EnvVars.Envs = maps.Clone(c.EnvVars.Envs)
	copy.SecretVars.Secrets = maps.Clone(c.SecretVars.Secrets)
	copy.Matrix.Params = maps.Clone(c.Matrix.Params)
	copy.Build.RunOpts = append(copy.Build.RunOpts, c.Build.RunOpts...)
	copy.Build.Mounts = append(copy.Build.Mounts, c.Build.Mounts...)
	copy.Build.State = c.Build.State
	copy.Build.ContextState = c.Build.ContextState
	copy.Build.Ref = c.Build.Ref
	copy.Workdir.Path = c.Workdir.Path
	copy.Service.NetIP = c.Service.NetIP
	copy.Style.Style = c.Style.Style
	copy.Parent.Refs = append(copy.Parent.Refs, c.Parent.Refs...)

	return copy
}

func (t TaskContext) Merge(c TaskContext) TaskContext {
	maps.Copy(t.EnvVars.Envs, c.EnvVars.Envs)
	maps.Copy(t.SecretVars.Secrets, c.SecretVars.Secrets)
	maps.Copy(t.InputVars.Inputs, c.InputVars.Inputs)
	maps.Copy(t.Tasks, c.Tasks)

	return t
}

func (t TaskContext) FromV1Beta1(vars *v1beta1.Context) {
}

func (t TaskContext) ToV1Beta1() *v1beta1.Context {
	vars := &v1beta1.Context{
		Tasks:   make(map[string]*v1beta1.TaskResult),
		Matrix:  maps.Clone(t.Matrix.Params),
		Envs:    maps.Clone(t.EnvVars.Envs),
		Secrets: maps.Clone(t.SecretVars.Secrets),
		Inputs:  maps.Clone(t.InputVars.Inputs),
		Os:      runtime.GOOS,
		Arch:    runtime.GOARCH,
		Uid:     fmt.Sprintf("%d", os.Getuid()),
		Guid:    fmt.Sprintf("%d", os.Getgid()),
	}

	for k, v := range t.Tasks {
		vars.Tasks[k] = &v1beta1.TaskResult{
			Outputs:   make(map[string]v1beta1.ParamValue),
			StartedAt: metav1.Time{Time: v.StartedAt},
			EndedAt:   metav1.Time{Time: v.EndedAt},
		}

		if v.Error != nil {
			vars.Tasks[k].Error = v.Error.Error()
		}

		//	maps.Copy(vars.Tasks[k].Outputs, v.OutputVars.OutputVars)
	}

	return vars
}
