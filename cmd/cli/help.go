package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/raffis/rageta/internal/ocisetup"
	"github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/internal/styles"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
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

func printHelpPipeline(cmd *cobra.Command, ref string, full bool) error {
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	if rootArgs.timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, rootArgs.timeout)
		defer cancel()
	}

	store, persistDB := createProvider(runtime.PullImagePolicyAlways, rootArgs.dbPath, helpArgs.ociOptions)
	command, err := store.Resolve(ctx, ref)
	if err != nil {
		return err
	}

	sections := formatPipelineHelpSections(command, full)
	fmt.Fprintln(cmd.ErrOrStderr(), strings.Join(sections, ""))

	if err := persistDB(); err != nil {
		logger.V(1).Error(err, "failed to persist database")
	}

	return nil
}

func formatFlagSetStyle(set *pflag.FlagSet) []string {
	var flagBlocks []string
	set.VisitAll(func(f *pflag.Flag) {
		name := "--" + f.Name
		if f.Shorthand != "" {
			name = "-" + string(f.Shorthand) + ", " + name
		}
		line := styles.HelpInputFlag.Render(name)
		if f.DefValue != "" && f.DefValue != "[]" && f.DefValue != "map[]" {
			line += styles.HelpInputType.Render("  [default: " + f.DefValue + "]")
		}
		if f.Usage != "" {
			line += "\n  " + styles.HelpMuted.Render(f.Usage)
		}
		flagBlocks = append(flagBlocks, line)
	})
	return flagBlocks
}

func printHelpCommand(cmd *cobra.Command) {
	var sections []string
	name := cmd.Name()
	if name == "" {
		name = cmd.Use
	}
	title := styles.HelpTitle.Render(name)
	sections = append(sections, title, "\n")

	if cmd.Short != "" {
		short := styles.HelpShort.Render(cmd.Short)
		sections = append(sections, short)
	}

	if cmd.Long != "" {
		descHeader := styles.HelpSection.Render("\n\nDescription:")
		descBody := styles.HelpBody.Render(cmd.Long)
		sections = append(sections, descHeader, "\n", descBody)
	}

	if cmd == runCmd {
		// Run has grouped flag sets (Execution, Pipeline, etc.)
		for _, group := range runFlagGroups {
			flagBlocks := formatFlagSetStyle(group.Set)
			if len(flagBlocks) > 0 {
				sections = append(sections, styles.HelpSection.Render("\n\n"+group.DisplayName), styles.HelpBody.Render("\n\n"), strings.Join(flagBlocks, "\n\n"))
			}
		}
	} else {
		// Generic: show this command's flags, then global (inherited) flags
		localBlocks := formatFlagSetStyle(cmd.NonInheritedFlags())
		if len(localBlocks) > 0 {
			sections = append(sections, "\n\n", styles.HelpSection.Render("Flags:"), "\n\n", strings.Join(localBlocks, "\n\n"))
		}
		inheritedBlocks := formatFlagSetStyle(cmd.InheritedFlags())
		if len(inheritedBlocks) > 0 {
			sections = append(sections, "\n\n", styles.HelpSection.Render("Global Flags:"), "\n\n", strings.Join(inheritedBlocks, "\n\n"))
		}
	}

	fmt.Fprintln(cmd.ErrOrStderr(), strings.Join(sections, ""))
}

func formatPipelineHelpSections(command v1beta1.Pipeline, full bool) []string {
	var sections []string

	title := styles.HelpTitle.Render(fmt.Sprintf("● %s ●\n", command.Name))
	sections = append(sections, title)

	if command.ShortDescription != "" {
		short := styles.HelpShort.Render(command.ShortDescription)
		sections = append(sections, short)
	}

	if command.LongDescription != "" {
		descHeader := styles.HelpSection.Render("\n\nDescription:\n\n")
		descBody := styles.HelpBody.Render(command.LongDescription + "\n")
		sections = append(sections, descHeader, descBody)
	}

	hasTargets := false
	for _, step := range command.Steps {
		if step.Expose {
			hasTargets = true
			break
		}
	}

	if hasTargets {
		var targetBlocks []string
		for _, step := range command.Steps {
			if !step.Expose {
				continue
			}
			block := styles.HelpTargetName.Render(step.Name) + " " + styles.HelpTargetShort.Render(step.Short)
			if step.Long != "" {
				block += "\n" + styles.HelpTargetLong.Render(step.Long)
			}
			targetBlocks = append(targetBlocks, block)
		}
		sections = append(sections, styles.HelpSection.Render("\n\nTargets:"), styles.HelpBody.Render("\n\n"), strings.Join(targetBlocks, "\n\n"))
	}

	if len(command.Inputs) > 0 {
		var inputBlocks []string
		for _, input := range command.Inputs {
			input.SetDefaults()
			flagPart := styles.HelpInputFlag.Render("--" + input.Name)
			line := flagPart + styles.HelpInputType.Render(" "+string(input.Type))
			if input.Default != nil {
				line += styles.HelpInputType.Render(fmt.Sprintf("  [default: %s]", formatParamDefault(input.Default)))
			}
			if input.Description != "" {
				line += "\n  " + styles.HelpMuted.Render(input.Description)
			}
			inputBlocks = append(inputBlocks, line)
		}
		sections = append(sections, styles.HelpSection.Render("\n\nInputs:"), styles.HelpBody.Render("\n\n"), strings.Join(inputBlocks, "\n\n"))
	}

	if full {
		for _, group := range runFlagGroups {
			var flagBlocks []string
			group.Set.VisitAll(func(f *pflag.Flag) {
				name := "--" + f.Name
				if f.Shorthand != "" {
					name = "-" + string(f.Shorthand) + ", " + name
				}
				line := styles.HelpInputFlag.Render(name)
				if f.DefValue != "" && f.DefValue != "[]" && f.DefValue != "map[]" {
					line += styles.HelpInputType.Render("  [default: " + f.DefValue + "]")
				}
				if f.Usage != "" {
					line += "\n  " + styles.HelpMuted.Render(f.Usage)
				}

				flagBlocks = append(flagBlocks, line)
			})

			if len(flagBlocks) > 0 {
				sections = append(sections, styles.HelpSection.Render("\n\n"+group.DisplayName), styles.HelpBody.Render("\n\n"), strings.Join(flagBlocks, "\n\n"))
			}
		}
	}

	return sections
}

func runHelp(cmd *cobra.Command, args []string) error {
	var ref string
	if len(args) > 0 {
		ref = args[0]
	}

	return printHelpPipeline(cmd, ref, helpArgs.full)
}

func formatParamDefault(p *v1beta1.ParamValue) string {
	if p == nil {
		return ""
	}
	switch p.Type {
	case v1beta1.ParamTypeString:
		return p.StringVal
	case v1beta1.ParamTypeArray:
		return strings.Join(p.ArrayVal, ", ")
	case v1beta1.ParamTypeObject:
		var pairs []string
		for k, v := range p.ObjectVal {
			pairs = append(pairs, k+"="+v)
		}
		return strings.Join(pairs, ", ")
	default:
		return p.StringVal
	}
}
