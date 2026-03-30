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

	// Selected row in a list — bright enough to see on any dark terminal
	SelectedRowStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#4DA6FF")).
				Foreground(lipgloss.Color("#000000")).
				Bold(true)

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

// Sparkline converts a slice of float64 CPU percentage values (0-100) into a
// unicode bar string using the block element characters ▁▂▃▄▅▆▇█.
// The result is always 10 runes wide, left-padded with spaces when fewer than
// 10 values are provided.
//
// Values are mapped against a fixed 0-100% scale so bars reflect actual
// CPU utilisation — a container at 5% always looks nearly empty, and one at
// 90% always looks nearly full, regardless of other containers' activity.
func Sparkline(values []float64) string {
	if len(values) == 0 {
		return "          " // 10 spaces — placeholder before any data arrives
	}
	bars := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

	result := make([]rune, len(values))
	for i, v := range values {
		if v < 0 {
			v = 0
		}
		if v > 100 {
			v = 100
		}
		idx := int((v / 100.0) * float64(len(bars)-1))
		if idx >= len(bars) {
			idx = len(bars) - 1
		}
		result[i] = bars[idx]
	}

	// Left-pad with spaces to fill the fixed 10-character column width.
	s := string(result)
	for len([]rune(s)) < 10 {
		s = " " + s
	}
	return s
}
