package run

import (
	"fmt"
	"os"
	"path"
	"time"

	"github.com/spf13/pflag"
)

type ContextDirOptions struct {
	ContextDir    string
	SkipContextGC bool
}

func (s ContextDirOptions) Build() Step {
	return &ContextDir{
		opts: s,
	}
}

func (s *ContextDirOptions) BindFlags(flags *pflag.FlagSet) {
	flags.StringVarP(&s.ContextDir, "context-dir", "", s.ContextDir, "Use a static context directory. If any context is found it attempts to recover it.")
	flags.BoolVarP(&s.SkipContextGC, "skip-context-gc", "", s.SkipContextGC, "Do not keep the context directory after the pipeline execution.")
}

type ContextDir struct {
	opts ContextDirOptions
}

type ContextDirContext struct {
	Path string
}

func (s *ContextDir) Run(rc *RunContext, next Next) error {
	contextDir := s.opts.ContextDir

	if contextDir == "" {
		tmpDir, err := os.MkdirTemp(os.TempDir(), "rageta")
		if err != nil {
			return fmt.Errorf("failed to create temp context run directory: %w", err)
		}

		contextDir = tmpDir

		if s.opts.SkipContextGC {
			defer func() {
				_ = os.RemoveAll(contextDir)
			}()
		}
	} else {
		contextDir = path.Join(contextDir, time.Now().Format(time.RFC3339))

		if err := os.MkdirAll(contextDir, 0700); err != nil {
			return fmt.Errorf("failed to create context run directory: %w", err)
		}
	}

	rc.ContextDir.Path = contextDir
	rc.Logging.Logger.V(1).Info("use context directory", "path", contextDir)
	return next(rc)
}
