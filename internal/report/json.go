package report

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/raffis/rageta/internal/processor"
)

type jsonReport struct {
	store *store
	w     io.Writer
}

func JSON(w io.Writer) *jsonReport {
	return &jsonReport{
		w:     w,
		store: &store{},
	}
}

func (r *jsonReport) Report(ctx processor.StepContext, name string) error {
	r.store.Add(name, ctx)
	return nil
}

func (r *jsonReport) Finalize() error {
	b, err := json.MarshalIndent(r.store.Ordered(), "", "  ")
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(r.w, "\n%s", b)
	return err
}
