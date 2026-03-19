package main

import (
	"os"

	"github.com/raffis/rageta/internal/run"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Execute a pipeline.",
	RunE:  runRun,
}

// runFlagGroup is used by help -f to print rageta flags in the same style as pipeline inputs.
type runFlagGroup struct {
	Set         *pflag.FlagSet
	DisplayName string
}

var runFlagGroups []runFlagGroup

/*
	func applyFlagProfile() error {
		switch runArgs.profile {
		case flagProfileGithubActions.String():
			return runArgs.githubActionsProfile()
		case flagProfileDebug.String():
			return runArgs.debugProfile()
		case flagProfileDefault.String():
			return nil
		default:
			return fmt.Errorf("invalid flag profile given: %s", runArgs.profile)
		}
	}
*/
type flagProfile string

var (
	flagProfileGithubActions flagProfile = "github-actions"
	flagProfileDebug         flagProfile = "debug"
	flagProfileDefault       flagProfile = "default"
)

func (d flagProfile) String() string {
	return string(d)
}

func electDefaultProfile() flagProfile {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		return flagProfileGithubActions
	}

	return flagProfileDefault
}

var runOpts run.Options

func init() {
	runOpts = run.DefaultOptions()
	runOpts.BindFlags(runCmd.Flags())
	runCmd.SetUsageFunc(func(cmd *cobra.Command) error {
		return nil
	})

	rootCmd.AddCommand(runCmd)

}

func runRun(cmd *cobra.Command, args []string) error {
	runOpts.LoggingOptions.ZapConfig = zapConfig
	_, err := runOpts.Build().
		Run(cmd.Context(), args, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.OutOrStderr())

	return err
}
