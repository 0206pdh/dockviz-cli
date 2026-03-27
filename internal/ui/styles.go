// Package ui defines all Lip Gloss visual styles used across the TUI.
// Centralizing styles here makes it easy to change the color scheme in one place.
package ui

import "github.com/charmbracelet/lipgloss"

// Color palette
var (
	ColorGreen  = lipgloss.Color("#00C896")
	ColorRed    = lipgloss.Color("#FF4F64")
	ColorYellow = lipgloss.Color("#FFD700")
	ColorBlue   = lipgloss.Color("#4DA6FF")
	ColorGray   = lipgloss.Color("#6C7A89")
	ColorWhite  = lipgloss.Color("#EAEAEA")
	ColorBg     = lipgloss.Color("#1E1E2E") // dark background
)

// Base styles
var (
	// Title bar at the top of the screen
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorBlue).
			Padding(0, 1)

	// Panel border box
	PanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorGray).
			Padding(0, 1)

	// Active (focused) panel border
	ActivePanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorBlue).
				Padding(0, 1)

	// Table header row
	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorBlue).
			Underline(true)

	// Selected row in a list
	SelectedRowStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#2A2A4A")).
				Foreground(ColorWhite)

	// Status badges
	StatusRunning = lipgloss.NewStyle().Foreground(ColorGreen).Bold(true)
	StatusStopped = lipgloss.NewStyle().Foreground(ColorRed)
	StatusPaused  = lipgloss.NewStyle().Foreground(ColorYellow)

	// CPU/MEM values
	StatStyle = lipgloss.NewStyle().Foreground(ColorWhite)

	// Footer / keybinding hint bar
	FooterStyle = lipgloss.NewStyle().
			Foreground(ColorGray).
			Padding(0, 1)

	// Error message
	ErrorStyle = lipgloss.NewStyle().
			Foreground(ColorRed).
			Bold(true)
)

// StatusStyle returns the appropriate badge style based on container state.
func StatusStyle(state string) lipgloss.Style {
	switch state {
	case "running":
		return StatusRunning
	case "paused":
		return StatusPaused
	default:
		return StatusStopped
	}
}

// StatusIcon returns a unicode icon for a container state.
func StatusIcon(state string) string {
	switch state {
	case "running":
		return "●"
	case "paused":
		return "◑"
	default:
		return "○"
	}
}
