package runner

import (
	"fmt"
	"io"
	"os"

	"github.com/raffis/rageta/internal/mask"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type LoggingStep struct {
	zapConfig zap.Config
}

func WithLogging(zapConfig zap.Config) Step {
	return &LoggingStep{zapConfig: zapConfig}
}

func (s *LoggingStep) Run(rc *RunContext, next Next) error {
	logFile, err := os.CreateTemp(os.TempDir(), "rageta-log")
	if err != nil {
		return err
	}

	rc.SecretStore = mask.NewSecretStore(mask.DefaultMask)
	maskedLog := rc.SecretStore.Writer(logFile)

	if rc.Stdout == nil {
		rc.Stdout = rc.SecretStore.Writer(os.Stdout)
	}
	if rc.Stderr == nil {
		if IsTerm() {
			rc.Stderr = rc.Stdout
		} else {
			rc.Stderr = rc.SecretStore.Writer(os.Stderr)
		}
	}

	logCoreFile, err := s.buildZapCore(maskedLog)
	if err != nil {
		return err
	}

	rc.LogCoreFile = logCoreFile
	_ = logFile
	return next(rc)
}

func (s *LoggingStep) buildZapCore(w io.Writer) (zapcore.Core, error) {
	config := s.zapConfig
	var encoder zapcore.Encoder
	switch config.Encoding {
	case "json":
		encoder = zapcore.NewJSONEncoder(config.EncoderConfig)
	case "console":
		encoder = zapcore.NewConsoleEncoder(config.EncoderConfig)
	default:
		return nil, fmt.Errorf("failed setup step logger: no such log encoder `%s`", config.Encoding)
	}
	return zapcore.NewCore(encoder, zapcore.AddSync(w), config.Level), nil
}
