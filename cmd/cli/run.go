package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/raffis/rageta/internal/dockersetup"
	"github.com/raffis/rageta/internal/kubesetup"
	"github.com/raffis/rageta/internal/ocisetup"
	"github.com/raffis/rageta/internal/otelsetup"
	"github.com/raffis/rageta/internal/runner"
	cruntime "github.com/raffis/rageta/internal/runtime"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"
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

type runFlags struct {
	output              string
	noGC                bool
	tee                 bool
	containerRuntime    string
	gracefulTermination time.Duration
	dockerQuiet         bool
	report              string
	reportOutput        string
	pull                string
	entrypoint          string
	contextDir          string
	inputs              []string
	envs                []string
	secretEnvs          []string
	tags                []string
	volumes             []string
	retry               uint64
	skipDone            bool
	skipSteps           []string
	logDetached         bool
	fork                bool
	maxConcurrent       int
	expand              bool
	noStatus            bool
	statusOutput        string
	waitUpdateInterval  time.Duration
	withInternals       bool
	user                string
	profile             string

	lifecycleOptions run.LifecycleOptions
	otelOptions   otelsetup.Options
	dockerOptions dockersetup.Options
	ociOptions    *ocisetup.Options
	kubeOptions   *kubesetup.Options
}

var runArgs = newRunFlags()

func newRunFlags() runFlags {
	return runFlags{
		waitUpdateInterval:  time.Second * 5,
		gracefulTermination: time.Second * 1,
		report:              "none",
		reportOutput:        os.Stdout.Name(),
		maxConcurrent:       runtime.NumCPU(),
		pull:                pullImageMissing.String(),
		containerRuntime:    electDefaultDriver().String(),
		output:              electDefaultOutput(),
		kubeOptions:         kubesetup.DefaultOptions(),
		ociOptions:          ocisetup.DefaultOptions(),
		profile:             electDefaultProfile().String(),
	}
}

var isTerm = term.IsTerminal(int(os.Stdout.Fd()))

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

func init() {
	_ = applyFlagProfile()
	runCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		return applyFlagProfile()
	}

	executionFlags := pflag.NewFlagSet("execution", pflag.ExitOnError)
	executionFlags.BoolVarP(&runArgs.tee, "tee", "", false, "Dump any internal redirected streams to stdout. Works similar as piping to tee on the console.")
	executionFlags.StringVarP(&runArgs.output, "output", "o", runArgs.output, "Output renderer. One of [prefix, ui, buffer[=gotpl], passthrough, discard]. The default `prefix` adds a colored task name prefix to the output lines while `ui` renders the tasks in a terminal ui. `passthrough` dumps all outputs directly without any modification.")
	executionFlags.BoolVarP(&runArgs.noGC, "no-gc", "", false, "Keep all containers and temporary files after execution.")
	executionFlags.BoolVarP(&runArgs.expand, "expand", "", false, "Expand steps from inherited pipelines and display them as separate entities.")
	executionFlags.IntVarP(&runArgs.maxConcurrent, "max-concurrent", "", runArgs.maxConcurrent, "Maximum number of concurrent steps. Affects concurrent and matrix steps.")
	executionFlags.BoolVarP(&runArgs.noStatus, "no-status", "", false, "Do not print task status messages")
	executionFlags.DurationVarP(&runArgs.waitUpdateInterval, "wait-update-interval", "", runArgs.waitUpdateInterval, "Print waiting for task status updates every n interval")
	executionFlags.BoolVarP(&runArgs.withInternals, "with-internals", "", false, "Expose internal steps")
	executionFlags.BoolVarP(&runArgs.skipDone, "skip-done", "", false, "Skip steps which have been successfully processed before. This is only useful in combination with a static context directory `--context-dir`.")
	executionFlags.StringSliceVarP(&runArgs.skipSteps, "skip-steps", "", nil, "Do not executed these steps")
	executionFlags.StringSliceVarP(&runArgs.tags, "tags", "", nil, "Add global custom tags to pipeline steps. Format is `key=value(:#color). Example: `--tags domain=example.com:#FF0000`")
	executionFlags.DurationVarP(&runArgs.gracefulTermination, "graceful-termination", "", runArgs.gracefulTermination, "Allow containers to exit gracefully.")
	executionFlags.StringVarP(&runArgs.containerRuntime, "container-runtime", "", runArgs.containerRuntime, "Container runtime. One of [docker].")
	executionFlags.StringVarP(&runArgs.report, "report", "r", runArgs.report, "Report summary of steps at the end of execution. One of [none, table, json, markdown].")
	executionFlags.StringVarP(&runArgs.statusOutput, "status-output", "", "", "Destination for the status output. By default this depends on the output (-o) set.")
	executionFlags.StringVarP(&runArgs.reportOutput, "report-output", "", runArgs.reportOutput, "Destination for the report output.")
	executionFlags.StringVarP(&runArgs.pull, "pull", "", runArgs.pull, "Pull image before running. one of [always, missing, never].")
	executionFlags.StringVarP(&runArgs.contextDir, "context-dir", "", "", "Use a static context directory. If any context is found it attempts to recover it.")
	executionFlags.BoolVarP(&runArgs.logDetached, "log-detached", "", false, "Detach logs.")
	executionFlags.Uint64VarP(&runArgs.retry, "retry", "", 0, "Retry pipeline if a failure occurred.")
	executionFlags.StringVarP(&runArgs.profile, "profile", "", runArgs.profile, "Use a predefined flag profile. One of [github]. Profiles can be overridden with explicit flags.")
	runCmd.Flags().AddFlagSet(executionFlags)

	pipelineFlags := pflag.NewFlagSet("pipeline", pflag.ExitOnError)
	pipelineFlags.StringVarP(&runArgs.entrypoint, "entrypoint", "t", "", "Entrypoint for the given pipeline. The pipelines default is used otherwise.")
	pipelineFlags.BoolVarP(&runArgs.fork, "fork", "", runArgs.fork, "Creates a controller container which handles this pipeline and exit.")
	pipelineFlags.StringSliceVarP(&runArgs.secretEnvs, "secret", "s", nil, "Pass secret envs to the pipeline. Secrets are handled as env variables but it is ensured they are masked in any sort of outputs.")
	pipelineFlags.StringSliceVarP(&runArgs.envs, "env", "e", nil, "Pass envs to the pipeline.")
	pipelineFlags.StringSliceVarP(&runArgs.volumes, "bind", "b", nil, "Bind directory as volume to the pipeline.")
	pipelineFlags.StringArrayVarP(&runArgs.inputs, "input", "i", nil, "Pass inputs to the pipeline.")
	pipelineFlags.StringVarP(&runArgs.user, "user", "u", "", "Username or UID (format: <name|uid>[:<group|gid>])")
	runCmd.Flags().AddFlagSet(pipelineFlags)

	dockerFlags := pflag.NewFlagSet("docker", pflag.ExitOnError)
	dockerFlags.BoolVarP(&runArgs.dockerQuiet, "docker-quiet", "q", false, "Suppress the docker pull output.")
	runArgs.dockerOptions.BindFlags(dockerFlags)
	runCmd.Flags().AddFlagSet(dockerFlags)

	otelFlags := pflag.NewFlagSet("otel", pflag.ExitOnError)
	runArgs.otelOptions.BindFlags(otelFlags)
	runCmd.Flags().AddFlagSet(otelFlags)

	ociFlags := pflag.NewFlagSet("oci", pflag.ExitOnError)
	runArgs.ociOptions.BindFlags(ociFlags)
	runCmd.Flags().AddFlagSet(ociFlags)

	kubeFlags := pflag.NewFlagSet("kube", pflag.ExitOnError)
	runArgs.kubeOptions.BindFlags(kubeFlags)
	runCmd.Flags().AddFlagSet(kubeFlags)

	sets := []runFlagGroup{
		{Set: executionFlags, DisplayName: "Execution"},
		{Set: pipelineFlags, DisplayName: "Pipeline"},
		{Set: dockerFlags, DisplayName: "Docker runtime"},
		{Set: kubeFlags, DisplayName: "Kubernetes runtime"},
		{Set: otelFlags, DisplayName: "Open Telemetry"},
		{Set: ociFlags, DisplayName: "OCI Registry"},
		{Set: rootCmd.Flags(), DisplayName: "Global"},
	}
	runFlagGroups = sets

	runCmd.SetUsageFunc(func(cmd *cobra.Command) error {
		printHelpCommand(cmd)
		return nil
	})

	runCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		printHelpCommand(cmd)
	})

	rootCmd.AddCommand(runCmd)

}

type pullImage string

var (
	pullImageAlways  pullImage = "always"
	pullImageNever   pullImage = "never"
	pullImageMissing pullImage = "missing"
)

func (d pullImage) String() string {
	return string(d)
}

type reportType string

var (
	reportTypeNone     reportType = "none"
	reportTypeTable    reportType = "table"
	reportTypeMarkdown reportType = "markdown"
)

func (d reportType) String() string {
	return string(d)
}

type renderOutput string

var (
	renderOutputPrefix                renderOutput = "prefix"
	renderOutputUI                    renderOutput = "ui"
	renderOutputPassthrough           renderOutput = "passthrough"
	renderOutputDiscard               renderOutput = "discard"
	renderOutputBuffer                renderOutput = "buffer"
	renderOutputBufferDefaultTemplate string       = "{{ .Buffer }}"
)

func (d renderOutput) String() string {
	return string(d)
}


func electDefaultOutput() string {
	if isTerm {
		return renderOutputUI.String()
	}

	return renderOutputPrefix.String()
}

func runRun(cmd *cobra.Command, args []string) error {
	runner := run.Build(
		run.WithLogging(zapConfig),
		run.WithLifecycle(runArgs.lifecycleOptions)
		run.WithOtel(runArgs.otelOptions, runArgs.output, zapConfig),
	)

	in := runInputFromArgs(cmd.Context(), args)

	r := runner.New().
		WithLogging(zapConfig).
		WithOtel(runArgs.otelOptions, runArgs.output, zapConfig).
		WithIO(runArgs.reportOutput, runArgs.envs, runArgs.secretEnvs).
		WithLifecycle().
		WithDriver(runArgs.containerRuntime, runArgs.dockerQuiet, &runArgs.dockerOptions, runArgs.kubeOptions).
		WithContextDir(runArgs.contextDir).
		WithTemplate(runArgs.volumes, runArgs.user).
		WithImagePolicy(runArgs.pull).
		WithFork(runArgs.fork, runArgs.noGC, runArgs.gracefulTermination).
		WithProvider(runArgs.ociOptions).
		WithCEL().
		WithOutput(runArgs.output).
		WithReport(runArgs.report).
		WithTeardown(runArgs.maxConcurrent, runArgs.statusOutput, runArgs.output).
		WithBuilder(runner.BuilderStepOptions{
			Tags:               runArgs.tags,
			WithInternals:      runArgs.withInternals,
			Expand:             runArgs.expand,
			NoStatus:           runArgs.noStatus,
			WaitUpdateInterval: runArgs.waitUpdateInterval,
			LogDetached:        runArgs.logDetached,
			SkipSteps:          runArgs.skipSteps,
			NoGC:               runArgs.noGC,
			SkipDone:           runArgs.skipDone,
			Tee:                runArgs.tee,
		}).
		WithInputs().
		WithExecute(runArgs.retry).
		WithCleanup(runArgs.contextDir, runArgs.noGC, runArgs.gracefulTermination)
	rc, err := r.Run(in)
	if err != nil {
		if rc != nil && rc.InputFlagSet != nil {
			helpAndExit(rc.InputFlagSet, err)
		}
		return err
	}
	logger.V(1).Info("pipeline completed", "result", rc.Result)
	runner.WriteErrorToStderr(rc)
	return rc.Result
}

func runInputFromArgs(ctx context.Context, args []string) runner.RunInput {
	ref := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "--") {
		ref = args[0]
	}
	return runner.RunInput{
		Ctx:        ctx,
		Ref:        ref,
		Entrypoint: runArgs.entrypoint,
		Inputs:     runArgs.inputs,
		Logger:     logger,
		ZapConfig:  zapConfig,
		Timeout:    rootArgs.timeout,
		DBPath:     rootArgs.dbPath,
	}
}

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
