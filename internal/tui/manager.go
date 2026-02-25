package tui

import (
	tea "charm.land/bubbletea/v2"
)

// Session state constants
type sessionState int

const (
	// mainView represents the primary UI view
	mainView sessionState = iota
	// modalView represents an overlay modal view
	modalView
)

// Keyboard shortcuts for manager
const (
	KeyQuitManager   = "q"
	KeyEscapeManager = "esc"
	KeySpaceManager  = "space"
)

// Manager implements tea.Model and manages the overall UI state including modals
// It coordinates between the main UI and any overlay components
type Manager struct {
	state        sessionState
	windowWidth  int
	windowHeight int
	foreground   tea.Model
	background   tea.Model
}

// NewManager creates a new Manager instance with the given background UI model
func NewManager(ui tea.Model) *Manager {
	return &Manager{
		background: ui,
	}
}

// Init initializes the Manager on program load
// It implements part of the tea.Model interface
func (m *Manager) Init() tea.Cmd {
	m.state = mainView
	m.foreground = &Modal{}
	return nil
}

// Update handles events and manages internal state
// It implements part of the tea.Model interface
func (m *Manager) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.handleWindowResize(msg)
	case tea.KeyPressMsg:
		if cmd := m.handleKeyMessage(msg); cmd != nil {
			return m, cmd
		}
	}

	return m.updateComponents(message)
}

// handleWindowResize handles window resize events
func (m *Manager) handleWindowResize(msg tea.WindowSizeMsg) {
	m.windowWidth = msg.Width
	m.windowHeight = msg.Height
}

// handleKeyMessage handles keyboard input for manager-level actions
func (m *Manager) handleKeyMessage(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case KeyQuitManager, KeyEscapeManager:
		return tea.Quit
	case KeySpaceManager:
		m.toggleView()
		return nil
	}
	return nil
}

// toggleView switches between main and modal views
func (m *Manager) toggleView() {
	if m.state == mainView {
		m.state = modalView
	} else {
		m.state = mainView
	}
}

// updateComponents updates both foreground and background components
func (m *Manager) updateComponents(message tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Update foreground component
	if fg, fgCmd := m.foreground.Update(message); fg != nil {
		m.foreground = fg
		cmds = append(cmds, fgCmd)
	}

	// Update background component
	if bg, bgCmd := m.background.Update(message); bg != nil {
		m.background = bg
		cmds = append(cmds, bgCmd)
	}

	return m, tea.Batch(cmds...)
}

// View renders the appropriate view based on current state
// It implements part of the tea.Model interface
func (m *Manager) View() tea.View {
	if m.state == modalView {
		return m.foreground.View()
	}
	return m.background.View()
}
