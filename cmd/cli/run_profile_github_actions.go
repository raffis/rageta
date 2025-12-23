package main

import (
	"fmt"
	"os"
)

func (f *runFlags) githubActionsProfile() error {
	if !runCmd.Flags().Changed("report") {
		f.report = reportTypeMarkdown.String()
	}

	if !runCmd.Flags().Changed("output") {
		renderOutputBufferDefaultTemplate = `
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
		runArgs.output = fmt.Sprintf("%s=%s", renderOutputBuffer.String(), renderOutputBufferDefaultTemplate)
	}

	if !runCmd.Flags().Changed("report-output") && os.Getenv("GITHUB_STEP_SUMMARY") != "" {
		runArgs.reportOutput = os.Getenv("GITHUB_STEP_SUMMARY")
	}
	
  if !runCmd.Flags().Changed("no-gc") {
		runArgs.noGC = true
	}

	return nil
}
