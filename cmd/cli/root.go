package main

import (
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	version = "0.0.0-dev"
	commit  = "none"
	date    = "unknown"
)

type rootFlags struct {
	timeout time.Duration `env:"TIMEOUT"`
	log     struct {
		level    string `env:"LOG_LEVEL"`
		encoding string `env:"LOG_ENCODING"`
	}
}

var rootArgs rootFlags
var logger logr.Logger

var rootCmd = &cobra.Command{
	Use:               "rageta",
	Short:             "Run opinionated tasks everywhere anyhow",
	Long:              `Cloud native task engine`,
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
	rootCmd.PersistentFlags().StringVarP(&rootArgs.log.level, "log-level", "", "panic", "")
	rootCmd.PersistentFlags().StringVarP(&rootArgs.log.encoding, "log-encoding", "", "json", "")
}

func runRoot(cmd *cobra.Command, args []string) error {
	l, err := buildLogger(zap.NewDevelopmentConfig())
	if err != nil {
		return err
	}

	logger = l
	return nil
}

func buildLogger(logOpts zap.Config) (logr.Logger, error) {
	logOpts.Encoding = rootArgs.log.encoding
	err := logOpts.Level.UnmarshalText([]byte(rootArgs.log.level))
	if err != nil {
		return logr.Discard(), err
	}

	logOpts.DisableStacktrace = false

	zapLog, err := logOpts.Build()
	if err != nil {
		return logr.Discard(), err
	}

	return zapr.NewLogger(zapLog), nil
}
