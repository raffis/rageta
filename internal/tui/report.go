package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/raffis/rageta/internal/tui/pager"
)

type report struct {
	viewport *pager.Model
}

func (t *report) Write(b []byte) (int, error) {
	return t.viewport.Write(b)
}

func (t *report) Update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	return tea.Batch(cmds...)
}

func (t *report) Init() tea.Cmd {
	return nil
}

func (t *report) getViewport() *pager.Model {
	return t.viewport
}

func (t *report) Matrix() string {
	return ""
}

func (t *report) GetName() string {
	return lipgloss.NewStyle().Foreground(highlightColor).Bold(true).Render("Report")
}

func (t *report) Title() string {
	return zone.Mark(t.GetName(), t.GetName())
}

func (t *report) Description() string {
	return lipgloss.NewStyle().Foreground(highlightColor).Render("───")
}

func (t *report) FilterValue() string {
	return t.GetName()
}
