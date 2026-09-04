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
	// TaskGroups holds every matrix-instance context for a task that ran as
	// a matrix, keyed by the task's plain name. Tasks[name] only ever keeps
	// the last instance to finish, which loses the other combinations; a
	// consumer that needs to fan out over all of them (e.g. sources.go
	// copying from every matrix instance) reads TaskGroups instead.
	TaskGroups map[string][]*TaskContext `json:"-"`
	Labels     LabelsContext
	Display    DisplayContext
	Style      StyleContext
	EnvVars    EnvVarsContext
	SecretVars SecretVarsContext
	InputVars  InputVarsContext
	Matrix     MatrixContext
	Build      BuildContext
	Workdir    WorkdirContext
	Service    ServiceContext
	Stats      StatsContext
	Ancestors  AncestorsContext
	mu         *sync.Mutex
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
		TaskGroups: make(map[string][]*TaskContext),
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
	copy.TaskGroups = maps.Clone(c.TaskGroups)
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
	copy.Build.DebugState = c.Build.DebugState
	copy.Workdir.Path = c.Workdir.Path
	copy.Service.NetIP = c.Service.NetIP
	copy.Style.Style = c.Style.Style
	copy.Ancestors.Refs = append(copy.Ancestors.Refs, c.Ancestors.Refs...)

	return copy
}

// Merge folds a dependency/child task's context into t, e.g. so a later
// sibling's CEL expressions can reference $(tasks.<name>...). It's additive
// only: a key t already has (its own env/secret/input, set from its own
// image or spec) is never overwritten by c's value for that same key. This
// matters because DependsOn's fan-out (see depends_on.go) merges every
// dependent's result back into the task that launched it — without the
// additive guard, a dependent using a completely different base image (e.g.
// docker-build's docker:27-cli after build's golang:*-alpine) would clobber
// build's own PATH with its own.
func (t TaskContext) Merge(c TaskContext) TaskContext {
	mergeAdditive(t.EnvVars.Envs, c.EnvVars.Envs)
	mergeAdditive(t.SecretVars.Secrets, c.SecretVars.Secrets)
	mergeAdditive(t.InputVars.Inputs, c.InputVars.Inputs)
	maps.Copy(t.Tasks, c.Tasks)

	for name, instances := range c.TaskGroups {
		if _, exists := t.TaskGroups[name]; !exists {
			t.TaskGroups[name] = instances
		}
	}

	return t
}

// mergeAdditive copies entries from src into dst, skipping any key dst
// already has.
func mergeAdditive[K comparable, V any](dst, src map[K]V) {
	for k, v := range src {
		if _, exists := dst[k]; !exists {
			dst[k] = v
		}
	}
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
			StartedAt: metav1.Time{Time: v.StartedAt},
			EndedAt:   metav1.Time{Time: v.EndedAt},
		}

		if v.Error != nil {
			vars.Tasks[k].Error = v.Error.Error()
		}
	}

	return vars
}
