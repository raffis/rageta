package run

import (
	"strings"

	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/setup/flagset"
)

type LabelsOptions struct {
	Labels []string
}

func (s *LabelsOptions) BindFlags(flags flagset.Interface) {
	flags.StringSliceVarP(&s.Labels, "label", "", s.Labels, "Add global labels to pipeline tasks. Format is `key=value(:#color). Example: `--labels key=value(:#FF0000)`")
}

func (s LabelsOptions) Build() Task {
	return &Labels{opts: s}
}

type Labels struct {
	opts LabelsOptions
}

type LabelsContext struct {
	Labels []processor.Label
}

func (s *Labels) Label() string {
	return "Applying labels"
}

func (s *Labels) Run(rc *RunContext, next Next) error {
	rc.Labels.Labels = s.parseLabels(s.opts.Labels)
	return next(rc)
}

func (s *Labels) parseLabels(labels []string) []processor.Label {
	var result []processor.Label
	for _, label := range labels {
		v := strings.SplitN(label, "=", 2)
		if len(v) != 2 {
			continue
		}
		t := processor.Label{Key: v[0]}
		value := strings.SplitN(v[1], ":", 2)
		if len(value) == 2 {
			t.Value = value[0]
			t.HEXColor = value[1]
		} else {
			t.Value = v[1]
		}
		result = append(result, t)
	}
	return result
}
