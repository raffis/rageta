package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"os/user"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	"github.com/gofrs/flock"
	"github.com/raffis/rageta/internal/dockersetup"
	"github.com/raffis/rageta/internal/kubesetup"
	"github.com/raffis/rageta/internal/logbridge"
	"github.com/raffis/rageta/internal/mask"
	"github.com/raffis/rageta/internal/ocisetup"
	"github.com/raffis/rageta/internal/otelsetup"
	"github.com/raffis/rageta/internal/output"
	"github.com/raffis/rageta/internal/pipeline"
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/provider"
	"github.com/raffis/rageta/internal/report"
	cruntime "github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/internal/styles"
	"github.com/raffis/rageta/internal/tui"
	"github.com/raffis/rageta/internal/utils"
	"github.com/raffis/rageta/internal/xio"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"github.com/sethvargo/go-retry"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/alitto/pond/v2"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/term"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
)

var runCmd = &cobra.Command{
	Use:  "run",
	RunE: runRun,
}

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
	output              string        `env:"OUTPUT"`
	noGC                bool          `env:"NO_GC"`
	tee                 bool          `env:"TEE"`
	containerRuntime    string        `env:"CONTAINER_RUNTIME"`
	gracefulTermination time.Duration `env:"GRACEFUL_TERMINATION"`
	dockerQuiet         bool          `env:"DOCKER_QUIT"`
	report              string        `env:"REPORT"`
	reportOutput        string        `env:"REPORT_OUTPUT"`
	pull                string        `env:"PULL"`
	entrypoint          string        `env:"ENTRYPOINT"`
	contextDir          string        `env:"CONTEXT_DIR"`
	inputs              []string      `env:"INPUTS"`
	envs                []string      `env:"ENVS"`
	secretEnvs          []string      `env:"SECRET_ENVS"`
	tags                []string      `env:"TAGS"`
	volumes             []string      `env:"VOLUMES"`
	retry               uint64        `env:"RETRY"`
	skipDone            bool          `env:"SKIP_DONE"`
	skipSteps           []string      `env:"SKIP_STEPS"`
	logDetached         bool          `env:"LOG_DETACHED"`
	fork                bool          `env:"FORK"`
	maxConcurrent       int           `env:"MAX_CONCURRENT"`
	expand              bool          `env:"EXPAND"`
	noStatus            bool          `env:"NO_STATUS"`
	profile             string        `env:"PROFILE"`

	statusOutput       string        `env:"STATUS_OUTPUT"`
	waitUpdateInterval time.Duration `env:"WAIT_UPDATE_INTERVAL"`
	withInternals      bool          `env:"WITH_INTERNALS"`
	user               string        `env:"USER"`
	otelOptions        otelsetup.Options
	dockerOptions      dockersetup.Options
	ociOptions         *ocisetup.Options
	kubeOptions        *kubesetup.Options
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

var (
	secretStore           = mask.NewSecretStore(mask.DefaultMask)
	stdout      io.Writer = os.Stdout
	stderr      io.Writer = os.Stderr
	isTerm                = term.IsTerminal(int(os.Stdout.Fd()))
)

const otelName = "github.com/raffis/rageta"

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

	sets := []struct {
		set         *pflag.FlagSet
		displayName string
	}{
		{
			set:         executionFlags,
			displayName: "Execution",
		},
		{
			set:         pipelineFlags,
			displayName: "Pipeline",
		},
		{
			set:         dockerFlags,
			displayName: "Docker runtime",
		},
		{
			set:         kubeFlags,
			displayName: "Kubernetes runtime",
		},
		{
			set:         otelFlags,
			displayName: "Open Telemetry",
		},
		{
			set:         ociFlags,
			displayName: "OCI Registry",
		},
		{
			set:         rootCmd.Flags(),
			displayName: "Global",
		},
	}

	runCmd.SetUsageFunc(func(c *cobra.Command) error {
		for _, group := range sets {
			fmt.Printf("%s\n%s\n", styles.Bold.Render(group.displayName), group.set.FlagUsages())
		}
		return nil
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

type containerRuntime string

var (
	containerRuntimeDocker     containerRuntime = "docker"
	containerRuntimeKubernetes containerRuntime = "kubernetes"
)

func (d containerRuntime) String() string {
	return string(d)
}

func electDefaultOutput() string {
	if isTerm {
		return renderOutputUI.String()
	}

	return renderOutputPrefix.String()
}

func electDefaultDriver() containerRuntime {
	docker, _ := isPossiblyInsideDocker()

	switch {
	case docker:
		return containerRuntimeDocker
		//case isPossiblyInsideKube():
		//		return containerRuntimeKubernetes
	}

	return containerRuntimeDocker
}

func isPossiblyInsideDocker() (bool, error) {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true, nil
	} else {
		return false, err
	}
}

func createContainerRuntime(ctx context.Context, d containerRuntime, logger logr.Logger, hideOutput bool) (cruntime.Interface, error) {
	logger.V(3).Info("create container runtime client", "container-runtime", d)

	switch d {
	case containerRuntimeDocker:
		runArgs.dockerOptions.Logger = logger
		c, err := runArgs.dockerOptions.Build()
		if err != nil {
			return nil, fmt.Errorf("failed to create docker client: %w", err)
		}

		driver := cruntime.NewDocker(c,
			cruntime.WithContext(ctx),
			cruntime.WithHidePullOutput(hideOutput),
			cruntime.WithLogger(logger),
		)

		return driver, err
	case containerRuntimeKubernetes:
		clientset, err := kubeRestClient(runArgs.kubeOptions.ConfigFlags)
		if err != nil {
			return nil, fmt.Errorf("failed to create kube client: %w", err)
		}

		driver := cruntime.NewKubernetes(clientset.CoreV1())
		return driver, err
	}

	return nil, errors.New("unknown container runtime")
}

func kubeRestClient(kubeconfigArgs *genericclioptions.ConfigFlags) (*kubernetes.Clientset, error) {
	config, err := kubeconfigArgs.ToRESTConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	return clientset, nil
}

func buildZapCore(config zap.Config, w io.Writer) (zapcore.Core, error) {
	var encoder zapcore.Encoder
	switch config.Encoding {
	case "json":
		encoder = zapcore.NewJSONEncoder(config.EncoderConfig)
	case "console":
		encoder = zapcore.NewConsoleEncoder(config.EncoderConfig)
	default:
		return nil, fmt.Errorf("failed setup step logger: no such log encoder `%s`", config.Encoding)
	}

	return zapcore.NewCore(
		encoder,
		zapcore.AddSync(w),
		config.Level,
	), nil
}

func logBuilder(defaultLog zapcore.Core, zapConfig zap.Config) processor.LogBuilder {
	return func(w io.Writer) (logr.Logger, error) {
		log, err := buildZapCore(zapConfig, w)
		if err != nil {
			return logr.Discard(), err
		}

		zapLogger := zap.New(zapcore.NewTee(defaultLog, log))
		return zapr.NewLogger(zapLogger), nil
	}
}

func tags() []processor.Tag {
	var tags []processor.Tag
	for _, tag := range runArgs.tags {
		v := strings.SplitN(tag, "=", 2)
		if len(v) == 2 {
			t := processor.Tag{
				Key: v[0],
			}

			value := strings.SplitN(v[1], ":", 2)
			if len(value) == 2 {
				t.Value = value[0]
				t.Color = value[1]
			} else {
				t.Value = v[1]
			}

			tags = append(tags, t)
		}
	}
	return tags
}

func stepBuilder(
	logBuilder processor.LogBuilder,
	logger logr.Logger,
	osEnv,
	envs,
	secrets map[string]string,
	secretStore *mask.SecretStore,
	celEnv *cel.Env,
	driver cruntime.Interface,
	imagePullPolicy cruntime.PullImagePolicy,
	tracer trace.Tracer,
	meter metric.Meter,
	outputFactory processor.OutputFactory,
	reporter processor.Reporter,
	teardown chan processor.Teardown,
	builder *processor.PipelineBuilder,
	provider provider.Interface,
	pool pond.Pool,
	template v1beta1.Template,
	monitorDev io.Writer,
	tags []processor.Tag,
) pipeline.StepBuilder {
	return func(spec v1beta1.Step) []processor.Bootstraper {
		processors := processor.Builder(&spec,
			processor.WithRecover(),
			processor.WithReport(reporter),
			processor.WithRetry(),
			processor.WithResult(),
			processor.WithInputVars(celEnv),
			processor.WithEnvVars(osEnv, envs),
			processor.WithSecretVars(osEnv, secrets, secretStore),
			processor.WithOutputVars(),
			processor.WithTags(tags),
			processor.WithMatrix(pool),
			processor.WithOutput(outputFactory, runArgs.withInternals, runArgs.expand),
			processor.WithMonitor(!runArgs.noStatus, runArgs.waitUpdateInterval, monitorDev),
			processor.WithOtelTrace(logger, tracer),
			processor.WithLogger(logger, logBuilder, runArgs.logDetached),
			processor.WithOtelMetrics(meter),
			processor.WithSkipBlacklist(runArgs.skipSteps),
			processor.WithGarbageCollector(runArgs.noGC, driver, teardown),
			processor.WithAllowFailure(),
			processor.WithTimeout(),
			processor.WithSkipDone(runArgs.skipDone),
			processor.WithIf(celEnv),
			processor.WithTmpDir(),
			processor.WithTemplate(template),
			processor.WithNeeds(),
			processor.WithStdioRedirect(runArgs.tee),
			processor.WithRun(imagePullPolicy, driver, outputFactory, teardown),
			processor.WithInherit(*builder, provider),
			processor.WithAnd(),
			processor.WithConcurrent(pool),
			processor.WithPipe(runArgs.tee),
		)

		return processor.WithDebug(logger, zapConfig.Level.Level() <= -5, &spec, processors...)
	}
}

func runRun(cmd *cobra.Command, args []string) error {
	logFile, err := os.CreateTemp(os.TempDir(), "rageta-log")
	if err != nil {
		return err
	}

	maskedLog := secretStore.Writer(logFile)
	stdout = secretStore.Writer(os.Stdout)

	if isTerm {
		stderr = stdout
	} else {
		stderr = secretStore.Writer(os.Stderr)
	}

	logCoreFile, err := buildZapCore(zapConfig, maskedLog)
	if err != nil {
		return err
	}

	traceProvider, err := runArgs.otelOptions.BuildTraceProvider(context.Background())
	if err != nil {
		return err
	}

	var tracer trace.Tracer
	if traceProvider != nil {
		tracer = traceProvider.Tracer(otelName)
		defer func() {
			if err := traceProvider.Shutdown(context.Background()); err != nil {
				logger.V(3).Error(err, "failed to shutdown tracer provider")
			}
		}()
	}

	meterProvider, err := runArgs.otelOptions.BuildMeterProvider(context.Background())
	if err != nil {
		return err
	}

	var meter metric.Meter
	if meterProvider != nil {
		meter = meterProvider.Meter(otelName)
		defer func() {
			if meterProvider != nil {
				if err := meterProvider.Shutdown(context.Background()); err != nil {
					logger.V(3).Error(err, "failed to shutdown meter provider")
				}
			}
		}()
	}

	logProvider, err := runArgs.otelOptions.BuildLoggerProvider(context.Background())
	if err != nil {
		return err
	}

	var loggr log.Logger
	if logProvider != nil {
		loggr = logProvider.Logger(otelName)
		defer func() {
			if logProvider != nil {
				if err := logProvider.Shutdown(context.Background()); err != nil {
					logger.V(3).Error(err, "failed to shutdown log provider")
				}
			}
		}()
	}

	defaultLog := logCoreFile
	if loggr != nil {
		defaultLog = zapcore.NewTee(logCoreFile, logbridge.OtelCore(loggr))
	}

	logBuilder := logBuilder(defaultLog, zapConfig)

	if runArgs.output == renderOutputUI.String() {
		logger = zapr.NewLogger(zap.New(defaultLog))
	} else {
		logger, err = logBuilder(stderr)
		if err != nil {
			return err
		}
	}

	reportOutput := runArgs.reportOutput
	var reportDev io.Writer

	if reportOutput == "/dev/stdout" || reportOutput == "" {
		reportDev = stdout
	} else {
		output, err := os.OpenFile(reportOutput, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0640)
		if err != nil {
			return err
		}

		reportDev = secretStore.Writer(output)
	}

	envs := envMap(runArgs.envs)
	secrets := envMap(runArgs.secretEnvs)

	for _, secretValue := range secrets {
		secretStore.AddSecrets([]byte(secretValue))
	}

	logger.V(5).Info("run flags", "args", runArgs)
	logger.V(1).Info("log file at", "path", logFile.Name())

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	if rootArgs.timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, rootArgs.timeout)
		defer cancel()
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	driver, err := createContainerRuntime(ctx, containerRuntime(runArgs.containerRuntime), logger, runArgs.dockerQuiet)
	if err != nil {
		return err
	}

	contextDir := runArgs.contextDir
	if contextDir == "" {
		tmpDir, err := os.MkdirTemp(os.TempDir(), "rageta")
		if err != nil {
			return fmt.Errorf("failed to create tmp dir: %w", err)
		}

		contextDir = tmpDir
	}

	logger.V(1).Info("use context directory", "path", contextDir)

	template, err := buildTemplate(contextDir)
	if err != nil {
		return fmt.Errorf("failed to create template: %w", err)
	}

	imagePullPolicy, err := imagePullPolicy()
	if err != nil {
		return err
	}

	if runArgs.fork {
		return fork(ctx, driver, template, envs, imagePullPolicy)
	}

	var ref string
	if len(args) > 0 && !strings.HasPrefix(args[0], "--") {
		ref = args[0]
	}

	store, persistDB := createProvider(imagePullPolicy, rootArgs.dbPath, runArgs.ociOptions)
	logger.V(3).Info("resolve pipeline reference", "source", ref)
	command, err := store.Resolve(ctx, ref)
	if err != nil {
		return err
	}

	celEnv, err := cel.NewEnv(
		ext.Strings(),
		ext.Math(),
		ext.Lists(),
		ext.Encoders(),
		ext.Sets(),
		ext.NativeTypes(ext.ParseStructTags(true),
			reflect.TypeOf(&v1beta1.Context{}),
			reflect.TypeOf(&v1beta1.StepResult{}),
			reflect.TypeOf(&v1beta1.ParamValue{}),
			reflect.TypeOf(&v1beta1.Output{}),
			reflect.TypeOf(&v1beta1.ContainerStatus{}),
		),
		cel.Variable("context", cel.ObjectType("v1beta1.Context")),
	)

	if err != nil {
		return fmt.Errorf("setup cel env failed: %w", err)
	}

	outputFactory, err := outputFactory(logger, cancel)
	if err != nil {
		return err
	}

	reportFactory, err := reportFactory(reportDev)
	if err != nil {
		return err
	}

	var teardownFuncs []processor.Teardown
	teardown := make(chan processor.Teardown)

	go func() {
		for teardownFunc := range teardown {
			teardownFuncs = append(teardownFuncs, teardownFunc)
		}
	}()

	logger.V(3).Info("worker pool", "max-concurrency", runArgs.maxConcurrent)
	pool := pond.NewPool(runArgs.maxConcurrent)

	var monitorDev io.Writer
	switch {
	case runArgs.statusOutput == "/dev/stdout" || runArgs.statusOutput == "-":
		monitorDev = stdout
	case runArgs.statusOutput == "/dev/stderr":
		monitorDev = stderr
	case runArgs.statusOutput != "":
		f, err := os.OpenFile(runArgs.statusOutput, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0640)
		if err != nil {
			return err
		}

		defer func() {
			_ = f.Close()
		}()

		monitorDev = f
	case runArgs.output == renderOutputDiscard.String() || runArgs.output == renderOutputPassthrough.String():
		monitorDev = stdout
	default:
		monitorDev = nil
	}

	var builder processor.PipelineBuilder
	builder = pipeline.NewBuilder(
		pipeline.WithStepBuilder(stepBuilder(
			logBuilder,
			logger,
			osEnvMap(),
			envs,
			secrets,
			secretStore,
			celEnv,
			driver,
			imagePullPolicy,
			tracer,
			meter,
			outputFactory,
			reportFactory,
			teardown,
			&builder,
			store,
			pool,
			template,
			monitorDev,
			tags(),
		)),
		pipeline.WithLogger(logger),
		pipeline.WithTmpDir(contextDir),
	)

	go func() {
		sig := <-signals
		logger.V(1).Info("received signal", "signal", sig)
		cancel()
	}()

	flagSet := pflag.NewFlagSet("inputs", pflag.ContinueOnError)
	pipeline.Flags(flagSet, command.Inputs)
	if flagStart := slices.Index(os.Args, "--"); flagStart != -1 {
		err = flagSet.Parse(os.Args[flagStart+1:])
		if err != nil {
			cleanup()
			helpAndExit(flagSet, err)
		}
	}

	inputs, err := parseInputs(command.Inputs, runArgs.inputs, flagSet)
	if err != nil {
		if err := persistDB(); err != nil {
			logger.V(1).Error(err, "failed to persist database")
		}

		cleanup()
		helpAndExit(flagSet, err)
	}

	var result error
	command.Name = ""

	stepCtx := processor.NewContext()
	stepCtx.Context = ctx

	pipelineCmd, err := builder.Build(command, runArgs.entrypoint, inputs, stepCtx)
	if err != nil {
		result = err
	} else {
		result = retryRun(ctx, pipelineCmd)
	}

	logger.V(1).Info("pipeline completed", "result", result)
	if err := persistDB(); err != nil {
		logger.V(1).Error(err, "failed to persist database")
	}

	tearDown := func() {
		cancel()
		close(teardown)

		// The container engines handle timeouts on the server, to avoid canceling the context before the timeout
		// on the remote is reached we add one second here.
		teardownCtx, cancel := context.WithTimeout(context.Background(), runArgs.gracefulTermination+time.Second)
		defer cancel()
		var wg sync.WaitGroup

		for _, teardownFunc := range teardownFuncs {
			wg.Add(1)
			go func(teardownFunc processor.Teardown) {
				defer wg.Done()
				logger.V(5).Info("execute teardown")
				if err := teardownFunc(teardownCtx, runArgs.gracefulTermination); err != nil {
					logger.V(5).Info("failed execute teardown", "err", err)
				}
			}(teardownFunc)
		}

		wg.Wait()
	}

	if tuiDone != nil {
		if errors.Is(result, pipeline.ErrInvalidInput) {
			tuiApp.Quit()
		}

		if result != nil {
			tuiApp.Send(tui.PipelineDoneMsg{Status: tui.StepStatusFailed, Error: result})
		} else {
			tuiApp.Send(tui.PipelineDoneMsg{Status: tui.StepStatusDone, Error: result})
		}

		tearDown()
		<-tuiDone
	} else {
		tearDown()
	}

	if runArgs.contextDir == "" && !runArgs.noGC {
		defer func() {
			_ = os.RemoveAll(contextDir)
		}()
	}

	if reportFactory != nil {
		if err := reportFactory.Finalize(); err != nil {
			result = errors.Join(result, err)
		}
	}

	if result != nil {
		helpAndExit(flagSet, result)
	}

	return nil
}

func cleanup() {
	if tuiDone == nil {
		return
	}

	tuiApp.Quit()
	<-tuiDone
}

func createProvider(imagePullPolicy cruntime.PullImagePolicy, dbPath string, ociOptions *ocisetup.Options) (provider.Interface, func() error) {
	scheme := kruntime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)

	factory := serializer.NewCodecFactory(scheme)
	decoder := factory.UniversalDeserializer()
	encoder := json.NewYAMLSerializer(json.DefaultMetaFactory, scheme, scheme)
	var localDB *provider.Database
	var mu sync.Mutex

	openDB := func() (*provider.Database, error) {
		mu.Lock()
		defer mu.Unlock()

		if localDB == nil {
			dbFile, err := os.OpenFile(dbPath, os.O_RDONLY|os.O_CREATE, 0644)
			if err != nil {
				return nil, err
			}

			localDB, err = provider.OpenDatabase(dbFile, decoder, encoder)
			if err != nil {
				return nil, err
			}
		}

		return localDB, nil
	}

	localDBProviderWrapper := func(ctx context.Context, ref string) (io.Reader, error) {
		localDB, err := openDB()
		if err != nil {
			return nil, err
		}

		return provider.WithLocalDB(localDB)(ctx, ref)
	}

	ociProviderWrapper := func(ctx context.Context, ref string) (io.Reader, error) {
		ociOptions.URL = ref
		ociClient, err := ociOptions.Build(ctx)
		if err != nil {
			return nil, err
		}

		r, err := provider.WithOCI(ociClient)(ctx, ref)
		if err != nil {
			return nil, err
		}

		localDB, err := openDB()
		if err != nil {
			return r, err
		}

		manifest, err := io.ReadAll(r)
		if err != nil {
			return r, err
		}

		r = bytes.NewReader(manifest)
		err = localDB.Add(ref, manifest)

		return r, err
	}

	providers := []provider.Resolver{
		provider.WithFile(),
		provider.WithRagetafile(),
	}

	// If pull policy is always, the oci provider has priority over the local cache
	if imagePullPolicy == cruntime.PullImagePolicyAlways {
		providers = append(providers,
			ociProviderWrapper,
			localDBProviderWrapper,
		)
	} else {
		providers = append(providers,
			localDBProviderWrapper,
			ociProviderWrapper,
		)
	}

	return provider.New(decoder, providers...), func() error {
		if localDB == nil {
			return nil
		}

		return persistDatabase(dbPath, localDB)
	}
}

func persistDatabase(dbPath string, db *provider.Database) error {
	scheme := kruntime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	factory := serializer.NewCodecFactory(scheme)
	decoder := factory.UniversalDeserializer()
	encoder := json.NewYAMLSerializer(json.DefaultMetaFactory, scheme, scheme)

	fileLock := flock.New(fmt.Sprintf("%s.lock", dbPath))
	locked, err := fileLock.TryLock()

	if !locked || err != nil {
		return fmt.Errorf("failed to lock database: %w", err)
	}

	defer func() {
		_ = fileLock.Unlock()
	}()

	dbFile, err := os.OpenFile(dbPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	localDB, err := provider.OpenDatabase(dbFile, decoder, encoder)
	if err != nil {
		return err
	}

	err = localDB.Merge(db)
	if err != nil {
		return err
	}

	err = localDB.Persist(dbFile)
	return err
}

func retryRun(ctx context.Context, pipeline processor.Executable) error {
	var backoff retry.Backoff
	return retry.Do(ctx, retry.WithMaxRetries(runArgs.retry, backoff), func(ctx context.Context) error {
		_, _, err := pipeline()

		if err != nil {
			return retry.RetryableError(err)
		}

		return nil
	})
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

func fork(ctx context.Context, driver cruntime.Interface, template v1beta1.Template, env map[string]string, imagePullPolicy cruntime.PullImagePolicy) error {
	logger.V(0).Info("fork pipeline runner, attaching streams. This process can be exited using ctrl+c")

	forkFlags := os.Args[1:]
	forkFlags = slices.DeleteFunc(forkFlags, func(s string) bool {
		return s == "--fork"
	})

	container := cruntime.ContainerSpec{
		Name:            "rageta",
		Image:           "ghcr.io/rageta/rageta:latest",
		Args:            forkFlags,
		Stdin:           true,
		TTY:             isTerm,
		Env:             env,
		ImagePullPolicy: imagePullPolicy,
	}
	/*
		processor.ContainerSpec(&container, &template)
		if runArgs.containerRuntime == containerRuntimeDocker.String() {
			container.Volumes = append(container.Volumes, cruntime.Volume{
				Name:     "docker-sock",
				HostPath: "/var/run/docker.sock",
				Path:     "/var/run/docker.sock",
			})
		}
	*/
	pod := cruntime.Pod{
		Name: fmt.Sprintf("rageta-%s", utils.RandString(5)),
		Spec: cruntime.PodSpec{
			Containers: []cruntime.ContainerSpec{
				container,
			},
		},
	}

	status, err := driver.CreatePod(ctx, &pod, os.Stdin, stdout, stderr)
	if err != nil {
		return err
	}

	if !runArgs.noGC {
		defer func() {
			_ = driver.DeletePod(ctx, &pod, runArgs.gracefulTermination)
		}()
	}

	return status.Wait()
}

func buildTemplate(contextDir string) (v1beta1.Template, error) {
	tmpl := v1beta1.Template{}

	runArgs.volumes = append(runArgs.volumes, fmt.Sprintf("%s:%s", contextDir, contextDir))

	for i, volume := range runArgs.volumes {
		v := strings.Split(volume, ":")
		if len(v) != 2 {
			return tmpl, errors.New("invalid volume mount provided")
		}

		tmpl.VolumeMounts = append(tmpl.VolumeMounts, v1beta1.VolumeMount{
			Name:      fmt.Sprintf("volume-%d", i),
			MountPath: v[1],
			HostPath:  v[0],
		})
	}

	if runArgs.user != "" {
		user := strings.SplitN(runArgs.user, ":", 2)
		uid, err := getUid(user[0])
		if err != nil {
			return tmpl, err
		}

		intOrStr := intstr.FromInt(uid)
		tmpl.Uid = &intOrStr

		if len(user) == 2 {
			guid, err := getGuid(user[1])
			if err != nil {
				return tmpl, err
			}

			intOrStr := intstr.FromInt(guid)
			tmpl.Guid = &intOrStr
		}
	}

	return tmpl, nil
}

func getUid(name string) (int, error) {
	if uid, err := strconv.Atoi(name); err == nil {
		return uid, nil
	}

	u, err := user.Lookup(name)
	if err != nil {
		return 0, err
	}

	if uid, err := strconv.Atoi(u.Uid); err == nil {
		return uid, nil
	}

	return 0, nil
}

func getGuid(name string) (int, error) {
	if uid, err := strconv.Atoi(name); err == nil {
		return uid, nil
	}

	g, err := user.LookupGroup(name)
	if err != nil {
		return 0, err
	}

	if guid, err := strconv.Atoi(g.Gid); err == nil {
		return guid, nil
	}

	return 0, nil
}

// parseInputs parses a list of input strings and maps them to their corresponding pipeline input
func parseInputs(params []v1beta1.InputParam, inputs []string, flagSet *pflag.FlagSet) (map[string]v1beta1.ParamValue, error) {
	result := make(map[string]v1beta1.ParamValue)
	steps := make(map[string][]string)

	for _, v := range inputs {
		flag := strings.SplitN(v, "=", 2)
		if len(flag) != 2 {
			return result, errors.New("expected input key=value")
		}

		steps[flag[0]] = append(steps[flag[0]], flag[1])
	}

	for _, v := range params {
		if input, ok := steps[v.Name]; ok {
			x := result[v.Name]

			if len(input) == 1 {
				if err := x.UnmarshalJSON([]byte(input[0])); err != nil {
					return result, fmt.Errorf("failed to decode input: %w", err)
				}

				result[v.Name] = x
				continue
			}

			x.Type = v1beta1.ParamTypeArray
			x.ArrayVal = input
			result[v.Name] = x
		}
	}

	flagSet.Visit(func(f *pflag.Flag) {
		switch f.Value.Type() {
		case "string":
			val, _ := flagSet.GetString(f.Name)
			result[f.Name] = v1beta1.ParamValue{
				Type:      v1beta1.ParamTypeString,
				StringVal: val,
			}
		case "stringSlice":
			val, _ := flagSet.GetStringSlice(f.Name)
			result[f.Name] = v1beta1.ParamValue{
				Type:     v1beta1.ParamTypeArray,
				ArrayVal: val,
			}
		case "stringToString":
			val, _ := flagSet.GetStringToString(f.Name)
			result[f.Name] = v1beta1.ParamValue{
				Type:      v1beta1.ParamTypeObject,
				ObjectVal: val,
			}
		}
	})

	return result, nil
}

func imagePullPolicy() (cruntime.PullImagePolicy, error) {
	switch runArgs.pull {
	case pullImageAlways.String():
		return cruntime.PullImagePolicyAlways, nil
	case pullImageMissing.String():
		return cruntime.PullImagePolicyMissing, nil
	case pullImageNever.String():
		return cruntime.PullImagePolicyNever, nil
	default:
		return "", fmt.Errorf("invalid pull policy given: %s", runArgs.pull)
	}
}

func osEnvMap() map[string]string {
	envs := make(map[string]string)
	for _, v := range os.Environ() {
		s := strings.SplitN(v, "=", 2)
		envs[s[0]] = s[1]
	}

	return envs
}

func envMap(from []string) map[string]string {
	envs := make(map[string]string)
	for _, v := range from {
		s := strings.SplitN(v, "=", 2)
		if len(s) == 1 {
			if env, ok := os.LookupEnv(s[0]); ok {
				envs[s[0]] = env
			}
		} else {
			envs[s[0]] = s[1]
		}
	}

	return envs
}

var (
	tuiModel tea.Model
	tuiDone  chan struct{}
	tuiApp   *tea.Program
)

func uiOutput(logger logr.Logger, cancel context.CancelFunc) *tea.Program {
	if runArgs.output != "ui" {
		return nil
	}

	if tuiApp != nil {
		return tuiApp
	}

	tuiDone = make(chan struct{})
	tuiModel = tui.NewUI(logger.WithValues("component", "tui"))
	tuiApp = tea.NewProgram(
		tuiModel,
		tea.WithOutput(xio.NewFDWrapper(stdout, os.Stdout)),
	)

	go func() {
		for c := range time.Tick(100 * time.Millisecond) {
			tuiApp.Send(tui.TickMsg(c))
		}
	}()

	go func() {
		_, _ = tuiApp.Run()
		if err := tuiApp.ReleaseTerminal(); err != nil {
			logger.V(3).Error(err, "failed to release terminal")
		}

		fmt.Print("\033[H\033[2J")
		cancel()
		tuiDone <- struct{}{}
	}()

	return tuiApp
}

func outputFactory(logger logr.Logger, cancel context.CancelFunc) (processor.OutputFactory, error) {
	var outputHandler processor.OutputFactory

	outputOpt := strings.Split(runArgs.output, "=")
	var renderer string
	var opts string

	renderer = outputOpt[0]
	if len(outputOpt) == 2 {
		opts = outputOpt[1]
	}

	switch renderer {
	case renderOutputUI.String():
		outputHandler = output.UI(uiOutput(logger, cancel))
	case renderOutputPrefix.String():
		outputHandler = output.Prefix(stdout, stderr)
	case renderOutputPassthrough.String():
		outputHandler = output.Passthrough(stdout, stderr)
	case renderOutputDiscard.String():
		outputHandler = output.Discard()
	case renderOutputBuffer.String():
		if opts == "" {
			opts = renderOutputBufferDefaultTemplate
		}

		tmpl, err := template.New("output").Parse(opts)
		if err != nil {
			return nil, fmt.Errorf("failed to parse report buffer template: %w", err)
		}

		outputHandler = output.Buffer(tmpl, stdout)
	default:
		return nil, fmt.Errorf("invalid output type given: %s", runArgs.output)
	}

	return outputHandler, nil
}

type reporter interface {
	report.Finalizer
	processor.Reporter
}

func reportFactory(w io.Writer) (reporter, error) {
	switch runArgs.report {
	case reportTypeNone.String():
		return nil, nil
	case reportTypeTable.String():
		return report.Table(w), nil
	case reportTypeMarkdown.String():
		return report.Markdown(w), nil
	default:
		return nil, fmt.Errorf("invalid report type given: %s", runArgs.report)
	}
}
