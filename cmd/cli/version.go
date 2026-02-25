package main

import (
	"fmt"
	"os"

	"charm.land/lipgloss/v2"
	"github.com/raffis/rageta/internal/tui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var versionCmd = &cobra.Command{
	Use:  "version",
	RunE: runVersion,
}

type versionFlags struct {
	json bool `env:"JSON"`
}

var versionArgs = versionFlags{}

func init() {
	versionCmd.Flags().BoolVarP(&versionArgs.json, "json", "", !term.IsTerminal(int(os.Stdout.Fd())), "")
	rootCmd.AddCommand(versionCmd)
}

func runVersion(cmd *cobra.Command, args []string) error {
	if versionArgs.json {
		fmt.Printf(`{"version":"%s","sha":"%s","date":"%s"}`+"\n", version, commit, date)
		return nil
	}

	fmt.Printf("%s\n%s\n\n%s\t%s\n%s\t%s\n%s\t%s\n",
		lipgloss.NewStyle().Bold(true).Render("RAGETA"),
		"Build cloud native style cli style pipelines",
		lipgloss.NewStyle().Bold(true).Render("Version:"),
		version,
		lipgloss.NewStyle().Bold(true).Render("Commit SHA:"),
		commit,
		lipgloss.NewStyle().Bold(true).Render("Build date:"),
		date,
	)

	fmt.Println(tui.Logo)

	return nil
}
