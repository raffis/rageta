package main

import (
	"context"
	"fmt"
	"os"

	"github.com/raffis/rageta/internal/ocisetup"
	"github.com/raffis/rageta/internal/pipeline"
	"github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/internal/styles"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var helpCmd = &cobra.Command{
	Use:  "help",
	RunE: runHelp,
}

type helpFlags struct {
	full       bool
	ociOptions *ocisetup.Options
}

var helpArgs = newHelpFlags()

func newHelpFlags() helpFlags {
	return helpFlags{
		ociOptions: ocisetup.DefaultOptions(),
	}
}

func init() {
	helpArgs.ociOptions.BindFlags(helpCmd.Flags())
	helpCmd.Flags().BoolVarP(&helpArgs.full, "full", "f", false, "Show full help page including all flags from rageta")

	rootCmd.AddCommand(helpCmd)
}

func runHelp(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	if rootArgs.timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, rootArgs.timeout)
		defer cancel()
	}

	var ref string
	if len(args) > 0 {
		ref = args[0]
	}

	store, persistDB := createProvider(runtime.PullImagePolicyAlways, rootArgs.dbPath, helpArgs.ociOptions)
	command, err := store.Resolve(ctx, ref)
	if err != nil {
		return err
	}

	shortDescription := command.ShortDescription
	if shortDescription != "" {
		shortDescription += "\n"
	}

	longDescription := command.LongDescription
	if longDescription == "" {
		longDescription = "n/a"
	}
	longDescription += "\n"

	fmt.Fprintf(os.Stderr, "%s\n%s\n%s\n%s",
		styles.Bold.Render(command.Name),
		shortDescription,
		styles.Bold.Render("Description:"),
		longDescription,
	)

	fmt.Fprintf(os.Stderr, "\n%s\n", styles.Bold.Render("Targets:"))
	for _, step := range command.Steps {
		if !step.Expose {
			continue
		}

		fmt.Fprintf(os.Stderr, "%s: %s\n%s\n\n", styles.Bold.Render(step.Name), step.Short, step.Long)
	}

	flagSet := pflag.NewFlagSet("inputs", pflag.ContinueOnError)
	pipeline.Flags(flagSet, command.Inputs)

	fmt.Fprintf(os.Stderr, "\n%s\n", styles.Bold.Render("Inputs:"))
	flagSet.PrintDefaults()

	if helpArgs.full {
		fmt.Fprintf(os.Stderr, "\n%s\n", styles.Bold.Render("Rageta flags:"))
		if err := runCmd.Usage(); err != nil {
			return err
		}
	}

	if err := persistDB(); err != nil {
		logger.V(1).Error(err, "failed to persist database")
	}

	return nil
}
