package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Modal styling constants
const (
	ModalBorderColor = "6"
	ModalTitle       = "Bubble Tea Overlay"
	ModalContent     = "Hello! I'm in a modal window.\n\nPress <space> to close the window."
)

// Modal implements tea.Model and represents a modal dialog overlay
// It provides a simple information display with styling
type Modal struct {
	windowWidth  int
	windowHeight int
}

// Init initializes the Modal on program load
// It implements part of the tea.Model interface
func (m *Modal) Init() tea.Cmd {
	return nil
}

// Update handles events and manages internal state
// It implements part of the tea.Model interface
func (m *Modal) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.handleWindowResize(msg)
	}

	return m, cmd
}

// handleWindowResize updates the modal dimensions when the window is resized
func (m *Modal) handleWindowResize(msg tea.WindowSizeMsg) {
	m.windowWidth = msg.Width
	m.windowHeight = msg.Height
}

// View renders the modal with appropriate styling
// It implements part of the tea.Model interface
func (m *Modal) View() tea.View {
	modalStyle := m.createModalStyle()
	titleStyle := m.createTitleStyle()

	title := titleStyle.Render(ModalTitle)
	layout := lipgloss.JoinVertical(lipgloss.Left, title, ModalContent)

	return tea.NewView(modalStyle.Render(layout))
}

// createModalStyle creates the main modal styling
func (m *Modal) createModalStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder(), true).
		BorderForeground(lipgloss.Color(ModalBorderColor)).
		Padding(0, 1)
}

// createTitleStyle creates the title styling
func (m *Modal) createTitleStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true)
}
