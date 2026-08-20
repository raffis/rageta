package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/moby/buildkit/client/llb"
	gwclient "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/raffis/rageta/internal/secrets"
	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithSteps(store secrets.Interface, noCache bool) ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if spec.Steps == nil {
			return nil
		}
		return &Steps{
			steps:    *spec.Steps,
			taskName: spec.Name,
			store:    store,
			noCache:  noCache,
		}
	}
}

const (
	defaultShell   = "/bin/ash"
	contextPath    = "/rageta/context.json"
	ashHistoryPath = "/rageta/ash_history"
	// ContextSecretPrefix marks secrets holding a task's serialized context.json rather than a user
	// secret. These live in the same store as user secrets so buildkit can resolve them by ID, but
	// must never be echoed back into a task's own SecretVars (see SecretVars.Bootstrap) - otherwise
	// each task's context.json would embed every prior task's context.json, growing without bound.
	ContextSecretPrefix = "rageta-context-"
)

type Steps struct {
	steps    []v1beta1.Step
	taskName string
	store    secrets.Interface
	noCache  bool
}

// Bootstrap chains every step's script execution onto ctx.Build.State as its
// own llb.Run() (and therefore its own image layer) before handing off to
// the Build processor. All steps are solved together in a single Solve()
// call so buildkit's progress display keeps one continuous vertex sequence
// instead of restarting at #1 for every step.
func (s *Steps) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		ctx.Build.State = ctx.Build.State.File(
			llb.Mkdir("/rageta", 0755),
		)

		contextJSON, err := json.MarshalIndent(ctx.ToV1Beta1(), "", "  ")
		if err != nil {
			return ctx, err
		}

		secretID := ContextSecretPrefix + s.taskName
		s.store.AddSecret(context.Background(), secretID, contextJSON)
		contextSecretOpt := llb.AddSecret(contextPath, llb.SecretID(secretID))

		baseRunOpts := append([]llb.RunOption{contextSecretOpt}, ctx.Build.RunOpts...)
		if s.noCache {
			baseRunOpts = append(baseRunOpts, llb.IgnoreCache)
		}

		var history []string

		for k, step := range s.steps {
			script := step.Script
			if err := substitute.Substitute(ctx.ToV1Beta1(), &script); err != nil {
				return ctx, err
			}
			script = strings.TrimSpace(script)

			interpreter := defaultShell
			if strings.HasPrefix(script, "#!") {
				lines := strings.SplitN(script, "\n", 2)
				interpreter = strings.TrimSpace(strings.TrimPrefix(lines[0], "#!"))
			}

			scriptEntrypointPath := fmt.Sprintf("/rageta/script-entrypoint-%d.sh", k)
			scriptPath := fmt.Sprintf("/rageta/script-%d.sh", k)
			exitCodePath := fmt.Sprintf("/rageta/exitcode-%d", k)

			ctx.Build.State = ctx.Build.State.File(
				llb.Mkfile(scriptPath, 0755, []byte(script)),
			)

			// We need the script to always exit 0 in order to get the filesystem state even in case of an error
			ctx.Build.State = ctx.Build.State.File(
				llb.Mkfile(scriptEntrypointPath, 0755, []byte(fmt.Sprintf("/bin/ash -e -x  %s; echo $? > %s", scriptPath, exitCodePath))),
			)

			history = append(history, scriptPath)
			stepRunOpts := append(append([]llb.RunOption{}, baseRunOpts...), llb.Args([]string{
				shimPath,
				"-stats",
				interpreter,
				scriptEntrypointPath,
			}))

			ctx.Build.State = ctx.Build.State.Run(stepRunOpts...).Root()
		}

		ctx, err = next(ctx)
		if err != nil {
			return ctx, err
		}

		ctx.Build.State = ctx.Build.State.File(
			llb.Mkfile(ashHistoryPath, 0755, []byte(strings.Join(history, "\n"))),
		)

		for k := range s.steps {
			exitCodePath := fmt.Sprintf("/rageta/exitcode-%d", k)
			data, err := ctx.Build.Ref.ReadFile(ctx, gwclient.ReadRequest{Filename: exitCodePath})
			if err != nil {
				return ctx, err
			}

			code, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				return ctx, fmt.Errorf("invalid exit code %q from step #%d: %w", string(data), k, err)
			}

			if code != 0 {
				return ctx, &scriptError{
					exitCode: code,
					parent:   fmt.Errorf("step #%d exited with code %d", k, code),
				}
			}
		}

		return ctx, nil
	}, nil
}

type scriptError struct {
	exitCode int
	parent   error
}

func (e *scriptError) Error() string {
	return fmt.Sprintf("script failed: %s", e.parent.Error())
}

func (e *scriptError) Unwrap() error {
	return e.parent
}

func (e *scriptError) ExitCode() int {
	return e.exitCode
}
