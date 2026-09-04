package processor

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/moby/buildkit/client/llb"
	gwclient "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/solver/pb"
	"github.com/moby/term"
	"github.com/raffis/rageta/internal/utils"
	"github.com/raffis/rageta/internal/xio"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithInteractive(enabled bool, gwClient gwclient.Client) ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if !enabled {
			return nil
		}

		return &Interactive{
			gwClient: gwClient,
		}
	}
}

type Interactive struct {
	gwClient gwclient.Client
}

func (s *Interactive) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		ctx, err := next(ctx)
		if err == nil {
			return ctx, nil
		}

		terminalErr := ctx.Display.Interrupt(func(stdin io.Reader, stdout, stderr io.Writer) error {
			return RunDebugShell(ctx, s.gwClient, ctx, stdin, stdout, stderr)
		})

		if terminalErr != nil {
			return ctx, terminalErr
		}

		return ctx, err
	}, nil
}

func RunDebugShell(ctx context.Context, gwClient gwclient.Client, stepCtx TaskContext, stdin io.Reader, stdout, stderr io.Writer) error {
	stepCtx = stepCtx.DeepCopy()

	// A task with exports: has Build.State shrunk down to just the exported
	// paths (see Exports.Bootstrap), for cross-task artifact sharing. For
	// debugging we want the task's own full environment instead.
	buildState := stepCtx.Build.State
	if stepCtx.Build.DebugState != nil {
		buildState = *stepCtx.Build.DebugState
	}

	def, err := buildState.Marshal(ctx)
	if err != nil {
		return err
	}

	rootRes, err := gwClient.Solve(ctx, gwclient.SolveRequest{Definition: def.ToPB()})
	if err != nil {
		return fmt.Errorf("solve debug root: %w", err)
	}

	mounts := []gwclient.Mount{
		{
			Dest:      "/",
			MountType: pb.MountType_BIND,
			Ref:       rootRes.Ref,
		},
	}

	var contextRef gwclient.Reference
	for _, mount := range stepCtx.Build.Mounts {
		switch {
		case mount.HostPath != nil:
			if contextRef == nil {
				contextDef, err := llb.Local("context").Marshal(ctx)
				if err != nil {
					return err
				}

				contextRes, err := gwClient.Solve(ctx, gwclient.SolveRequest{Definition: contextDef.ToPB()})
				if err != nil {
					return fmt.Errorf("solve debug context: %w", err)
				}
				contextRef = contextRes.Ref
			}

			mounts = append(mounts, gwclient.Mount{
				Dest:      mount.MountPath,
				MountType: pb.MountType_BIND,
				Ref:       contextRef,
				Selector:  mount.HostPath.Path,
				Readonly:  mount.ReadOnly,
			})
		case mount.Cache != nil:
			sharing := pb.CacheSharingOpt_SHARED
			switch strings.ToLower(strings.TrimSpace(mount.Cache.Sharing)) {
			case "private":
				sharing = pb.CacheSharingOpt_PRIVATE
			case "locked":
				sharing = pb.CacheSharingOpt_LOCKED
			}

			mounts = append(mounts, gwclient.Mount{
				Dest:      mount.MountPath,
				MountType: pb.MountType_CACHE,
				CacheOpt: &pb.CacheOpt{
					ID:      mount.Cache.Name,
					Sharing: sharing,
				},
				Readonly: mount.ReadOnly,
			})
		case mount.TmpFS != nil:
			mounts = append(mounts, gwclient.Mount{
				Dest:      mount.MountPath,
				MountType: pb.MountType_TMPFS,
				Readonly:  mount.ReadOnly,
			})
		}
	}

	ctr, err := gwClient.NewContainer(ctx, gwclient.NewContainerRequest{Mounts: mounts})
	if err != nil {
		return fmt.Errorf("create debug container: %w", err)
	}
	defer ctr.Release(ctx)

	/*for name, service := range stepCtx.Services.Status {
		envName := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
		stepCtx.EnvVars.Envs[fmt.Sprintf("SERVICE_%s", envName)] = service.ContainerIP
	}*/

	stepCtx.EnvVars.Envs["PS1"] = fmt.Sprintf("%s$ ", stepCtx.Style.Style.Render(stepCtx.UniqueName()))
	stepCtx.EnvVars.Envs["HISTFILE"] = "/rageta/ash_history"

	env := utils.EnvSlice(stepCtx.EnvVars.Envs)

	stdinRC, ok := stdin.(io.ReadCloser)
	if !ok {
		stdinRC = io.NopCloser(stdin)
	}

	proc, err := ctr.Start(ctx, gwclient.StartRequest{
		Args:   []string{"/bin/ash"},
		Env:    env,
		Cwd:    stepCtx.Workdir.Path,
		Tty:    true,
		Stdin:  stdinRC,
		Stdout: xio.NopWriteCloser{Writer: stdout},
		Stderr: xio.NopWriteCloser{Writer: stderr},
	})
	if err != nil {
		return fmt.Errorf("start debug shell: %w", err)
	}

	if f, ok := stdin.(*os.File); ok {
		if oldState, rawErr := term.MakeRaw(f.Fd()); rawErr == nil {
			defer term.RestoreTerminal(f.Fd(), oldState)
		}

		// Apply the current window size synchronously before anything can
		// run inside the shell. gwclient.StartRequest has no way to set an
		// initial winsize, so without this the container's pty starts out
		// at whatever default size BuildKit picked; a pager launched inside
		// the shell (e.g. `less`) reads that size once at startup via
		// TIOCGWINSZ, so it would otherwise see the wrong dimensions until
		// an actual terminal resize happens to fire a real SIGWINCH.
		if ws, err := term.GetWinsize(f.Fd()); err == nil {
			_ = proc.Resize(ctx, gwclient.WinSize{Rows: uint32(ws.Height), Cols: uint32(ws.Width)})
		}

		resize := make(chan os.Signal, 1)
		signal.Notify(resize, syscall.SIGWINCH)
		defer signal.Stop(resize)

		go func() {
			for range resize {
				if ws, err := term.GetWinsize(f.Fd()); err == nil {
					_ = proc.Resize(ctx, gwclient.WinSize{Rows: uint32(ws.Height), Cols: uint32(ws.Width)})
				}
			}
		}()
	}

	return proc.Wait()
}
