// start.go is the entry point into the TUI subsystem.
// It wires together the Docker client and the Bubble Tea program.
package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yourusername/dockviz-cli/internal/docker"
)

// Start connects to Docker, builds the initial model, and runs the TUI event loop.
func Start() error {
	dc, err := docker.NewClient()
	if err != nil {
		return fmt.Errorf("docker: %w", err)
	}
	defer dc.Close()

	m := newModel(dc)

	// Init() on the model handles the first fetch and tick automatically.
	p := tea.NewProgram(m,
		tea.WithAltScreen(),       // use alternate screen buffer (no scroll pollution)
		tea.WithMouseCellMotion(), // optional: enable mouse support
	)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}
