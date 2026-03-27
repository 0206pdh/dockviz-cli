// update.go implements the Bubble Tea Update function.
// It handles all incoming messages (key presses, ticks, fetched data) and
// returns a new Model + optional Command to run next.
package tui

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/yourusername/dockviz-cli/internal/docker"
)

// Update is called by Bubble Tea whenever a message arrives.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	// Terminal was resized
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	// Spinner animation tick
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	// Auto-refresh timer fired
	case tickMsg:
		return m, tea.Batch(fetchDataCmd(m.docker), tickCmd())

	// Fresh Docker data arrived
	case dataMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.containers = msg.containers
		m.networks = msg.networks
		m.images = msg.images
		// Clamp cursor to list length
		m.cursor = clamp(m.cursor, 0, m.activeListLen()-1)
		return m, nil

	// Keyboard input
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

// handleKey dispatches key presses based on the current view.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	km := m.keys

	switch {
	case keyMatches(msg, km.Quit):
		return m, tea.Quit

	case keyMatches(msg, km.Up):
		if m.cursor > 0 {
			m.cursor--
		}

	case keyMatches(msg, km.Down):
		if m.cursor < m.activeListLen()-1 {
			m.cursor++
		}

	case keyMatches(msg, km.Tab):
		m.activePanel = (m.activePanel + 1) % 3
		m.cursor = 0

	case keyMatches(msg, km.Refresh):
		m.loading = true
		return m, fetchDataCmd(m.docker)

	case keyMatches(msg, km.Back):
		m.activeView = ViewDashboard

	case keyMatches(msg, km.Enter):
		if m.activePanel == PanelContainers && len(m.containers) > 0 {
			m.selectedID = m.containers[m.cursor].ID
			m.activeView = ViewDetail
		}

	case keyMatches(msg, km.Toggle):
		if m.activePanel == PanelContainers && len(m.containers) > 0 {
			ctr := m.containers[m.cursor]
			return m, toggleContainerCmd(m.docker, ctr.ID, ctr.Status)
		}

	case keyMatches(msg, km.Logs):
		if m.activePanel == PanelContainers && len(m.containers) > 0 {
			m.selectedID = m.containers[m.cursor].ID
			m.activeView = ViewLogs
		}
	}

	return m, nil
}

// toggleContainerCmd starts or stops a container depending on its current status.
func toggleContainerCmd(dc docker.DockerClient, id, status string) tea.Cmd {
	return func() tea.Msg {
		var err error
		if status == "running" {
			err = dc.StopContainer(id)
		} else {
			err = dc.StartContainer(id)
		}
		if err != nil {
			return dataMsg{err: err}
		}
		// Fetch fresh data right after the action
		return fetchDataCmd(dc)()
	}
}

// --- helpers ---

func (m Model) activeListLen() int {
	switch m.activePanel {
	case PanelContainers:
		return len(m.containers)
	case PanelNetworks:
		return len(m.networks)
	case PanelImages:
		return len(m.images)
	}
	return 0
}

func keyMatches(msg tea.KeyMsg, b key.Binding) bool {
	return key.Matches(msg, b)
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
