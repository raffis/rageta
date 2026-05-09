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
	"github.com/tonistiigi/fsutil"
)

type StepTerminal string

var (
	StepTerminalNever       StepTerminal = "Never"
	StepTerminalAsk         StepTerminal = "Ask"
	StepTerminalAskIfFailed StepTerminal = "AskIfFailed"
	StepTerminalIfFailed    StepTerminal = "IfFailed"
	StepTerminalAlways      StepTerminal = "Always"
)

func (s StepTerminal) String() string {
	return string(s)
}

func NewTerminalOptions() TerminalOptions {
	return TerminalOptions{
		Terminal: string(StepTerminalNever),
	}
}

type TerminalOptions struct {
	Terminal string
}

func (s *TerminalOptions) BindFlags(flags flagset.Interface) {
	flags.StringVarP(&s.Terminal, "terminal", "", s.Terminal, "Run a terminal for each failed step. The step is exported and executed as a container with its entire state and the current tty is attached directly into a /bin/ash shell within the failed step.")
}

func (s TerminalOptions) Build() Step {
	return &Terminal{opts: s}
}

type Terminal struct {
	opts TerminalOptions
}

func (s *Terminal) Run(rc *RunContext, next Next) error {
	err := next(rc)
	return s.walkError(rc, err)
}

func (s *Terminal) walkError(rc *RunContext, err error) error {
	unwrappedErr := err
	for unwrappedErr != nil {
		if uw, ok := unwrappedErr.(interface{ Unwrap() []error }); ok {
			for _, unwrappedErr := range uw.Unwrap() {

				fmt.Printf("\nUNWRAPPED %#v\n", unwrappedErr)

				return s.walkError(rc, unwrappedErr)
			}

			return err
		}

		unwrappedErr = errors.Unwrap(unwrappedErr)
	}

	if err := s.openTerminal(rc, err); err != nil {
		return err
	}

	return err
}

func (s *Terminal) openTerminal(rc *RunContext, err error) error {
	var innerStepErr processor.StepError
	if !AsInner(err, &innerStepErr) {
		return err
	}

	var exitCodeErr processor.ExitCode
	if !AsInner(err, &exitCodeErr) {
		return err
	}

	switch {
	case s.opts.Terminal == StepTerminalNever.String():
		return nil
	case s.opts.Terminal == StepTerminalIfFailed.String():

	case s.opts.Terminal == StepTerminalAsk.String():
		return nil
	case s.opts.Terminal == StepTerminalAskIfFailed.String():
		return nil
	}

	fmt.Printf("start terminal in %#v", innerStepErr)

	ctx := context.Background()

	stepCtx := innerStepErr.Context().DeepCopy()
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

	contextFS, err := fsutil.NewFS(".")
	if err != nil {
		return err
	}

	_, solveErr := rc.Buildkit.Client.Solve(ctx, def, client.SolveOpt{
		LocalMounts: map[string]fsutil.FS{
			"context": contextFS,
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
		envName := strings.ToUpper(strings.Replace(name, "-", "_", -1))
		stepCtx.EnvVars.Envs[fmt.Sprintf("SERVICE_%s", envName)] = service.ContainerIP
	}

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
