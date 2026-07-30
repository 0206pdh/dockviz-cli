// start.go is the entry point into the TUI subsystem.
// It wires together the Docker client and the Bubble Tea program.
package tui

import (
	"context"
	"fmt"

	composectx "github.com/0206pdh/dockviz-cli/internal/compose"
	"github.com/0206pdh/dockviz-cli/internal/docker"
	tea "github.com/charmbracelet/bubbletea"
)

// StartOptions configures the TUI entry point.
type StartOptions struct {
	Demo         bool
	Host         string
	Version      string
	ComposeFiles []string
}

// Start connects to Docker (or uses demo data), builds the model, and runs the TUI.
// Demo mode runs without a live Docker daemon. Host overrides DOCKER_HOST when
// non-empty (e.g. "tcp://192.168.1.100:2375").
func Start(opts StartOptions) error {
	var dc docker.DockerClient
	if opts.Demo {
		dc = docker.NewDemoClient()
	} else {
		real, err := docker.NewClient(opts.Host)
		if err != nil {
			return fmt.Errorf("docker: %w", err)
		}
		dc = real
	}
	defer dc.Close()

	composeContext, err := composectx.Load(context.Background(), "", opts.ComposeFiles)
	if err != nil {
		return fmt.Errorf("compose: %w", err)
	}

	m := newModel(dc, opts.Version, opts.Demo, composeContext)

	// Init() on the model handles the first fetch and tick automatically.
	p := tea.NewProgram(m,
		tea.WithAltScreen(), // use alternate screen buffer (no scroll pollution)
	)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}
