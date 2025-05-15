package report

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/olekukonko/tablewriter"
	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/internal/styles"
)

type table struct {
	store *store
	w     io.Writer
}

func Table(w io.Writer) *table {
	return &table{
		w:     w,
		store: &store{},
	}
}

func (r *table) Report(ctx context.Context, name string, stepContext processor.StepContext) error {
	r.store.Add(name, stepContext)
	return nil
}

func (r *table) Finalize() error {
	table := tablewriter.NewWriter(r.w)
	table.SetHeader([]string{"#", "Step", "Status", "Duration", "Tags", "Error"})
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetBorder(false)
	table.SetAutoWrapText(false)
	table.SetCenterSeparator("")
	table.SetHeaderLine(false)
	table.SetReflowDuringAutoWrap(false)

	for i, step := range r.store.Ordered() {
		errMsg, status, duration := stringify(step.result)

		var tags []string
		for _, tag := range step.result.Tags {
			tags = append(tags, styles.TagLabel.Background(lipgloss.Color(tag.Color)).Render(fmt.Sprintf("%s: %s", tag.Key, tag.Value)))
		}

		table.Append([]string{
			fmt.Sprintf("%d", i),
			step.stepName,
			status,
			duration,
			strings.Join(tags, ""),
			errMsg})
	}

	table.Render()
	return nil
}
