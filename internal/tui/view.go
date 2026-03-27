// view.go implements the Bubble Tea View function.
// It converts the current Model state into a string that Bubble Tea renders to the terminal.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/yourusername/dockviz-cli/internal/docker"
	"github.com/yourusername/dockviz-cli/internal/ui"
)

// View is called by Bubble Tea after every Update to re-render the screen.
func (m Model) View() string {
	if m.loading && len(m.containers) == 0 {
		return fmt.Sprintf("\n  %s Connecting to Docker...\n", m.spinner.View())
	}
	if m.err != nil {
		return ui.ErrorStyle.Render(fmt.Sprintf("\n  Error: %v\n\n  Press q to quit.", m.err))
	}

	switch m.activeView {
	case ViewDetail:
		return m.renderDetail()
	case ViewLogs:
		return m.renderLogs()
	default:
		return m.renderDashboard()
	}
}

// renderDashboard builds the main three-panel layout.
func (m Model) renderDashboard() string {
	title := ui.TitleStyle.Render("  dockviz  ") +
		ui.FooterStyle.Render(fmt.Sprintf("v0.1.0  •  %d containers", len(m.containers)))

	tabs := m.renderTabs()
	body := m.renderActivePanel()
	footer := m.renderFooter()

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		tabs,
		body,
		footer,
	)
}

// renderTabs shows the panel switcher at the top.
func (m Model) renderTabs() string {
	panels := []string{"Containers", "Networks", "Images"}
	var parts []string
	for i, name := range panels {
		s := fmt.Sprintf(" %s ", name)
		if Panel(i) == m.activePanel {
			parts = append(parts, lipgloss.NewStyle().
				Bold(true).
				Foreground(ui.ColorBlue).
				Underline(true).
				Render(s))
		} else {
			parts = append(parts, lipgloss.NewStyle().
				Foreground(ui.ColorGray).
				Render(s))
		}
	}
	return "  " + strings.Join(parts, "  |  ") + "\n"
}

// renderActivePanel renders whichever panel is currently selected.
func (m Model) renderActivePanel() string {
	switch m.activePanel {
	case PanelNetworks:
		return m.renderNetworks()
	case PanelImages:
		return m.renderImages()
	default:
		return m.renderContainers()
	}
}

// renderContainers builds the container list table.
func (m Model) renderContainers() string {
	header := ui.HeaderStyle.Render(
		fmt.Sprintf("  %-20s %-8s %-8s %-10s %-18s", "NAME", "CPU", "MEM", "STATUS", "PORTS"),
	)

	var rows []string
	rows = append(rows, header)

	for i, c := range m.containers {
		statusIcon := ui.StatusIcon(c.Status)
		statusStr := ui.StatusStyle(c.Status).Render(fmt.Sprintf("%s %-8s", statusIcon, c.Status))

		cpu := "-"
		mem := "-"
		if c.Status == "running" {
			cpu = fmt.Sprintf("%.1f%%", c.CPUPerc)
			mem = fmt.Sprintf("%.0fMB", c.MemMB)
		}

		row := fmt.Sprintf("  %-20s %-8s %-8s %s %-18s",
			truncate(c.Name, 20), cpu, mem, statusStr, truncate(c.Ports, 18))

		if i == m.cursor {
			row = ui.SelectedRowStyle.Render(row)
		}
		rows = append(rows, row)
	}

	if len(m.containers) == 0 {
		rows = append(rows, "\n  No containers found.")
	}

	return strings.Join(rows, "\n")
}

// renderNetworks builds the network topology view.
func (m Model) renderNetworks() string {
	header := ui.HeaderStyle.Render("  Network Topology")
	graph := ui.RenderNetworkGraph(m.networks)
	return header + "\n\n" + graph
}

// renderImages builds the image list table.
func (m Model) renderImages() string {
	header := ui.HeaderStyle.Render(
		fmt.Sprintf("  %-40s %-14s %-10s", "TAG", "ID", "SIZE"),
	)
	var rows []string
	rows = append(rows, header)

	for i, img := range m.images {
		row := fmt.Sprintf("  %-40s %-14s %-10s",
			truncate(img.Tags, 40), img.ID, docker.FormatSize(img.SizeMB))
		if i == m.cursor {
			row = ui.SelectedRowStyle.Render(row)
		}
		rows = append(rows, row)
	}
	return strings.Join(rows, "\n")
}

// renderDetail shows detailed info for the selected container.
func (m Model) renderDetail() string {
	for _, c := range m.containers {
		if c.ID == m.selectedID {
			return fmt.Sprintf(
				"\n  %s\n\n  ID:     %s\n  Image:  %s\n  Status: %s\n  Ports:  %s\n\n  [Esc] Back\n",
				ui.TitleStyle.Render(c.Name),
				c.ID, c.Image,
				ui.StatusStyle(c.Status).Render(ui.StatusIcon(c.Status)+" "+c.Status),
				c.Ports,
			)
		}
	}
	return "\n  Container not found. [Esc] Back\n"
}

// renderLogs is a placeholder — full log streaming will be implemented next.
func (m Model) renderLogs() string {
	return fmt.Sprintf("\n  Logs for container %s\n\n  (log streaming coming soon)\n\n  [Esc] Back\n", m.selectedID)
}

// renderFooter shows the keybinding hints at the bottom.
func (m Model) renderFooter() string {
	hints := "[q] Quit  [Tab] Switch Panel  [↑↓] Navigate  [Enter] Detail  [s] Start/Stop  [l] Logs  [r] Refresh"
	return "\n" + ui.FooterStyle.Render(hints)
}

// truncate shortens s to max length, adding "…" if needed.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
