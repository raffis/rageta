package run

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/report"
	"github.com/spf13/pflag"
)

type ReportType string

var (
	ReportTypeTable    ReportType = "table"
	ReportTypeMarkdown ReportType = "markdown"
	ReportTypeJSON     ReportType = "json"
)

func (d ReportType) String() string {
	return string(d)
}

type ReportOptions struct {
	ReportType   string
	ReportOutput string
}

func (s *ReportOptions) BindFlags(flags *pflag.FlagSet) {
	flags.StringVarP(&s.ReportType, "report", "r", s.ReportType, "Report summary of steps at the end of execution. One of [table, json, markdown].")
	flags.StringVarP(&s.ReportOutput, "report-output", "", s.ReportOutput, "Destination for the report output.")
}

func (s ReportOptions) Build() Step {
	return &Report{opts: s}
}

func NewReportOptions() ReportOptions {
	return ReportOptions{
		ReportOutput: "/dev/stdout",
	}
}

type Report struct {
	opts ReportOptions
}

type ReportContext struct {
	Factory reportFinalizer
}

func (s *Report) Run(rc *RunContext, next Next) error {
	if s.opts.ReportType == "" {
		return next(rc)
	}

	reportOutput := s.opts.ReportOutput
	var reportDev io.Writer

	if reportOutput == "/dev/stdout" || reportOutput == "" {
		reportDev = rc.Output.Stdout
	} else {
		output, err := os.OpenFile(reportOutput, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0640)
		if err != nil {
			return err
		}

		defer func() {
			_ = output.Close()
		}()
		reportDev = rc.Secrets.Store.Writer(output)
	}

	reportFactory, err := s.buildReportFactory(reportDev)
	if err != nil {
		return err
	}

	rc.Report.Factory = reportFactory
	err = next(rc)

	if reportFactory != nil && (errors.Is(err, PipelineExecutionError) || err == nil) {
		if reportErr := reportFactory.Finalize(); reportErr != nil {
			err = errors.Join(reportErr, err)
		}
	}
	return err
}

func (s *Report) buildReportFactory(w io.Writer) (reportFinalizer, error) {
	if w == nil {
		return nil, nil
	}
	switch s.opts.ReportType {
	case ReportTypeTable.String():
		return report.Table(w), nil
	case ReportTypeMarkdown.String():
		return report.Markdown(w), nil
	case ReportTypeJSON.String():
		return report.JSON(w), nil
	default:
		return nil, fmt.Errorf("invalid report type given: %s", s.opts.ReportType)
	}
}

type reportFinalizer interface {
	processor.Reporter
	Finalize() error
}
