package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
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

func (r *table) Report(ctx processor.StepContext, name string) error {
	r.store.Add(name, ctx)
	return nil
}

func (r *table) Finalize() error {
	var rows [][]string
	rows = append(rows, []string{"#", "STEP", "STATUS", "DURATION", "TAGS", "ERROR"})
	for i, step := range r.store.Ordered() {
		errMsg, status, duration := stringify(step.result)

		var tags []string
		for _, tag := range step.result.Tags() {
			tags = append(tags, styles.TagLabel.Background(lipgloss.Color(tag.Color)).Render(fmt.Sprintf("%s: %s", tag.Key, tag.Value)))
		}

		rows = append(rows, []string{
			fmt.Sprintf("%d", i),
			step.stepName,
			status,
			duration,
			strings.Join(tags, ""),
			errMsg,
		})
	}

	var columnWidth []int
	for _, row := range rows {
		for key, cell := range row {
			if len(columnWidth) <= key {
				columnWidth = append(columnWidth, 0)
			}

			width := lipgloss.Width(cell)
			if width > columnWidth[key] {
				columnWidth[key] = width
			}
		}
	}

	for _, row := range rows {
		for key, cell := range row {
			width := lipgloss.Width(cell)
			if key < len(row)-1 && width < columnWidth[key] {
				row[key] = cell + strings.Repeat(" ", columnWidth[key]-width)
			}
		}
	}

	for _, row := range rows {
		fmt.Fprintf(r.w, "%s\n", strings.Join(row, " | "))
	}

	return nil
}
