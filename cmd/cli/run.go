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
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	"github.com/alitto/pond/v2"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-logr/logr"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
	"github.com/raffis/rageta/internal/dockersetup"
	"github.com/raffis/rageta/internal/ocisetup"
	"github.com/raffis/rageta/internal/otelsetup"
	"github.com/raffis/rageta/internal/output"
	"github.com/raffis/rageta/internal/pipeline"
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/report"
	cruntime "github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/internal/styles"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/raffis/rageta/internal/storage"
	"github.com/raffis/rageta/internal/tui"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	transport "github.com/raffis/rageta/pkg/http/middleware"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.uber.org/zap"
	"golang.org/x/term"
)

var runCmd = &cobra.Command{
	Use:  "run",
	RunE: runRun,
}

type runFlags struct {
	dbPath              string        `env:"DB_PATH"`
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
	volumes             []string      `env:"VOLUMES"`
	skipDone            bool          `env:"SKIP_DONE"`
	skipSteps           []string      `env:"SKIP_STEPS"`
	logDetached         bool          `env:"LOG_DETACHED"`
	maxConcurrent       int           `env:"MAX_CONCURRENT"`
	decouple            bool          `env:"DECOUPLE"`
	noProgress          bool          `env:"NO_PROGRESS"`
	withInternals       bool          `env:"WITH_INTERNALS"`
	user                string        `env:"USER"`
	otelOptions         otelsetup.Options
	dockerOptions       dockersetup.Options
	ociOptions          *ocisetup.Options
}

var runArgs = newRunFlags()

func newRunFlags() runFlags {
	return runFlags{
		ociOptions: ocisetup.DefaultOptions(),
	}
}

const otelName = "github.com/raffis/rageta"

func init() {
	dbPath := "/rageta.db"
	home, err := os.UserHomeDir()
	if err == nil {
		dbPath = filepath.Join(home, "rageta.db")
	}

	executionFlags := pflag.NewFlagSet("execution", pflag.ExitOnError)
	executionFlags.StringVarP(&runArgs.dbPath, "db-path", "", dbPath, "Path to the local rageta pipeline store.")
	executionFlags.BoolVarP(&runArgs.tee, "tee", "", false, "Dump any internal redirected streams to stdout. Works similar as piping to tee on the console.")
	executionFlags.StringVarP(&runArgs.output, "output", "o", electDefaultOutput(), "Output renderer. One of [prefix, prefix-nocolor, ui, json, buffer[=gotpl], passthrough]. The default `prefix` adds a colored task name prefix to the output lines while `ui` renders the tasks in a terminal ui. `passthrough` dumps all outputs directly without any modification.")
	executionFlags.BoolVarP(&runArgs.noGC, "no-gc", "", false, "Keep all containers and temporary files after execution. Useful for debugging purposes.")
	executionFlags.BoolVarP(&runArgs.decouple, "decouple", "", false, "Decouple steps from inherited pipelines and display them as separate entities.")
	executionFlags.IntVarP(&runArgs.maxConcurrent, "max-concurrent", "", runtime.NumCPU(), "Maximum number of concurrent steps. Affects concurrent and matrix steps.")
	executionFlags.BoolVarP(&runArgs.noProgress, "no-progress", "", false, "Do not print wait updates for steps to stderr")
	executionFlags.BoolVarP(&runArgs.withInternals, "with-internals", "", false, "Expose internal steps")
	executionFlags.BoolVarP(&runArgs.skipDone, "skip-done", "", false, "Skip steps which have been successfully processed before. This is only useful in combination with a static context directory `--context-dir`.")
	executionFlags.StringSliceVarP(&runArgs.skipSteps, "skip-steps", "", nil, "Do not executed these steps")
	executionFlags.DurationVarP(&runArgs.gracefulTermination, "graceful-termination", "", time.Second*5, "Allow containers to exit gracefully.")
	executionFlags.StringVarP(&runArgs.containerRuntime, "container-runtime", "", electDefaultDriver().String(), "Container runtime. One of [docker].")
	executionFlags.StringVarP(&runArgs.report, "report", "r", "none", "Report summary of steps at the end of execution. One of [none, table, json, markdown].")
	executionFlags.StringVarP(&runArgs.reportOutput, "report-output", "", electDefaultReportOutput(), "Destination for the report output.")
	executionFlags.StringVarP(&runArgs.pull, "pull", "", pullImageMissing.String(), "Pull image before running. one of [always, missing, never].")
	executionFlags.StringVarP(&runArgs.contextDir, "context-dir", "", "", "Use a static context directory. If any context is found it attempts to recover it.")
	runCmd.Flags().AddFlagSet(executionFlags)

	pipelineFlags := pflag.NewFlagSet("pipeline", pflag.ExitOnError)
	pipelineFlags.StringVarP(&runArgs.entrypoint, "entrypoint", "t", "", "Entrypoint for the given pipeline. The pipelines default is used otherwise.")
	pipelineFlags.StringSliceVarP(&runArgs.envs, "env", "e", nil, "Pass envs to the pipeline.")
	pipelineFlags.StringSliceVarP(&runArgs.volumes, "volumes", "v", nil, "Pass volumes to the pipeline.")
	pipelineFlags.StringArrayVarP(&runArgs.inputs, "input", "i", nil, "Pass inputs to the pipeline.")
	pipelineFlags.StringVarP(&runArgs.user, "user", "", "", "Username or UID (format: <name|uid>[:<group|gid>])")
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
	reportTypeJSON     reportType = "json"
	reportTypeMarkdown reportType = "markdown"
	reportTypeTimeline reportType = "timeline"
)

func (d reportType) String() string {
	return string(d)
}

type renderOutput string

var (
	renderOutputPrefix                renderOutput = "prefix"
	renderOutputPrefixNoColor         renderOutput = "prefix-nocolor"
	renderOutputUI                    renderOutput = "ui"
	renderOutputPassthrough           renderOutput = "passthrough"
	renderOutputJSON                  renderOutput = "json"
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
	switch {
	case os.Getenv("GITHUB_ACTIONS") == "true":
		renderOutputBufferDefaultTemplate = `{{- if .Error }}{{ printf "%s %s\n%s\n" .Symbol .StepName .Buffer }}{{- else }}{{ printf "::group::%s %s\n%s\n::endgroup::\n" .Symbol .StepName .Buffer }}{{- end }}`
		return fmt.Sprintf("%s=%s", renderOutputBuffer.String(), renderOutputBufferDefaultTemplate)
	case term.IsTerminal(int(os.Stdout.Fd())):
		return renderOutputPrefix.String()
	default:
		return renderOutputPrefixNoColor.String()
	}
}

func electDefaultReportOutput() string {
	if os.Getenv("GITHUB_STEP_SUMMARY") != "" {
		return os.Getenv("GITHUB_STEP_SUMMARY")
	}

	return os.Stdout.Name()
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

func isPossiblyInsideKube() bool {
	_, set := os.LookupEnv("KUBERNETES_SERVICE_HOST")
	return set
}

func createContainerRuntime(ctx context.Context, d containerRuntime, logger logr.Logger, hideOutput bool, contextDir string) (cruntime.Interface, error) {
	switch {
	case d == containerRuntimeDocker:
		c, err := runArgs.dockerOptions.Build()
		if err != nil {
			return nil, fmt.Errorf("failed to create docker client: %w", err)
		}

		c.HTTPClient().Transport = transport.NewLogger(logger, c.HTTPClient().Transport)

		driver := cruntime.NewDocker(c,
			cruntime.WithContext(ctx),
			cruntime.WithHidePullOutput(hideOutput),
		)

		return driver, err
	}

	return nil, errors.New("unknown container runtime")
}

func getRunLogger() (logr.Logger, *os.File, error) {
	if runArgs.output == renderOutputUI.String() {
		f, err := os.CreateTemp(os.TempDir(), "rageta-log")
		if err != nil {
			return logger, nil, err
		}

		config := zap.NewDevelopmentConfig()
		config.ErrorOutputPaths = []string{f.Name()}
		config.OutputPaths = []string{f.Name()}
		l, err := buildLogger(config)
		if err != nil {
			return logger, f, err
		}

		return l, f, nil
	}

	return logger, os.Stderr, nil
}

func stepBuilder(
	logger logr.Logger,
	osEnv,
	envs map[string]string,
	celEnv *cel.Env,
	driver cruntime.Interface,
	imagePullPolicy cruntime.PullImagePolicy,
	tracer trace.Tracer,
	meter metric.Meter,
	outputFactory processor.OutputFactory,
	resultStore processor.ResultStore,
	teardown chan processor.Teardown,
	builder *processor.PipelineBuilder,
	store storage.Interface,
	pool pond.Pool,
	template v1beta1.Template,
) pipeline.StepBuilder {
	return func(spec v1beta1.Step) []processor.Bootstraper {
		processors := processor.Builder(&spec,
			processor.WithRecover(),
			processor.WithReport(resultStore),
			processor.WithRetry(),
			processor.WithProgress(!runArgs.noProgress),
			processor.WithResult(),
			processor.WithInputVars(celEnv),
			processor.WithEnvVars(osEnv, envs),
			processor.WithOutputVars(),
			processor.WithMatrix(pool),
			processor.WithOutput(outputFactory, runArgs.withInternals, runArgs.decouple),
			processor.WithOtelTrace(logger, tracer),
			processor.WithLogger(logger, &zapConfig),
			processor.WithOtelMetrics(meter),
			processor.WithSkipBlacklist(runArgs.skipSteps),
			processor.WithGarbageCollector(runArgs.noGC, driver, teardown),
			processor.WithAllowFailure(),
			processor.WithTimeout(),
			processor.WithSkipDone(runArgs.skipDone),
			processor.WithIf(celEnv),
			processor.WithTemplate(template),
			processor.WithNeeds(),
			processor.WithTmpDir(),
			processor.WithStdioRedirect(runArgs.tee),
			processor.WithRun(imagePullPolicy, driver, outputFactory, teardown),
			processor.WithInherit(*builder, store),
			processor.WithAnd(),
			processor.WithConcurrent(pool),
			processor.WithPipe(runArgs.tee),
		)

		logger.V(1).Info("register step", "spec", spec)
		for _, processor := range processors {
			logger.V(1).Info("register step processor", "processor", fmt.Sprintf("%T", processor))
		}

		return processors
	}
}

func runRun(cmd *cobra.Command, args []string) error {
	logger, _, err := getRunLogger()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	if rootArgs.timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, rootArgs.timeout)
		defer cancel()
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	scheme := kruntime.NewScheme()
	v1beta1.AddToScheme(scheme)
	factory := serializer.NewCodecFactory(scheme)
	decoder := factory.UniversalDeserializer()

	var ref string
	if len(args) > 0 && !strings.HasPrefix(args[0], "--") {
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

	contextDir := runArgs.contextDir
	if contextDir == "" {
		tmpDir, err := os.MkdirTemp(os.TempDir(), "rageta")
		if err != nil {
			return fmt.Errorf("failed to create tmp dir: %w", err)
		}

		contextDir = tmpDir
	}

	logger.Info("use context directory", "path", contextDir)

	driver, err := createContainerRuntime(ctx, containerRuntime(runArgs.containerRuntime), logger, runArgs.dockerQuiet, contextDir)
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

	imagePullPolicy, err := imagePullPolicy()
	if err != nil {
		return err
	}

	tp, err := runArgs.otelOptions.BuildTracer(context.Background())
	if err != nil {
		return err
	}

	defer tp.Shutdown(context.Background())
	meter := otel.Meter(otelName)

	outputFactory, err := outputFactory(cancel)
	if err != nil {
		return err
	}

	var resultStore processor.ResultStore

	if runArgs.report != "" && runArgs.report != "none" {
		resultStore = &report.Store{}
	}

	var teardownFuncs []processor.Teardown
	teardown := make(chan processor.Teardown)

	go func() {
		for teardownFunc := range teardown {
			teardownFuncs = append(teardownFuncs, teardownFunc)
		}
	}()

	pool := pond.NewPool(runArgs.maxConcurrent)
	template, err := buildTemplate(contextDir)
	if err != nil {
		return fmt.Errorf("failed to create template: %w", err)
	}

	var builder processor.PipelineBuilder
	builder = pipeline.NewBuilder(
		pipeline.WithStepBuilder(stepBuilder(logger, osEnvMap(), envMap(), celEnv, driver, imagePullPolicy, tp.Tracer(otelName), meter, outputFactory, resultStore, teardown, &builder, store, pool, template)),
		pipeline.WithLogger(logger),
		pipeline.WithTmpDir(contextDir),
	)

	go func() {
		sig := <-signals
		logger.Info("received signal", "signal", sig)
		cancel()
	}()

	flagSet := pflag.NewFlagSet("inputs", pflag.ContinueOnError)
	pipeline.Flags(flagSet, command.Inputs)

	if flagStart := slices.Index(os.Args, "--"); flagStart != -1 {
		err = flagSet.Parse(os.Args[flagStart+1:])
		if err != nil {
			return err
		}
	}

	inputs, err := parseInputs(command.Inputs, runArgs.inputs, flagSet)
	if err != nil {
		return err
	}

	var result error
	command.PipelineSpec.Name = ""
	pipelineCmd, err := builder.Build(command, runArgs.entrypoint, inputs, processor.NewContext())
	if err != nil {
		result = err
	} else {
		_, _, result = pipelineCmd(ctx)
	}

	logger.Info("pipeline completed", "result", result)

	if tuiDone != nil {
		if errors.Is(result, pipeline.ErrInvalidInput) {
			tuiApp.Quit()
		}

		if result != nil {
			tuiModel.SetStatus(tui.StepStatusFailed)
		} else {
			tuiModel.SetStatus(tui.StepStatusDone)
		}

		if resultStore, ok := resultStore.(*report.Store); ok {
			buf := &bytes.Buffer{}

			if err := printReport(buf, resultStore); err != nil {
				fmt.Fprintln(buf, err)
			}

			tuiModel.Report(buf.Bytes())
		}

		<-tuiDone
		tuiApp.ReleaseTerminal()
	}

	if prefixOutputDone != nil {
		close(prefixOutputCH)
		<-prefixOutputDone
	}

	cancel()
	close(teardown)

	if runArgs.contextDir == "" {
		defer func() {
			_ = os.RemoveAll(contextDir)
		}()
	}

	teardownCtx, cancel := context.WithTimeout(context.Background(), runArgs.gracefulTermination)
	defer cancel()
	var wg sync.WaitGroup

	for _, teardownFunc := range teardownFuncs {
		wg.Add(1)
		go func(teardownFunc processor.Teardown) {
			defer wg.Done()
			logger.V(1).Info("execute teardown", "func", teardownFunc)
			if err := teardownFunc(teardownCtx); err != nil {
				logger.Error(err, "failed execute teardown")
			}
		}(teardownFunc)
	}

	wg.Wait()

	if resultStore, ok := resultStore.(*report.Store); ok && len(resultStore.Ordered()) != 0 {
		outputPath := runArgs.reportOutput
		var output *os.File
		if outputPath == "/dev/stdout" || outputPath == "" {
			output = os.Stdout
		} else {
			var err error
			output, err = os.OpenFile(outputPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0640)
			if err != nil {
				return err
			}
			defer output.Close()
		}

		if err := printReport(output, resultStore); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}

	if result != nil {
		style := lipgloss.NewStyle().Bold(true)
		fmt.Fprintf(os.Stderr, "\n%s\n", style.Render("Error:"))
		fmt.Fprintln(os.Stderr, result.Error())

		fmt.Fprintf(os.Stderr, "\n%s\n", style.Render("Inputs:"))
		flagSet.PrintDefaults()

		if res, ok := result.(*cruntime.Result); ok {
			os.Exit(res.ExitCode)
		}

		os.Exit(1)
	}

	return nil
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

func envMap() map[string]string {
	envs := make(map[string]string)
	for _, v := range runArgs.envs {
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
	tuiModel         tui.UI
	tuiDone          chan struct{}
	tuiApp           *tea.Program
	prefixOutputDone chan struct{}
	prefixOutputCH   chan output.PrefixMessage
)

func uiOutput(cancel context.CancelFunc) tui.UI {
	if runArgs.output != "ui" {
		return nil
	}

	if tuiModel != nil {
		return tuiModel
	}

	tuiDone = make(chan struct{})
	tuiModel = tui.NewModel()
	tuiApp = tui.Program(tuiModel)

	go func() {
		_, err := tuiApp.Run()

		if err == nil {
			tuiApp.ReleaseTerminal()
		}

		cancel()
		tuiDone <- struct{}{}
	}()

	return tuiModel
}

func prefixOutput() chan output.PrefixMessage {
	if runArgs.output != "prefix" && runArgs.output != "prefix-nocolor" {
		return nil
	}

	prefixOutputDone = make(chan struct{})
	prefixOutputCH = make(chan output.PrefixMessage)

	go func() {
		output.TerminalWriter(prefixOutputCH)
		prefixOutputDone <- struct{}{}
	}()

	return prefixOutputCH
}

func outputFactory(cancel context.CancelFunc) (processor.OutputFactory, error) {
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
		outputHandler = output.UI(uiOutput(cancel))
	case renderOutputPrefix.String():
		outputHandler = output.Prefix(true, os.Stdout, os.Stderr, prefixOutput())
	case renderOutputPrefixNoColor.String():
		outputHandler = output.Prefix(false, os.Stdout, os.Stderr, prefixOutput())
	case renderOutputPassthrough.String():
		outputHandler = output.Passthrough(os.Stdout, os.Stderr)
	case renderOutputJSON.String():
		outputHandler = output.JSON(os.Stdout, os.Stderr)
	case renderOutputBuffer.String():
		if opts == "" {
			opts = renderOutputBufferDefaultTemplate
		}

		tmpl, err := template.New("output").Parse(opts)
		if err != nil {
			return nil, fmt.Errorf("failed to parse report buffer template: %w", err)
		}

		outputHandler = output.Buffer(tmpl, os.Stdout)
	default:
		return nil, fmt.Errorf("invalid output format given: %s", runArgs.output)
	}

	return outputHandler, nil
}

func printReport(w io.Writer, store *report.Store) error {
	switch runArgs.report {
	case reportTypeTable.String():
		report.Table(w, store.Ordered())
	case reportTypeJSON.String():
		report.JSON(w, store.Ordered())
	case reportTypeTimeline.String():
		return report.Timeline(w, store.Ordered())
	case reportTypeMarkdown.String():
		return report.Markdown(w, store.Ordered())
	case reportTypeNone.String():
		return nil
	default:
		return errors.New("unknown report type given")
	}

	return nil
}
