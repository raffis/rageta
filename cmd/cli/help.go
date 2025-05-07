package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/raffis/rageta/internal/ocisetup"
	"github.com/raffis/rageta/internal/pipeline"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	"github.com/raffis/rageta/internal/storage"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
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

	scheme := kruntime.NewScheme()
	v1beta1.AddToScheme(scheme)
	factory := serializer.NewCodecFactory(scheme)
	decoder := factory.UniversalDeserializer()

	var ref string
	if len(args) > 0 {
		ref = args[0]
	}

	store := storage.New(
		decoder,
		storage.WithFile(),
		storage.WithRagetafile(),
		func(ctx context.Context, ref string) (io.Reader, error) {
			runArgs.ociOptions.URL = ref
			ociClient, err := runArgs.ociOptions.Build(ctx)
			if err != nil {
				return nil, err
			}

			return storage.WithOCI(ociClient)(ctx, ref)
		},
	)

	command, err := store.Lookup(ctx, ref)
	if err != nil {
		return err
	}

	style := lipgloss.NewStyle().Bold(true)
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
		style.Render(command.ObjectMeta.Name),
		shortDescription,
		style.Render("Description:"),
		longDescription,
	)

	flagSet := pflag.NewFlagSet("inputs", pflag.ContinueOnError)
	pipeline.Flags(flagSet, command.Inputs)

	fmt.Fprintf(os.Stderr, "\n%s\n", style.Render("Inputs:"))
	flagSet.PrintDefaults()

	return nil
}
