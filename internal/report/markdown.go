package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/raffis/rageta/internal/processor"
)

type markdown struct {
	store *store
	w     io.Writer
}

func Markdown(w io.Writer) *markdown {
	return &markdown{
		w:     w,
		store: &store{},
	}
}

func (r *markdown) Report(ctx processor.StepContext, name string) error {
	r.store.Add(name, ctx)
	return nil
}

func (r *markdown) Finalize() error {
	fmt.Fprintln(r.w, "| # | Step | Status | Duration | Tags | Error |")
	fmt.Fprintln(r.w, "| --- | --- | --- | --- | --- |")

	for i, step := range r.store.Ordered() {
		var tags []string
		for _, tag := range step.result.Tags() {
			tags = append(tags, fmt.Sprintf("<span style=\"color:%s\">%s: %s</span>", tag.Color, tag.Key, tag.Value))
		}

		errMsg, status, duration := stringify(step.result)
		fmt.Fprintf(r.w, "| %d | %s | %s | %s | %s | %s |\n",
			i,
			step.stepName,
			status,
			duration,
			strings.Join(tags, " "),
			errMsg,
		)
	}

	return nil
}
