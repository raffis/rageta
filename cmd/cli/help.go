package main

import (
	"context"
	"fmt"
	"os"
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

	if helpArgs.full {
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

	fmt.Fprintln(os.Stderr, strings.Join(sections, ""))

	if err := persistDB(); err != nil {
		logger.V(1).Error(err, "failed to persist database")
	}

	return nil
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
