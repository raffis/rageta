package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

type handlerFlags struct {
	stdinPath  string   `env:"STDIN_PATH"`
	stdoutPath []string `env:"STDOUT_PATH"`
	stderrPath []string `env:"STDERR_PATH"`
}

var handlerArgs = handlerFlags{}

var rootCmd = &cobra.Command{
	Use:  "handler",
	RunE: runHandler,
}

func main() {
	err := rootCmd.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "handler error: %v\n", err)
		os.Exit(130)
	}
}

func init() {
	rootCmd.Flags().StringVarP(&handlerArgs.stdinPath, "stdin", "", "", "Path to redirect stdin from (only one allowed)")
	rootCmd.Flags().StringSliceVarP(&handlerArgs.stdoutPath, "stdout", "", []string{}, "Paths to redirect stdout to (multiple allowed)")
	rootCmd.Flags().StringSliceVarP(&handlerArgs.stderrPath, "stderr", "", []string{}, "Paths to redirect stderr to (multiple allowed)")
}

func runHandler(cmd *cobra.Command, args []string) error {
	execCmd := exec.CommandContext(context.Background(), args[0], args[1:]...)

	if handlerArgs.stdinPath != "" {
		stdinFile, err := os.Open(handlerArgs.stdinPath)
		if err != nil {
			return fmt.Errorf("failed to open stdin file %s: %w", handlerArgs.stdinPath, err)
		}
		defer stdinFile.Close()
		execCmd.Stdin = stdinFile
	} else {
		execCmd.Stdin = os.Stdin
	}

	if len(handlerArgs.stdoutPath) > 0 {
		writers := []io.Writer{os.Stdout}

		for _, path := range handlerArgs.stdoutPath {
			dir := filepath.Dir(path)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory for stdout file %s: %w", path, err)
			}

			file, err := os.Create(path)
			if err != nil {
				return fmt.Errorf("failed to create stdout file %s: %w", path, err)
			}

			defer file.Close()
			writers = append(writers, file)
		}

		execCmd.Stdout = io.MultiWriter(writers...)
	} else {
		execCmd.Stdout = os.Stdout
	}

	if len(handlerArgs.stderrPath) > 0 {
		writers := []io.Writer{os.Stderr}

		for _, path := range handlerArgs.stderrPath {
			dir := filepath.Dir(path)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory for stderr file %s: %w", path, err)
			}

			file, err := os.Create(path)
			if err != nil {
				return fmt.Errorf("failed to create stderr file %s: %w", path, err)
			}

			defer file.Close()
			writers = append(writers, file)
		}

		execCmd.Stderr = io.MultiWriter(writers...)
	} else {
		execCmd.Stderr = os.Stderr
	}

	if err := execCmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}

		return fmt.Errorf("failed to execute command: %w", err)
	}

	return nil
}
