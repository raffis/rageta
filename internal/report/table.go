package report

import (
	"fmt"
	"io"

	"github.com/olekukonko/tablewriter"
)

func Table(w io.Writer, store *Store) error {
	table := tablewriter.NewWriter(w)
	table.SetHeader([]string{"#", "Step", "Status", "Duration", "Error"})
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetBorder(false)
	table.SetAutoWrapText(false)

	for i, step := range store.steps {
		errMsg, status, duration := stringify(step)

		table.Append([]string{
			fmt.Sprintf("%d", i),
			step.stepName,
			status,
			duration,
			errMsg})
	}

	table.Render()
	return nil
}
