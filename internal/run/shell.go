package run

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/moby/buildkit/client"
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/setup/flagset"
)

type ShellOptions struct {
	Shell string
}

func (s *ShellOptions) BindFlags(flags flagset.Interface) {
	flags.StringVarP(&s.Shell, "shell", "", s.Shell, ".")
}

func (s ShellOptions) Build() Step {
	return &Shell{opts: s}
}

type Shell struct {
	opts ShellOptions
}

func (s *Shell) Run(rc *RunContext, next Next) error {
	err := next(rc)

	var innerStepErr processor.StepError
	if !AsInner(err, &innerStepErr) {
		return err
	}

	fmt.Printf("IUNNER STEP %#v\n", innerStepErr.Context().UniqueName())
	ctx := context.Background()

	def, marshalErr := innerStepErr.Context().Build.State.Marshal(ctx)
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

	fmt.Fprintf(os.Stderr, "launching shell in failed step state: %s\n", imageName)

	shell := s.opts.Shell
	if shell == "" {
		shell = "/bin/sh"
	}

	runCmd := exec.CommandContext(ctx, "docker", "run", "--rm", "-it", imageName, shell)
	runCmd.Stdin = os.Stdin
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr

	if runErr := runCmd.Run(); runErr != nil {
		return fmt.Errorf("docker run: %w", runErr)
	}

	return err
}
