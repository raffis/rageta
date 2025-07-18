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

	store := createProvider(runtime.PullImagePolicyAlways, rootArgs.dbPath, helpArgs.ociOptions)
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

	flagSet := pflag.NewFlagSet("inputs", pflag.ContinueOnError)
	pipeline.Flags(flagSet, command.Inputs)

	fmt.Fprintf(os.Stderr, "\n%s\n", styles.Bold.Render("Inputs:"))
	flagSet.PrintDefaults()

	fmt.Fprintf(os.Stderr, "\n%s\n", styles.Bold.Render("Rageta flags:"))
	if err := runCmd.Usage(); err != nil {
		return err
	}

	return nil
}
