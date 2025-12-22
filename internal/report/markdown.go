package report

import (
	"fmt"
	"io"
	"strings"
	"time"

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
	fmt.Fprintln(r.w, "| --- | --- | --- | --- | --- | --- |")

	for i, step := range r.store.Ordered() {
		var tags []string
		for _, tag := range step.result.Tags() {
			tags = append(tags, fmt.Sprintf("`%s: %s`", tag.Key, tag.Value))
		}

		errMsg, status, duration := r.stringify(step.result)
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

func (r *markdown) stringify(step processor.StepContext) (string, string, string) {
	var (
		duration time.Duration
		status   string
		errMsg   string
	)

	switch {
	case step.StartedAt.IsZero():
		status = `🕙`
	case step.Error != nil && !processor.AbortOnError(step.Error):
		status = `⚠️`
		errMsg = strings.ReplaceAll(step.Error.Error(), "\n", "")
	case step.Error != nil:
		status = `⛔`
		errMsg = strings.ReplaceAll(step.Error.Error(), "\n", "")
	case step.Error == nil:
		status = `✅`
	}

	if !step.EndedAt.IsZero() {
		duration = step.EndedAt.Sub(step.StartedAt).Round(time.Millisecond * 10)
	}

	return errMsg, status, duration.String()
}
