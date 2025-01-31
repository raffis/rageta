package report

import (
	"encoding/json"
	"fmt"
	"io"
)

func JSON(w io.Writer, store *Store) error {
	b, err := json.MarshalIndent(store.steps, "", "  ")
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "\n%s", b)
	return nil
}
