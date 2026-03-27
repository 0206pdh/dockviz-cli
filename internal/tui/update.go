// update.go implements the Bubble Tea Update function.
// It handles all incoming messages (key presses, ticks, fetched data) and
// returns a new Model + optional Command to run next.
package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/0206pdh/dockviz-cli/internal/docker"
)

// logLineMsg carries a single log line received from the streaming goroutine.
type logLineMsg string

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

		// Update per-container CPU sparkline history.
		// We keep at most 10 readings per container to fit the sparkline width.
		for _, c := range m.containers {
			h := m.history[c.ID]
			if c.Status == "running" {
				h = append(h, c.CPUPerc)
				if len(h) > 10 {
					h = h[len(h)-10:]
				}
			}
			m.history[c.ID] = h
		}
		return m, nil

	// A single log line arrived from the streaming goroutine
	case logLineMsg:
		m.logs = append(m.logs, string(msg))
		// Auto-scroll to the newest line
		m.logScroll = len(m.logs)
		// Immediately wait for the next line on the same channel
		if m.logCh != nil {
			return m, waitForLogCmd(m.logCh)
		}
		return m, nil

	// Keyboard input
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

// handleKey dispatches key presses based on the current view and overlay state.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	km := m.keys

	// --- Delete confirmation overlay intercepts ALL keys while visible ---
	if m.confirmDelete {
		switch msg.String() {
		case "y", "Y":
			// User confirmed: remove the container and refresh data.
			id := m.containers[m.cursor].ID
			m.confirmDelete = false
			return m, removeContainerCmd(m.docker, id)
		case "n", "N", "esc":
			// User cancelled.
			m.confirmDelete = false
		}
		return m, nil
	}

	// --- Log view key handling ---
	if m.activeView == ViewLogs {
		switch {
		case keyMatches(msg, km.Quit):
			// Stop the log stream before quitting.
			if m.logCancel != nil {
				m.logCancel()
				m.logCancel = nil
			}
			return m, tea.Quit

		case keyMatches(msg, km.Back):
			// Stop stream and return to the dashboard.
			if m.logCancel != nil {
				m.logCancel()
				m.logCancel = nil
			}
			m.logCh = nil
			m.activeView = ViewDashboard
			return m, nil

		case keyMatches(msg, km.Up):
			if m.logScroll > 0 {
				m.logScroll--
			}
			return m, nil

		case keyMatches(msg, km.Down):
			if m.logScroll < len(m.logs) {
				m.logScroll++
			}
			return m, nil
		}
		return m, nil
	}

	// --- Normal dashboard key handling ---
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

	case keyMatches(msg, km.Delete):
		// Show the confirmation overlay — actual removal happens on "y".
		if m.activePanel == PanelContainers && len(m.containers) > 0 {
			m.confirmDelete = true
		}

	case keyMatches(msg, km.Logs):
		// Open the log view for the selected container, starting a fresh stream.
		if m.activePanel == PanelContainers && len(m.containers) > 0 {
			ctr := m.containers[m.cursor]
			// Cancel any previous stream to avoid leaking goroutines.
			if m.logCancel != nil {
				m.logCancel()
			}
			ctx, cancel := context.WithCancel(context.Background())
			ch := m.docker.StreamLogs(ctx, ctr.ID)
			m.selectedID = ctr.ID
			m.activeView = ViewLogs
			m.logs = nil
			m.logScroll = 0
			m.logCh = ch
			m.logCancel = cancel
			return m, waitForLogCmd(ch)
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

// removeContainerCmd force-removes a container then refreshes the container list.
func removeContainerCmd(dc docker.DockerClient, id string) tea.Cmd {
	return func() tea.Msg {
		_ = dc.RemoveContainer(id)
		return fetchDataCmd(dc)()
	}
}

// waitForLogCmd blocks until the next line arrives on ch, then emits it as a logLineMsg.
// Returns nil when the channel is closed (stream ended).
func waitForLogCmd(ch <-chan docker.LogLine) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			// Channel closed — stream ended or context cancelled.
			return nil
		}
		return logLineMsg(line.Text)
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
