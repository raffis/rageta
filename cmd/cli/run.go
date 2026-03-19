package main

import (
	"fmt"
	"os"

	"charm.land/lipgloss/v2"
	"github.com/raffis/rageta/internal/run"
	cruntime "github.com/raffis/rageta/internal/runtime"
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
	//runArgs.loggingOptions.ZapConfig = zapConfig

	runOpts.LoggingOptions.ZapConfig = zapConfig
	_, err := runOpts.Build().
		Run(cmd.Context(), args, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.OutOrStderr())

	return err
}

/*func runInputFromArgs(ctx context.Context, args []string) run.RunInput {
	ref := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "--") {
		ref = args[0]
	}
	return run.RunInput{
		Ctx:        ctx,
		Ref:        ref,
		Entrypoint: runArgs.entrypoint,
		Inputs:     runArgs.inputs,
		Logger:     logger,
		ZapConfig:  zapConfig,
		Timeout:    rootArgs.timeout,
		DBPath:     rootArgs.dbPath,
	}
}*/

func helpAndExit(flagSet *pflag.FlagSet, err error) {
	style := lipgloss.NewStyle().Bold(true)
	fmt.Fprintf(os.Stderr, "\n%s\n", style.Render("Error:"))
	fmt.Fprintln(os.Stderr, err.Error())

	fmt.Fprintf(os.Stderr, "\n%s\n", style.Render("Inputs:"))
	flagSet.PrintDefaults()

	if res, ok := err.(*cruntime.Result); ok {
		os.Exit(res.ExitCode)
	}

	os.Exit(1)
}
