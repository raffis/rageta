package run

import (
	"context"
	"errors"
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
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/setup/flagset"
	"github.com/raffis/rageta/internal/utils"
	"github.com/spf13/pflag"
)

type TaskInteractive string

var (
	TaskInteractiveNever       TaskInteractive = "Never"
	TaskInteractiveAsk         TaskInteractive = "Ask"
	TaskInteractiveAskIfFailed TaskInteractive = "AskIfFailed"
	TaskInteractiveIfFailed    TaskInteractive = "IfFailed"
	TaskInteractiveAlways      TaskInteractive = "Always"
)

func (s TaskInteractive) String() string {
	return string(s)
}

func NewInteractiveOptions() InteractiveOptions {
	return InteractiveOptions{
		Interactive: string(TaskInteractiveNever),
	}
}

type InteractiveOptions struct {
	Interactive string
}

func (s *InteractiveOptions) BindFlags(flags flagset.Interface) {
	flags.StringVarP(&s.Interactive, "interactive", "i", s.Interactive, "Exec a shell in failed tasks. The task is exported and executed as a container with its entire state and the current tty is attached directly to a /bin/ash shell within the failed task.")
	if fs, ok := flags.(interface{ Lookup(string) *pflag.Flag }); ok {
		if f := fs.Lookup("interactive"); f != nil {
			f.NoOptDefVal = string(TaskInteractiveIfFailed)
		}
	}
}

func (s InteractiveOptions) Build() Task {
	return &Interactive{opts: s}
}

type Interactive struct {
	opts InteractiveOptions
}

func (s *Interactive) Label() string {
	return "Setting up interactive mode"
}

func (s *Interactive) Run(rc *RunContext, next Next) error {
	err := next(rc)
	return s.walkError(rc, err)
}

func (s *Interactive) walkError(rc *RunContext, err error) error {
	unwrappedErr := err
	for unwrappedErr != nil {
		if uw, ok := unwrappedErr.(interface{ Unwrap() []error }); ok {
			for _, unwrappedErr := range uw.Unwrap() {
				return s.walkError(rc, unwrappedErr)
			}

			return err
		}

		unwrappedErr = errors.Unwrap(unwrappedErr)
	}

	if err := s.openInteractive(rc, err); err != nil {
		return err
	}

	return err
}

func (s *Interactive) openInteractive(rc *RunContext, err error) error {
	var innerTaskErr processor.TaskError
	if !AsInner(err, &innerTaskErr) {
		return err
	}

	var exitCodeErr processor.ExitCode
	if !AsInner(err, &exitCodeErr) {
		return err
	}

	switch {
	case s.opts.Interactive == TaskInteractiveNever.String():
		return nil
	case s.opts.Interactive == TaskInteractiveIfFailed.String():

	case s.opts.Interactive == TaskInteractiveAsk.String():
		return nil
	case s.opts.Interactive == TaskInteractiveAskIfFailed.String():
		return nil
	}

	ctx := context.Background()
	return RunDebugShell(ctx, rc, innerTaskErr.Context(), os.Stdin, os.Stdout, os.Stderr)
}

// RunDebugShell drops an interactive shell into the failed step's root
// filesystem using BuildKit's own gateway container (gwclient.NewContainer)
// rather than exporting an image and running it through a separate
// container runtime. This keeps the step's volume/cache mounts (which only
// exist as BuildKit Run-op mounts, not as part of the exported rootfs)
// available inside the debug shell.
func RunDebugShell(ctx context.Context, rc *RunContext, stepCtx processor.TaskContext, stdin io.Reader, stdout, stderr io.Writer) error {
	stepCtx = stepCtx.DeepCopy()
	gwClient := rc.Buildkit.GatewayClient

	def, err := stepCtx.Build.State.Marshal(ctx)
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
		Stdout: nopWriteCloser{stdout},
		Stderr: nopWriteCloser{stderr},
	})
	if err != nil {
		return fmt.Errorf("start debug shell: %w", err)
	}

	if f, ok := stdin.(*os.File); ok {
		if oldState, rawErr := term.MakeRaw(f.Fd()); rawErr == nil {
			defer term.RestoreTerminal(f.Fd(), oldState)
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
		resize <- syscall.SIGWINCH
	}

	return proc.Wait()
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }
