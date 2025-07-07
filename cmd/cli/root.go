package main

import (
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/go-logr/logr"
	"github.com/muesli/termenv"
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
	noColor    bool          `env:"NO_COLOR"`
	dbPath     string        `env:"DB_PATH"`
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
	dbPath := "/rageta.db"
	home, err := os.UserHomeDir()
	if err == nil {
		dbPath = filepath.Join(home, "rageta.db")
	}

	rootCmd.PersistentFlags().StringVarP(&rootArgs.dbPath, "db-path", "", dbPath, "Path to the local rageta pipeline store.")
	rootCmd.PersistentFlags().DurationVarP(&rootArgs.timeout, "timeout", "", 0, "")
	rootCmd.PersistentFlags().BoolVarP(&rootArgs.noColor, "no-color", "", false, "Disable all color output to the terminal.")
	rootArgs.logOptions.BindFlags(rootCmd.PersistentFlags())
}

func runRoot(cmd *cobra.Command, args []string) error {
	var err error
	logger, zapConfig, err = rootArgs.logOptions.Build()
	if err != nil {
		return err
	}

	if rootArgs.noColor {
		lipgloss.SetColorProfile(termenv.Ascii)
	}

	return err
}
