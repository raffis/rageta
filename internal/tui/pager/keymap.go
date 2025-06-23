package pager

import "github.com/charmbracelet/bubbles/key"

const spacebar = " "

// KeyMap defines the keybindings for the viewport
type KeyMap struct {
	PageDown       key.Binding
	PageUp         key.Binding
	HalfPageUp     key.Binding
	HalfPageDown   key.Binding
	Down           key.Binding
	Up             key.Binding
	Search         key.Binding
	NextMatch      key.Binding
	PrevMatch      key.Binding
	ExitSearchMode key.Binding
}

// DefaultKeyMap returns a set of pager-like default keybindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		PageDown: key.NewBinding(
			key.WithKeys("pgdown", spacebar),
			key.WithHelp("f/pgdn", "page down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("b/pgup", "page up"),
		),
		HalfPageUp: key.NewBinding(
			key.WithKeys("ctrl+u"),
			key.WithHelp("ctrl+u", "½ page up"),
		),
		HalfPageDown: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("ctrl+", "½ page down"),
		),
		Up: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down"),
			key.WithHelp("↓/j", "down"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		NextMatch: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "next match"),
		),
		PrevMatch: key.NewBinding(
			key.WithKeys("N"),
			key.WithHelp("N", "prev match"),
		),
		ExitSearchMode: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "exit search"),
		),
	}
}
