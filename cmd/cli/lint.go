package main

import (
	"context"
	"fmt"

	"github.com/raffis/rageta/internal/run"
	"github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/internal/setup/ocisetup"
	"github.com/spf13/cobra"
)

var lintCmd = &cobra.Command{
	Use:   "lint <ref>",
	Short: "Validate a pipeline against its CRD schema.",
	Args:  cobra.ExactArgs(1),
	RunE:  runLint,
}

type lintFlags struct {
	ociOptions *ocisetup.Options
}

var lintArgs = newLintFlags()

func newLintFlags() lintFlags {
	return lintFlags{
		ociOptions: ocisetup.DefaultOptions(),
	}
}

func init() {
	lintArgs.ociOptions.BindFlags(lintCmd.Flags())
	rootCmd.AddCommand(lintCmd)
}

func runLint(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	if rootArgs.timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, rootArgs.timeout)
		defer cancel()
	}

	store, persistDB := run.CreateProvider(runtime.PullImagePolicyAlways, rootArgs.dbPath, lintArgs.ociOptions, false)
	_, err := store.Resolve(ctx, args[0])
	if err != nil {
		return err
	}

	if err := persistDB(); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "pipeline %q is valid\n", args[0])
	return nil
}
