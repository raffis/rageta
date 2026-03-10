package runner

import (
	"fmt"
	"io"

	"github.com/raffis/rageta/internal/report"
)

const (
	reportTypeNone     = "none"
	reportTypeTable    = "table"
	reportTypeMarkdown = "markdown"
)

type ReportStep struct {
	reportType string
}

func WithReport(reportType string) *ReportStep {
	return &ReportStep{reportType: reportType}
}

func (s *ReportStep) Run(rc *RunContext, next Next) error {
	reportFactory, err := s.buildReportFactory(rc.ReportDev)
	if err != nil {
		return err
	}
	rc.ReportFactory = reportFactory
	return next(rc)
}

func (s *ReportStep) buildReportFactory(w io.Writer) (reportFinalizer, error) {
	if w == nil {
		return nil, nil
	}
	switch s.reportType {
	case reportTypeNone:
		return nil, nil
	case reportTypeTable:
		return report.Table(w), nil
	case reportTypeMarkdown:
		return report.Markdown(w), nil
	default:
		return nil, fmt.Errorf("invalid report type given: %s", s.reportType)
	}
}
