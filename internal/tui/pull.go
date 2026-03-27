// pull.go implements the pull progress TUI program.
// It is a standalone Bubble Tea program (not the main dashboard),
// launched by `dockviz pull <image>`.
package tui

import (
	"fmt"
	"math"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/0206pdh/dockviz-cli/internal/docker"
	"github.com/0206pdh/dockviz-cli/internal/ui"
)

// pullModel is the Bubble Tea model for the pull progress screen.
type pullModel struct {
	ref     string               // image reference being pulled
	layers  []docker.LayerStatus // latest layer snapshot
	overall string               // final status line from Docker
	done    bool
	err     error
	events  <-chan docker.PullEvent
}

// pullEventMsg wraps a docker.PullEvent for Bubble Tea.
type pullEventMsg docker.PullEvent

// StartPull runs the pull TUI for the given image reference.
func StartPull(dc docker.DockerClient, ref string) error {
	realClient, ok := dc.(*docker.Client)
	if !ok {
		return fmt.Errorf("pull requires a live Docker connection (not demo mode)")
	}

	ch := realClient.PullImage(ref)
	m := pullModel{ref: ref, events: ch}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// Init starts listening for pull events.
func (m pullModel) Init() tea.Cmd {
	return waitForPullEvent(m.events)
}

// waitForPullEvent returns a Cmd that blocks until the next PullEvent arrives.
func waitForPullEvent(ch <-chan docker.PullEvent) tea.Cmd {
	return func() tea.Msg {
		evt, ok := <-ch
		if !ok {
			return pullEventMsg{Done: true}
		}
		return pullEventMsg(evt)
	}
}

func (m pullModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" || msg.String() == "esc" {
			return m, tea.Quit
		}

	case pullEventMsg:
		m.layers = msg.Layers
		m.overall = msg.Overall
		m.done = msg.Done
		m.err = msg.Err

		if m.err != nil || m.done {
			return m, tea.Quit
		}
		return m, waitForPullEvent(m.events)
	}
	return m, nil
}

func (m pullModel) View() string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(ui.TitleStyle.Render("  Pulling " + m.ref))
	sb.WriteString("\n\n")

	if m.err != nil {
		sb.WriteString(ui.ErrorStyle.Render("  Error: " + m.err.Error()))
		sb.WriteString("\n")
		return sb.String()
	}

	var totalCurrent, totalBytes int64
	for _, l := range m.layers {
		totalCurrent += l.Current
		totalBytes += l.Total
		sb.WriteString(renderLayer(l))
		sb.WriteString("\n")
	}

	if len(m.layers) > 0 {
		sb.WriteString("\n")
		if totalBytes > 0 {
			sb.WriteString(ui.FooterStyle.Render(
				fmt.Sprintf("  Downloaded %s / %s",
					formatBytes(totalCurrent), formatBytes(totalBytes)),
			))
			sb.WriteString("\n")
		}
	}

	if m.overall != "" {
		sb.WriteString("\n")
		sb.WriteString(ui.HeaderStyle.Render("  " + m.overall))
		sb.WriteString("\n")
	}

	if !m.done {
		sb.WriteString("\n" + ui.FooterStyle.Render("  [q] Cancel"))
	}

	return sb.String()
}

// renderLayer draws one layer row: ID  progressbar  percent  status
func renderLayer(l docker.LayerStatus) string {
	id := l.ID
	if len(id) > 12 {
		id = id[:12]
	}

	idStr := lipgloss.NewStyle().Foreground(ui.ColorGray).Render(fmt.Sprintf("  %-12s", id))

	var bar, pct, statusStr string

	switch l.Status {
	case "Already exists":
		bar = lipgloss.NewStyle().Foreground(ui.ColorGray).Render(strings.Repeat("─", 20))
		statusStr = lipgloss.NewStyle().Foreground(ui.ColorGray).Render("Already exists")
		pct = "     "

	case "Pull complete":
		bar = ui.StatusRunning.Render(strings.Repeat("█", 20))
		statusStr = ui.StatusRunning.Render("Pull complete ✓")
		pct = "100% "

	case "Downloading":
		ratio := 0.0
		if l.Total > 0 {
			ratio = float64(l.Current) / float64(l.Total)
		}
		filled := int(math.Round(ratio * 20))
		empty := 20 - filled
		bar = ui.StatusRunning.Render(strings.Repeat("█", filled)) +
			lipgloss.NewStyle().Foreground(ui.ColorGray).Render(strings.Repeat("░", empty))
		pct = fmt.Sprintf("%3.0f%% ", ratio*100)
		statusStr = lipgloss.NewStyle().Foreground(ui.ColorWhite).
			Render(fmt.Sprintf("%s / %s", formatBytes(l.Current), formatBytes(l.Total)))

	default:
		bar = lipgloss.NewStyle().Foreground(ui.ColorGray).Render(strings.Repeat("░", 20))
		statusStr = lipgloss.NewStyle().Foreground(ui.ColorGray).Render(l.Status)
		pct = "     "
	}

	return fmt.Sprintf("%s  %s  %s  %s", idStr, bar, pct, statusStr)
}

func formatBytes(b int64) string {
	const mb = 1024 * 1024
	if b >= mb {
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	}
	return fmt.Sprintf("%d KB", b/1024)
}
