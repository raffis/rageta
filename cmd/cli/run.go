package main

import (
	"fmt"
	"os"

	"github.com/raffis/rageta/internal/run"
	"github.com/raffis/rageta/internal/setup/flagset"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Execute a pipeline.",
	RunE:  runRun,
}

func applyFlagProfile(opts *run.Options) error {
	switch runFlagProfile {
	case flagProfileGithubActions.String():
		return githubActionsProfile(opts)
	case flagProfileDebug.String():
		return debugProfile(opts)
	case flagProfileDefault.String():
		return nil
	default:
		return fmt.Errorf("invalid flag profile given: %s", runFlagProfile)
	}
}

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
var runFlagProfile = electDefaultProfile().String()
var runFlags *flagset.Wrapper

func init() {
	runOpts = run.DefaultOptions()

	_ = applyFlagProfile(&runOpts)
	runCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		return applyFlagProfile(&runOpts)
	}

	runCmd.Flags().StringVarP(&runFlagProfile, "profile", "p", runFlagProfile, "Flag profile")
	runFlags = flagset.NewWrapper(runCmd.Flags())
	runOpts.BindFlags(runFlags)
	rootCmd.AddCommand(runCmd)
}

func runRun(cmd *cobra.Command, args []string) error {
	runOpts.LoggingOptions.ZapConfig = zapConfig
	runOpts.ProviderOptions.DBPath = rootArgs.dbPath
	runOpts.LifecycleOptions.Timeout = rootArgs.timeout
	_, err := runOpts.Build().
		Run(cmd.Context(), args, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.OutOrStderr())

	return err
}

func debugProfile(opts *run.Options) error {
	if !runCmd.Flags().Changed("report") {
		opts.ReportOptions.ReportType = run.ReportTypeTable.String()
	}

	if !runCmd.Flags().Changed("expand") {
		opts.OutputOptions.Expand = true
	}

	if !runCmd.Flags().Changed("pull") {
		opts.ImagePolicyOptions.Policy = run.PullImageAlways.String()
	}

	if !runCmd.Flags().Changed("skip-done") {
		opts.PipelineOptions.SkipDone = false
	}

	if !runCmd.Flags().Changed("no-gc") {
		opts.TeardownOptions.Disabled = true
	}

	if !runCmd.Root().PersistentFlags().Changed("verbose") {
		rootArgs.logOptions.Verbose = 10
		var err error
		logger, zapConfig, err = rootArgs.logOptions.Build()
		if err != nil {
			return err
		}
	}

	return nil
}

func githubActionsProfile(opts *run.Options) error {
	if !runCmd.Flags().Changed("report") {
		opts.ReportOptions.ReportType = run.ReportTypeMarkdown.String()
	}

	if !runCmd.Flags().Changed("output") {
		renderOutputBufferDefaultTemplate := `
	{{- $tags := "" }}
	{{- range $tag := .Tags}}
		{{- if eq $tags "" }}
			{{- $tags = printf "%s=%s" $tag.Key $tag.Value }}
		{{- else }}
			{{- $tags = printf "%s %s=%s" $tags $tag.Key $tag.Value }}
		{{- end }}
	{{- end }}

	{{- $stepName := .StepName }}
	{{- if $tags }}
		{{- $stepName = printf "%s[%s]" .StepName $tags }}
	{{- end }}

	{{- if and .Error .Skipped }}
		{{- printf "⚠️ %s\n%s\n" $stepName .Buffer }}
	{{- else if .Error }}
		{{- printf "⛔ %s\n%s\n" $stepName .Buffer }}
	{{- else }}
		{{- printf "::group::✅ %s\n%s\n::endgroup::\n" $stepName .Buffer }}
	{{- end }}`
		opts.OutputOptions.Output = fmt.Sprintf("%s=%s", run.RenderOutputBuffer.String(), renderOutputBufferDefaultTemplate)
	}

	if !runCmd.Flags().Changed("report-output") && os.Getenv("GITHUB_STEP_SUMMARY") != "" {
		opts.ReportOptions.ReportOutput = os.Getenv("GITHUB_STEP_SUMMARY")
	}

	if !runCmd.Flags().Changed("no-gc") {
		opts.TeardownOptions.Disabled = true
	}

	return nil
}
