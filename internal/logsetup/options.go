package logsetup

import (
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/spf13/pflag"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Options struct {
	Timeout time.Duration
	Verbose int8
	Log     struct {
		Encoding string
	}
}

func DefaultOptions() *Options {
	var level int8

	if os.Getenv("RUNNER_DEBUG") != "" {
		level = 10
	}

	return &Options{
		Verbose: level,
	}
}

func (o *Options) BindFlags(fs *pflag.FlagSet) {
	fs.Int8VarP(&o.Verbose, "verbose", "v", 0, "Log verbosity level. With `0` no logs visible while 128 is the most verbose level.")
	fs.StringVar(&o.Log.Encoding, "log-encoding", "json", "Log encoding format")
}

func (o *Options) Build() (logr.Logger, zap.Config, error) {
	zapConfig := zap.NewDevelopmentConfig()
	zapConfig.Encoding = o.Log.Encoding
	zapConfig.Level = zap.NewAtomicLevelAt(zapcore.Level(-1 * o.Verbose))

	zapConfig.EncoderConfig.EncodeLevel = func(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendInt(int(l) * -1)
	}

	zapConfig.DisableStacktrace = false
	zapLog, err := zapConfig.Build()
	if err != nil {
		return logr.Discard(), zapConfig, err
	}

	return zapr.NewLogger(zapLog), zapConfig, nil
}
