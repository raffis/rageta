package processor

import (
	"context"
	"fmt"
	"maps"
	"os"
	"runtime"
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
	ContextDir      string
	Tasks           map[string]*TaskContext `json:"-"`
	Tags            TagsContext
	Streams         StreamsContext
	Style           StyleContext
	EnvVars         EnvVarsContext
	SecretVars      SecretVarsContext
	InputVars       InputVarsContext
	Matrix          MatrixContext
	Events          EventsContext
	Build           BuildContext
	Workdir         WorkdirContext
	Services        ServiceContext
}

func (c TaskContext) UniqueID() string {
	return c.uniqueID
}

func (c TaskContext) UniqueName() string {
	return c.uniqueName
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
		Events:     newEventsContext(),
		Services:   newServiceContext(),
		Tasks:      make(map[string]*TaskContext),
	}
}

func (c TaskContext) DeepCopy() TaskContext {
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
	copy.Tasks = maps.Clone(c.Tasks)
	copy.Tags.tags = append(copy.Tags.tags, c.Tags.tags...)
	copy.InputVars.Inputs = maps.Clone(c.InputVars.Inputs)
	copy.EnvVars.Envs = maps.Clone(c.EnvVars.Envs)
	copy.SecretVars.Secrets = maps.Clone(c.SecretVars.Secrets)
	copy.Matrix.Params = maps.Clone(c.Matrix.Params)
	copy.Build.RunOpts = append(copy.Build.RunOpts, c.Build.RunOpts...)
	copy.Build.State = c.Build.State
	copy.Build.Ref = c.Build.Ref
	copy.Workdir.Path = c.Workdir.Path
	copy.Services.Status = maps.Clone(c.Services.Status)
	copy.Style.Style = c.Style.Style

	return copy
}

func (t TaskContext) Merge(c TaskContext) TaskContext {
	maps.Copy(t.EnvVars.Envs, c.EnvVars.Envs)
	maps.Copy(t.SecretVars.Secrets, c.SecretVars.Secrets)
	maps.Copy(t.InputVars.Inputs, c.InputVars.Inputs)
	maps.Copy(t.Tasks, c.Tasks)
	maps.Copy(t.Services.Status, c.Services.Status)

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
