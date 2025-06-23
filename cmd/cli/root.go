package main

import (
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	version = "0.0.0-dev"
	commit  = "none"
	date    = "unknown"
)

type rootFlags struct {
	timeout time.Duration `env:"TIMEOUT"`
	verbose int8          `env:"VERBOSE"`
	log     struct {
		encoding string `env:"LOG_ENCODING"`
	}
}

var logLevels = map[int8]zapcore.Level{
	0: zapcore.FatalLevel,
	1: zapcore.PanicLevel,
	2: zapcore.ErrorLevel,
	3: zapcore.WarnLevel,
	4: zapcore.InfoLevel,
	5: zapcore.DebugLevel,
}

var rootArgs rootFlags
var logger logr.Logger
var zapConfig zap.Config

var rootCmd = &cobra.Command{
	Use:               "rageta",
	Short:             "Cloud native pipeline engine",
	PersistentPreRunE: runRoot,
}

func main() {
	err := rootCmd.Execute()
	if err != nil {
		panic(err)
	}
}

func init() {
	rootCmd.PersistentFlags().DurationVarP(&rootArgs.timeout, "timeout", "", 0, "")
	rootCmd.PersistentFlags().Int8VarP(&rootArgs.verbose, "verbose", "v", 0, "")
	rootCmd.PersistentFlags().StringVarP(&rootArgs.log.encoding, "log-encoding", "", "json", "")
}

func runRoot(cmd *cobra.Command, args []string) error {
	zapConfig = zap.NewDevelopmentConfig()
	zapConfig.Encoding = rootArgs.log.encoding

	if level, ok := logLevels[rootArgs.verbose]; ok {
		zapConfig.Level = zap.NewAtomicLevelAt(level)
	} else {
		zapConfig.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	}

	zapConfig.DisableStacktrace = false
	l, err := buildLogger(zapConfig)
	if err != nil {
		return err
	}

	logger = l
	return nil
}

func buildLogger(logOpts zap.Config) (logr.Logger, error) {
	zapLog, err := logOpts.Build()
	if err != nil {
		return logr.Discard(), err
	}

	return zapr.NewLogger(zapLog), nil
}
