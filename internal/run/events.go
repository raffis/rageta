package run

import (
	"io"
	"os"
	"time"

	"github.com/spf13/pflag"
)

type EventsOptions struct {
	EventsOutput       string
	Output             string
	Disabled           bool
	WaitUpdateInterval time.Duration
}

func (s *EventsOptions) BindFlags(flags *pflag.FlagSet) {
	flags.StringVarP(&s.EventsOutput, "events-output", "", "", "Destination for the events. By default this depends on the output (-o) set.")
	flags.BoolVarP(&s.Disabled, "no-events", "", s.Disabled, "Do not emits event messages")
	flags.DurationVarP(&s.WaitUpdateInterval, "wait-update-interval", "", s.WaitUpdateInterval, "Print waiting for task status updates every n interval")

}

func (s EventsOptions) Build() Step {
	return &Events{opts: s}
}

func NewEventsOptions() EventsOptions {
	return EventsOptions{
		WaitUpdateInterval: 5 * time.Second,
		EventsOutput:       "/dev/stdout",
	}
}

type Events struct {
	opts EventsOptions
}

type EventsContext struct {
	Dev                io.Writer
	Enabled            bool
	WaitUpdateInterval time.Duration
}

func (s *Events) Run(rc *RunContext, next Next) error {
	switch {
	case s.opts.EventsOutput == "/dev/stdout" || s.opts.EventsOutput == "-":
		rc.Events.Dev = rc.Output.Stdout
	case s.opts.EventsOutput == "/dev/stderr":
		rc.Events.Dev = rc.Output.Stderr
	case s.opts.EventsOutput != "":
		f, err := os.OpenFile(s.opts.EventsOutput, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0640)
		if err != nil {
			return err
		}

		defer func() {
			_ = f.Close()
		}()

		rc.Events.Dev = f
	default:
		return next(rc)
	}

	rc.Events.Enabled = !s.opts.Disabled
	rc.Events.WaitUpdateInterval = s.opts.WaitUpdateInterval

	return next(rc)
}
