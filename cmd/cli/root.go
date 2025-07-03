package main

import (
	"time"

	"github.com/go-logr/logr"
	"github.com/raffis/rageta/internal/logsetup"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	version = "0.0.0-dev"
	commit  = "none"
	date    = "unknown"
)

type rootFlags struct {
	timeout    time.Duration `env:"TIMEOUT"`
	logOptions logsetup.Options
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
	rootArgs.logOptions.BindFlags(rootCmd.PersistentFlags())
}

func runRoot(cmd *cobra.Command, args []string) error {
	var err error
	logger, zapConfig, err = rootArgs.logOptions.Build()
	return err
}
