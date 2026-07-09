package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/term"
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/internal/setup/flagset"
	"github.com/raffis/rageta/internal/utils"
	"github.com/spf13/pflag"
	"github.com/tonistiigi/fsutil"
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

// RunDebugShell exports the buildkit state of stepCtx as a docker image, starts it as a
// container with a /bin/ash shell attached to the given stdio streams, and waits for the
// shell to exit. It is used both by the -i/--interactive CLI flag and by the TUI's debug
// shell key binding.
func RunDebugShell(ctx context.Context, rc *RunContext, stepCtx processor.TaskContext, stdin io.Reader, stdout, stderr io.Writer) error {
	stepCtx = stepCtx.DeepCopy()
	def, marshalErr := stepCtx.Build.State.Marshal(ctx)
	if marshalErr != nil {
		return marshalErr
	}

	const imageName = "rageta-shell-debug:latest"

	if err := exportDebugImage(ctx, rc, def, stdout, stderr, imageName); err != nil {
		return err
	}

	for name, service := range stepCtx.Services.Status {
		//ctx.Build.State = ctx.Build.State.AddExtraHost(name, net.IP(service.ContainerIP))
		envName := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
		stepCtx.EnvVars.Envs[fmt.Sprintf("SERVICE_%s", envName)] = service.ContainerIP
	}

	stepCtx.EnvVars.Envs["PS1"] = fmt.Sprintf("%s$ ", stepCtx.Style.Style.Render(stepCtx.UniqueName()))
	stepCtx.EnvVars.Envs["HISTFILE"] = "/rageta/ash_history"

	pod := &runtime.Pod{
		Name: fmt.Sprintf("rageta-%s", utils.RandString(5)),
		Spec: runtime.PodSpec{
			Containers: []runtime.ContainerSpec{
				{
					Name:    stepCtx.UniqueID(),
					Image:   imageName,
					Env:     stepCtx.EnvVars.Envs,
					PWD:     stepCtx.Workdir.Path,
					Stdin:   true,
					TTY:     true,
					Command: []string{"/bin/ash"},
				},
			},
		},
	}

	if f, ok := stdin.(*os.File); ok {
		if oldState, rawErr := term.MakeRaw(f.Fd()); rawErr == nil {
			defer term.RestoreTerminal(f.Fd(), oldState)
		}
	}

	await, err := rc.ContainerRuntime.Driver.CreatePod(ctx, pod, stdin, stdout, stderr)
	if err != nil {
		return err
	}

	if err := await.Wait(ctx); err != nil {
		return err
	}

	rc.Teardown.Teardown <- func(teardownCtx context.Context, timeout time.Duration) error {
		return rc.ContainerRuntime.Driver.DeletePod(teardownCtx, pod, timeout)
	}

	return err
}

// exportDebugImage makes the built image available under imageName to the
// configured container runtime. When the runtime reads images directly out of
// a containerd content/image store shared with buildkit (see
// runtime.ContainerdBacked), the image is written straight into that store.
// Otherwise it falls back to exporting a docker image tarball and `docker load`.
func exportDebugImage(ctx context.Context, rc *RunContext, def *llb.Definition, stdout, stderr io.Writer, imageName string) error {
	if _, ok := rc.ContainerRuntime.Driver.(runtime.ContainerdBacked); ok {
		_, err := rc.Buildkit.Client.Solve(ctx, def, client.SolveOpt{
			LocalMounts: map[string]fsutil.FS{
				"context": rc.Buildkit.ContextFS,
			},
			Exports: []client.ExportEntry{
				{
					Type: client.ExporterImage,
					Attrs: map[string]string{
						"name": imageName,
					},
				},
			},
		}, nil)

		if err != nil {
			return fmt.Errorf("export image: %w", err)
		}

		return nil
	}

	pr, pw := io.Pipe()
	loadCmd := exec.CommandContext(ctx, "docker", "load")
	loadCmd.Stdin = pr
	loadCmd.Stdout = stdout
	loadCmd.Stderr = stderr

	loadErrCh := make(chan error, 1)
	go func() {
		loadErrCh <- loadCmd.Run()
		pr.Close()
	}()

	_, solveErr := rc.Buildkit.Client.Solve(ctx, def, client.SolveOpt{
		LocalMounts: map[string]fsutil.FS{
			"context": rc.Buildkit.ContextFS,
		},
		Exports: []client.ExportEntry{
			{
				Type: client.ExporterDocker,
				Attrs: map[string]string{
					"name": imageName,
				},
				Output: func(map[string]string) (io.WriteCloser, error) {
					return pw, nil
				},
			},
		},
	}, nil)

	pw.Close()
	loadErr := <-loadErrCh

	if solveErr != nil {
		return fmt.Errorf("export image: %w", solveErr)
	}

	if loadErr != nil {
		return fmt.Errorf("docker load: %w", loadErr)
	}

	return nil
}
