package run

import (
	"strings"

	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/setup/flagset"
)

type TagsOptions struct {
	Tags []string
}

func (s *TagsOptions) BindFlags(flags flagset.Interface) {
	flags.StringSliceVarP(&s.Tags, "tags", "", s.Tags, "Add global custom tags to pipeline steps. Format is `key=value(:#color). Example: `--tags domain=example.com:#FF0000`")
}

func (s TagsOptions) Build() Step {
	return &Tags{opts: s}
}

type Tags struct {
	opts TagsOptions
}

type TagsContext struct {
	Tags []processor.Tag
}

func (s *Tags) Run(rc *RunContext, next Next) error {
	rc.Tags.Tags = s.parseTags(s.opts.Tags)
	return next(rc)
}

func (s *Tags) parseTags(tags []string) []processor.Tag {
	var result []processor.Tag
	for _, tag := range tags {
		v := strings.SplitN(tag, "=", 2)
		if len(v) != 2 {
			continue
		}
		t := processor.Tag{Key: v[0]}
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
