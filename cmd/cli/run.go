package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	"github.com/alitto/pond/v2"
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
	"github.com/raffis/rageta/internal/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	"github.com/raffis/rageta/internal/storage"
	"github.com/raffis/rageta/internal/tui"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	transport "github.com/raffis/rageta/pkg/http/middleware"
	"github.com/spf13/cobra"
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
	deep                bool          `env:"DEEP"`
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

	runCmd.Flags().StringVarP(&runArgs.dbPath, "db-path", "", dbPath, "Path to the local rageta pipeline store.")
	runCmd.Flags().BoolVarP(&runArgs.tee, "tee", "", false, "Dump any internal redirected streams to stdout. Works similar as piping to tee on the console.")
	runCmd.Flags().StringVarP(&runArgs.output, "output", "o", electDefaultOutput(), "Output renderer. One of [prefix, prefix-nocolor, ui, json, buffer[=gotpl], raw]. The default `prefix` adds a colored task name prefix to the output lines while `ui` renders the tasks in a terminal ui. `none` dumps all tasks directly without any modification.")
	runCmd.Flags().BoolVarP(&runArgs.noGC, "no-gc", "", false, "Keep all containers and temporary files after execution. Useful for debugging purposes.")
	runCmd.Flags().BoolVarP(&runArgs.deep, "deep", "", false, "Add steps from inherited pipelines to report")
	runCmd.Flags().BoolVarP(&runArgs.skipDone, "skip-done", "", false, "Skip steps which have been successfully processed before. This is only useful in combination with a static context directory `--context-dir`.")
	runCmd.Flags().StringSliceVarP(&runArgs.skipSteps, "skip-steps", "", nil, "Do not executed these steps")
	runCmd.Flags().DurationVarP(&runArgs.gracefulTermination, "graceful-termination", "", time.Second*5, "Allow containers to exit gracefully.")
	runCmd.Flags().StringVarP(&runArgs.containerRuntime, "container-runtime", "", electDefaultDriver().String(), "Container runtime. One of [docker].")
	runCmd.Flags().StringSliceVarP(&runArgs.envs, "env", "e", nil, "Pass envs to the pipeline.")
	runCmd.Flags().StringSliceVarP(&runArgs.volumes, "volumes", "v", nil, "Pass volumes to the pipeline.")
	runCmd.Flags().StringArrayVarP(&runArgs.inputs, "input", "i", nil, "Pass inputs to the pipeline.")
	runCmd.Flags().StringVarP(&runArgs.report, "report", "r", "none", "Report summary of steps at the end of execution. One of [none, table, json, markdown].")
	runCmd.Flags().StringVarP(&runArgs.reportOutput, "report-output", "", electDefaultReportOutput(), "Destination for the report output.")
	runCmd.Flags().StringVarP(&runArgs.pull, "pull", "", pullImageMissing.String(), "Pull image before running. one of [always, missing, never].")
	runCmd.Flags().StringVarP(&runArgs.entrypoint, "entrypoint", "t", "", "Entrypoint for the given pipeline. The pipelines default is used otherwise.")
	runCmd.Flags().StringVarP(&runArgs.contextDir, "context-dir", "", "", "Use a static context directory. If any context is found it attempts to recover it.")
	runCmd.Flags().BoolVarP(&runArgs.dockerQuiet, "docker-quiet", "q", false, "Suppress the docker pull output.")
	runArgs.otelOptions.BindFlags(runCmd.Flags())
	runArgs.dockerOptions.BindFlags(runCmd.Flags())
	runArgs.ociOptions.BindFlags(runCmd.Flags())

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
	renderOutputRaw                   renderOutput = "raw"
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
		renderOutputBufferDefaultTemplate = `{{ printf "::group::%s\n%s\n::endgroup::\n" .StepName .Buffer }}`
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

func createContainerRuntime(ctx context.Context, d containerRuntime, logger logr.Logger, hideOutput bool, contextDir string) (runtime.Interface, error) {
	switch {
	case d == containerRuntimeDocker:
		c, err := runArgs.dockerOptions.Build()
		if err != nil {
			return nil, fmt.Errorf("failed to create docker client: %w", err)
		}

		c.HTTPClient().Transport = transport.NewLogger(logger, c.HTTPClient().Transport)

		runArgs.volumes = append(runArgs.volumes, fmt.Sprintf("%s:%s", contextDir, contextDir))

		driver := runtime.NewDocker(c,
			runtime.WithContext(ctx),
			runtime.WithHidePullOutput(hideOutput),
			runtime.WithVolumes(runArgs.volumes),
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
	envs map[string]string,
	celEnv *cel.Env,
	driver runtime.Interface,
	imagePullPolicy runtime.PullImagePolicy,
	tracer trace.Tracer, meter metric.Meter,
	outputFactory processor.OutputFactory,
	resultStore processor.ResultStore,
	teardown chan processor.Teardown,
	builder *processor.PipelineBuilder,
	store storage.Interface,
	pool pond.Pool,
) pipeline.StepBuilder {
	return func(spec v1beta1.Step, uniqueName string) []processor.Bootstraper {
		/*if ui := uiOutput(); ui != nil && spec.Run != nil {
			ui.AddTasks(tui.NewTask(uniqueName))
		}*/

		substitutableProcessors := processor.Builder(&spec,
			processor.WithMatrix(pool),
			processor.WithEnv(envs),
			//processor.WithStdioRedirect(),
			processor.WithRun(runArgs.tee, imagePullPolicy, driver, outputFactory, os.Stdin, os.Stdout, os.Stderr, teardown),
			processor.WithInherit(*builder, store),
		)

		processors := processor.Builder(&spec,
			processor.WithRecover(),
			processor.WithReport(resultStore, uniqueName),
			processor.WithRetry(),
			processor.WithStdio(runArgs.tee, outputFactory, os.Stdin, os.Stdout, os.Stderr),
			processor.WithOtelTrace(logger, tracer),
			processor.WithOtelLog(logger, &zapConfig),
			processor.WithOtelMetrics(meter),
			processor.WithSkipBlacklist(runArgs.skipSteps),
			processor.WithGarbageCollector(runArgs.noGC, driver, teardown),
			processor.WithAllowFailure(),
			processor.WithResult(),
			processor.WithOutput(),
			processor.WithTimeout(),
			processor.WithSkipDone(runArgs.skipDone),
			processor.WithIf(celEnv),
			processor.WithNeeds(),
			processor.WithTmpDir(),
			processor.WithSubstitute(celEnv, substitutableProcessors...),
		)

		processors = append(processors, substitutableProcessors...)

		operators := processor.Builder(&spec,
			processor.WithAnd(),
			processor.WithConcurrent(pool),
			processor.WithPipe(),
		)
		processors = append(processors, operators...)

		logger.V(1).Info("register step", "spec", spec)
		for _, processor := range processors {
			logger.V(1).Info("register step processor", "processor", fmt.Sprintf("%T", processor))
		}

		return processors
	}
}

func runRun(c *cobra.Command, args []string) error {
	logger, _, err := getRunLogger()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

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
		ext.Encoders(),
		ext.Sets(),
		cel.Variable("context", cel.MapType(cel.StringType, cel.DynType)),
	)

	if err != nil {
		return fmt.Errorf("setup cel env failed: %w", err)
	}

	imagePullPolicy, err := imagePullPolicy()
	if err != nil {
		return err
	}

	//logger := zap.New(zapbridge.NewOtelZapCore(otelName))
	tp, err := runArgs.otelOptions.BuildTracer(context.Background())
	if err != nil {
		return err
	}

	defer tp.Shutdown(context.Background())
	meter := otel.Meter(otelName)
	//logger2 := otelslog.NewLogger(name)
	//rollCnt metric.Int64Counter

	/*if runArgs.otelOptions.Endpoint != "" {
		tp, err := otelsetup.Tracing(context.Background(), runArgs.otelOptions)
		defer func() {
			if err := tp.Shutdown(context.Background()); err != nil {
				panic(err)
			}
		}()

		if err != nil {
			panic(err)
		}
	}*/

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

	var builder processor.PipelineBuilder
	builder = pipeline.NewBuilder(
		pipeline.WithStepBuilder(stepBuilder(logger, envMap(), celEnv, driver, imagePullPolicy, tp.Tracer(otelName), meter, outputFactory, resultStore, teardown, &builder, store, pool)),
		pipeline.WithLogger(logger),
		pipeline.WithTmpDir(contextDir),
	)

	go func() {
		sig := <-signals
		logger.Info("received signal", "signal", sig)
		cancel()
	}()

	inputs, err := parseInputs(command.Inputs, runArgs.inputs)
	if err != nil {
		return err
	}

	var result error
	command.PipelineSpec.Name = ""
	cmd, err := builder.Build(command, runArgs.entrypoint, inputs)
	if err != nil {
		result = err
	} else {
		_, result = cmd(ctx)
	}

	if tuiDone != nil {
		if result != nil {
			tuiInstance.SetStatus(tui.StepStatusFailed)
		} else {
			tuiInstance.SetStatus(tui.StepStatusDone)
		}

		if resultStore, ok := resultStore.(*report.Store); ok {
			buf := &bytes.Buffer{}

			if err := printReport(buf, resultStore); err != nil {
				fmt.Fprintln(buf, err)
			}

			tuiInstance.Report(buf.Bytes())
		}

		<-tuiDone
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

	if resultStore, ok := resultStore.(*report.Store); ok {
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
		fmt.Fprintln(os.Stderr, result.Error())

		if res, ok := result.(*runtime.Result); ok {
			os.Exit(res.ExitCode)
		}

		os.Exit(1)
	}

	return nil
}

func parseInputs(params []v1beta1.InputParam, inputs []string) (map[string]v1beta1.ParamValue, error) {
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
		result[v.Name] = v.Value

		if input, ok := steps[v.Name]; ok {
			x := result[v.Name]

			if len(input) == 1 {
				fmt.Printf("PARSE STR %s: %#v \n", v.Name, input)
				if err := x.UnmarshalJSON([]byte(input[0])); err != nil {
					return result, fmt.Errorf("failed to decode input: %w", err)
				}
				fmt.Printf("RES %s: %#v \n", v.Name, v.Value)

				result[v.Name] = x
				continue
			}
			fmt.Printf("PARSE Array %s: %#v \n", v.Name, input)

			x.Type = v1beta1.ParamTypeArray
			x.ArrayVal = input
			result[v.Name] = x

		}
	}
	fmt.Printf("============>  %#v \n", result)

	return result, nil
}

func imagePullPolicy() (runtime.PullImagePolicy, error) {
	switch runArgs.pull {
	case pullImageAlways.String():
		return runtime.PullImagePolicyAlways, nil
	case pullImageMissing.String():
		return runtime.PullImagePolicyMissing, nil
	case pullImageNever.String():
		return runtime.PullImagePolicyNever, nil
	default:
		return "", fmt.Errorf("invalid pull policy given: %s", runArgs.pull)
	}
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

var tuiInstance tui.UI
var tuiDone chan struct{}

func uiOutput(cancel context.CancelFunc) tui.UI {
	if runArgs.output != "ui" {
		return nil
	}

	if tuiInstance != nil {
		return tuiInstance
	}

	tuiDone = make(chan struct{})
	tuiInstance = tui.NewModel()

	/*if logger.GetV() > 0 {
		loggerTask := tui.NewTask("main()")
		tuiInstance.AddTasks(loggerTask)
	}*/

	//logger.WithSink(logr.)
	go func() {
		tui.Run(tuiInstance)
		cancel()
		tuiDone <- struct{}{}
	}()

	return tuiInstance
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
		outputHandler = output.Prefix(true)
	case renderOutputPrefixNoColor.String():
		outputHandler = output.Prefix(false)
	case renderOutputRaw.String():
		outputHandler = output.Raw()
	case renderOutputJSON.String():
		outputHandler = output.JSON()
	case renderOutputBuffer.String():
		if opts == "" {
			opts = renderOutputBufferDefaultTemplate
		}

		tmpl, err := template.New("output").Parse(opts)
		if err != nil {
			return nil, fmt.Errorf("failed to parse report buffer template: %w", err)
		}

		outputHandler = output.Buffer(tmpl)
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
