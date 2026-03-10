package run

import (
	"fmt"
	"os"

	"github.com/spf13/pflag"
)

type ContextDirOptions struct {
	ContextDir string
}

func (s ContextDirOptions) Build() Step {
	return &ContextDir{
		opts: s,
	}
}

func (s ContextDirOptions) BindFlags(flags *pflag.FlagSet) {
	flags.StringVarP(&s.ContextDir, "context-dir", "", "", "Use a static context directory. If any context is found it attempts to recover it.")
}

type ContextDir struct {
	opts ContextDirOptions
}

func (s *ContextDir) Run(rc *RunContext, next Next) error {
	contextDir := s.opts.ContextDir
	if contextDir == "" {
		tmpDir, err := os.MkdirTemp(os.TempDir(), "rageta")
		if err != nil {
			return fmt.Errorf("failed to create tmp dir: %w", err)
		}
		contextDir = tmpDir
	}
	rc.ContextDir = contextDir
	rc.Logger.V(1).Info("use context directory", "path", contextDir)
	return next(rc)
}
