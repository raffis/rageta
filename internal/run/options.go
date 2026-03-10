package runner

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/go-logr/logr"
	"go.uber.org/zap"
	"golang.org/x/term"
)

// RunInput is the per-run input passed to Runner.Run (ctx, ref, entrypoint, inputs, root-level settings).
type RunInput struct {
	Ctx        context.Context
	Ref        string
	Entrypoint string
	Inputs     []string

	Logger    logr.Logger
	ZapConfig zap.Config
	Timeout   time.Duration
	DBPath    string

	Stdout io.Writer
	Stderr io.Writer
}

// BuilderStepOptions holds options for the builder step (tags, output, status, skip, tee, etc.).
type BuilderStepOptions struct {
	Tags               []string
	WithInternals      bool
	Expand             bool
	NoStatus           bool
	WaitUpdateInterval time.Duration
	LogDetached        bool
	SkipSteps          []string
	NoGC               bool
	SkipDone           bool
	Tee                bool
}

func IsTerm() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}
