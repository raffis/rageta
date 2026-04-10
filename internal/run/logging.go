package run

import (
	"fmt"
	"io"
	"os"
	"path"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/setup/flagset"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type LoggingOptions struct {
	ZapConfig zap.Config
	Detached  bool
}

func (s LoggingOptions) Build() Step {
	return &Logging{
		opts: s,
	}
}

func (s *LoggingOptions) BindFlags(flags flagset.Interface) {
	flags.BoolVarP(&s.Detached, "log-detached", "", s.Detached, "Detach logs.")
}

func NewLoggingOptions() LoggingOptions {
	return LoggingOptions{
		ZapConfig: zap.NewDevelopmentConfig(),
	}
}

type Logging struct {
	opts LoggingOptions
}

type LoggingContext struct {
	Logger   logr.Logger
	Builder  processor.LogBuilder
	Detached bool
	Debug    bool
	MainLog  zapcore.Core
}

func (s *Logging) Run(rc *RunContext, next Next) error {
	logFile, err := os.OpenFile(path.Join(rc.ContextDir.Path, "main.log"), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0640)
	if err != nil {
		return err
	}

	maskedLog := rc.Secrets.Store.Writer(logFile)
	logCoreFile, err := s.buildZapCore(s.opts.ZapConfig, maskedLog)
	if err != nil {
		return err
	}

	defaultLog := logCoreFile
	if rc.Logging.MainLog != nil {
		defaultLog = zapcore.NewTee(logCoreFile, rc.Logging.MainLog)
	}

	logBuilder := s.logBuilder(defaultLog, s.opts.ZapConfig)
	rc.Logging.Logger = zapr.NewLogger(zap.New(defaultLog))
	rc.Logging.Detached = s.opts.Detached
	rc.Logging.Builder = logBuilder
	rc.Logging.Debug = s.opts.ZapConfig.Level.Level() <= -5
	return next(rc)
}

func (s *Logging) logBuilder(defaultLog zapcore.Core, zapConfig zap.Config) processor.LogBuilder {
	return func(w io.Writer) (logr.Logger, error) {
		log, err := s.buildZapCore(zapConfig, w)
		if err != nil {
			return logr.Discard(), err
		}

		zapLogger := zap.New(zapcore.NewTee(defaultLog, log))
		return zapr.NewLogger(zapLogger), nil
	}
}

func (s *Logging) buildZapCore(config zap.Config, w io.Writer) (zapcore.Core, error) {
	var encoder zapcore.Encoder
	switch config.Encoding {
	case "json":
		encoder = zapcore.NewJSONEncoder(config.EncoderConfig)
	case "console":
		encoder = zapcore.NewConsoleEncoder(config.EncoderConfig)
	default:
		return nil, fmt.Errorf("no such log encoder `%s`", config.Encoding)
	}

	return zapcore.NewCore(encoder, zapcore.AddSync(w), config.Level), nil
}
