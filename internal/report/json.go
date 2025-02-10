package report

import (
	"encoding/json"
	"fmt"
	"io"
)

func JSON(w io.Writer, steps []stepResult) error {
	b, err := json.MarshalIndent(steps, "", "  ")
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "\n%s", b)
	return nil
}
