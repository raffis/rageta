package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	dockerclient "github.com/docker/docker/client"
	"github.com/docker/go-connections/sockets"
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
	dbPath              string            `env:"RAGETA_DB_PATH"`
	output              string            `env:"RAGETA_OUTPUT"`
	noGC                bool              `env:"RAGETA_NO_GC"`
	tee                 bool              `env:"RAGETA_TEE"`
	containerRuntime    string            `env:"RAGETA_CONTAINER_RUNTIME"`
	gracefulTermination time.Duration     `env:"RAGETA_GRACEFUL_TERMINATION"`
	quiet               bool              `env:"RAGETA_QUIT"`
	report              string            `env:"RAGETA_REPORT"`
	reportOutput        string            `env:"RAGETA_REPORT_OUTPUT"`
	pull                string            `env:"RAGETA_PULL"`
	entrypoint          string            `env:"RAGETA_ENTRYPOINT"`
	contextDir          string            `env:"RAGETA_CONTEXT_DIR"`
	inputs              map[string]string `env:"RAGETA_INPUTS"`
	skipDone            bool              `env:"RAGETA_SKIP_DONE"`
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

const otelName = "go.opentelemetry.io/otel/example/dice"

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
	runCmd.Flags().BoolVarP(&runArgs.skipDone, "skip-done", "", false, "Skip steps which have been successfully processed before. This is only useful in combination with a static context directory `--context-dir`.")
	runCmd.Flags().DurationVarP(&runArgs.gracefulTermination, "graceful-termination", "", time.Second*5, "Allow containers to exit gracefully.")
	runCmd.Flags().StringVarP(&runArgs.containerRuntime, "container-runtime", "", electDefaultDriver().String(), "Container runtime. One of [docker].")
	runCmd.Flags().StringToStringVarP(&runArgs.inputs, "input", "i", nil, "Pass inputs to the pipeline.")
	runCmd.Flags().StringVarP(&runArgs.report, "report", "r", "none", "Report summary of steps at the end of execution. One of [none, table, json, markdown].")
	runCmd.Flags().StringVarP(&runArgs.reportOutput, "report-output", "", electDefaultReportOutput(), "Destination for the report output.")
	runCmd.Flags().StringVarP(&runArgs.pull, "pull", "", pullImageMissing.String(), "Pull image before running. one of [always, missing, never].")
	runCmd.Flags().StringVarP(&runArgs.entrypoint, "entrypoint", "", "", "Entrypoint for the given pipeline. The pipelines default is used otherwise.")
	runCmd.Flags().StringVarP(&runArgs.contextDir, "context-dir", "", "", "Use a static context directory. If any context is found it attempts to recover it.")
	runCmd.Flags().BoolVarP(&runArgs.quiet, "quiet", "q", false, "Suppress the pull output.")
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
		renderOutputBufferDefaultTemplate = `{{- printf "\n::group::%s\n%s\n::endgroup::\n" .StepName .Buffer  -}}`
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

func defaultDockerHTTPClient(hostURL *url.URL) (*http.Client, error) {
	transport := &http.Transport{}
	err := sockets.ConfigureTransport(transport, hostURL.Scheme, hostURL.Host)
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Transport:     transport,
		CheckRedirect: dockerclient.CheckRedirect,
	}, nil
}

func createContainerRuntime(ctx context.Context, d containerRuntime, logger logr.Logger, output io.Writer) (runtime.Interface, error) {
	switch {
	case d == containerRuntimeDocker:
		hostURL, err := dockerclient.ParseHostURL(dockerclient.DefaultDockerHost)
		if err != nil {
			return nil, err
		}

		client, err := defaultDockerHTTPClient(hostURL)
		if err != nil {
			return nil, err
		}

		dockerClient, err := dockerclient.NewClientWithOpts(
			dockerclient.WithHTTPClient(client),
			dockerclient.FromEnv,
			dockerclient.WithAPIVersionNegotiation(),
		)

		client.Transport = transport.NewLogger(logger, client.Transport)

		if err != nil {
			return nil, fmt.Errorf("failed to create docker client: %w", err)
		}

		driver := runtime.NewDocker(dockerClient,
			runtime.WithContext(ctx),
			runtime.WithLogger(logger),
			runtime.WithPullImageWriter(output),
		)

		return driver, err
	}

	return nil, nil
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

func stepBuilder(logger logr.Logger, celEnv *cel.Env, driver runtime.Interface, imagePullPolicy runtime.PullImagePolicy, tracer trace.Tracer, meter metric.Meter, outputFactory processor.OutputFactory, resultStore processor.ResultStore, teardown chan processor.Teardown, builder *processor.PipelineBuilder, store storage.Interface) pipeline.StepBuilder {
	return func(spec v1beta1.Step, uniqueName string) []processor.Bootstraper {
		/*if ui := uiOutput(); ui != nil && spec.Run != nil {
			ui.AddTasks(tui.NewTask(uniqueName))
		}*/

		substitutableProcessors := processor.Builder(&spec,
			processor.WithMatrix(),
			processor.WithStdio(),
			processor.WithRun(runArgs.tee, imagePullPolicy, driver, outputFactory, os.Stdin, os.Stdout, os.Stderr),
			processor.WithInherit(*builder, store),
		)

		processors := processor.Builder(&spec,
			processor.WithReport(resultStore, uniqueName),
			processor.WithGarbageCollector(runArgs.noGC, driver, teardown),
			processor.WithOtel(logger, tracer, meter),
			processor.WithRetry(),
			processor.WithAllowFailure(),
			processor.WithEventEmitter(),
			processor.WithTimeout(),
			processor.WithSkipDone(runArgs.skipDone),
			processor.WithIf(celEnv),
			processor.WithNeeds(),
			processor.WithExpressionParser(celEnv, substitutableProcessors...),
		)

		processors = append(processors, substitutableProcessors...)

		operators := processor.Builder(&spec,
			processor.WithAnd(),
			processor.WithConcurrent(),
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
	logger, logFile, err := getRunLogger()
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
	//wire := protobuf.NewSerializer(nil, kruntime.MultiObjectTyper{})

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

	var pullOutput io.Writer = logFile
	if runArgs.quiet {
		pullOutput = io.Discard
	}

	driver, err := createContainerRuntime(ctx, containerRuntime(runArgs.containerRuntime), logger, pullOutput)
	if err != nil {
		return err
	}

	celEnv, err := cel.NewEnv(
		ext.Strings(),
		ext.Math(),
		ext.Encoders(),
		ext.Sets(),
		cel.Types(&v1beta1.RuntimeVars{}),
		cel.Variable("context", cel.ObjectType("rageta.core.v1beta1.RuntimeVars")),
	)

	if err != nil {
		return err
	}

	imagePullPolicy, err := imagePullPolicy()
	if err != nil {
		return err
	}

	//logger := zap.New(zapbridge.NewOtelZapCore(otelName))
	tp, err := otelsetup.Tracing(context.Background(), runArgs.otelOptions)
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

	outputFactory, err := outputFactory()
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

	var engine processor.PipelineBuilder
	engine = pipeline.NewEngine(
		driver,
		pipeline.WithStepBuilder(stepBuilder(logger, celEnv, driver, imagePullPolicy, tp.Tracer(otelName), meter, outputFactory, resultStore, teardown, &engine, store)),
		pipeline.WithLogger(logger),
		pipeline.WithTmpDir(contextDir),
	)

	go func() {
		sig := <-signals
		logger.Info("received signal", "signal", sig)
		cancel()
	}()

	var result error
	command.PipelineSpec.Name = ""
	cmd, err := engine.Build(command, runArgs.entrypoint, runArgs.inputs)
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
		if err := printReport(resultStore); err != nil {
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

var tuiInstance tui.UI
var tuiDone chan struct{}

func uiOutput() tui.UI {
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
		tuiDone <- struct{}{}
	}()

	return tuiInstance
}

func outputFactory() (processor.OutputFactory, error) {
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
		outputHandler = output.UI(uiOutput())
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

func printReport(store *report.Store) error {
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

	switch runArgs.report {
	case reportTypeTable.String():
		report.Table(output, store)
	case reportTypeJSON.String():
		report.JSON(output, store)
	case reportTypeMarkdown.String():
		return report.Markdown(output, store)
	case reportTypeNone.String():
		return nil
	default:
		return errors.New("unknown report type given")
	}

	return nil
}
