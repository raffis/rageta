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

	stepCtx := innerTaskErr.Context().DeepCopy()
	def, marshalErr := stepCtx.Build.State.Marshal(ctx)
	if marshalErr != nil {
		return marshalErr
	}

	const imageName = "rageta-shell-debug:latest"

	pr, pw := io.Pipe()
	loadCmd := exec.CommandContext(ctx, "docker", "load")
	loadCmd.Stdin = pr
	loadCmd.Stdout = os.Stdout
	loadCmd.Stderr = os.Stderr

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

	fd := os.Stdin.Fd()
	if oldState, rawErr := term.MakeRaw(fd); rawErr == nil {
		defer term.RestoreTerminal(fd, oldState)
	}

	await, err := rc.ContainerRuntime.Driver.CreatePod(ctx, pod, os.Stdin, os.Stdout, os.Stderr)
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
