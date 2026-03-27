// view.go implements the Bubble Tea View function.
// It converts the current Model state into a string that Bubble Tea renders to the terminal.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/0206pdh/dockviz-cli/internal/docker"
	"github.com/0206pdh/dockviz-cli/internal/ui"
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
// If the delete confirmation overlay is active it is appended below the table.
func (m Model) renderDashboard() string {
	// Show ↻ Refreshing... next to the title when a manual refresh is in progress.
	statusHint := ""
	if m.refreshing {
		statusHint = lipgloss.NewStyle().Foreground(ui.ColorYellow).Render("  ↻ Refreshing...")
	}
	title := ui.TitleStyle.Render("  dockviz  ") +
		ui.FooterStyle.Render(fmt.Sprintf("%s  •  %d containers", m.version, len(m.containers))) +
		statusHint

	tabs := m.renderTabs()
	body := m.renderActivePanel()
	footer := m.renderFooter()

	// When 'd' is pressed, replace the entire screen with the confirmation dialog.
	if m.confirmDelete {
		if m.activePanel == PanelImages && len(m.images) > 0 {
			return m.renderConfirmDelete(m.images[m.cursor].Tags, "image")
		}
		if m.activePanel == PanelContainers && len(m.containers) > 0 {
			return m.renderConfirmDelete(m.containers[m.cursor].Name, "container")
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		tabs,
		body,
		footer,
	)
}

// renderConfirmDelete replaces the full screen with a prominent red-bordered dialog.
// kind is "container" or "image".
func (m Model) renderConfirmDelete(name, kind string) string {
	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(ui.ColorRed).
		Padding(1, 4).
		Width(50)

	kindLabel := "Container"
	subText := "This will force-remove the container.\nRunning containers are stopped first."
	if kind == "image" {
		kindLabel = "Image"
		subText = "This will remove the local image.\nContainers using it must be removed first."
	}

	title := lipgloss.NewStyle().Bold(true).Foreground(ui.ColorRed).Render("  ⚠  Confirm Delete")
	line1 := fmt.Sprintf("%s:  %s", kindLabel, lipgloss.NewStyle().Bold(true).Foreground(ui.ColorWhite).Render(truncate(name, 30)))
	line2 := lipgloss.NewStyle().Foreground(ui.ColorGray).Render(subText)
	gap := ""
	confirm := ui.ErrorStyle.Render("[y] Yes, remove") + "    " +
		lipgloss.NewStyle().Foreground(ui.ColorGray).Render("[n / Esc] Cancel")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title, gap, line1, gap, line2, gap, confirm,
	)

	return "\n\n\n" + "  " + dialogStyle.Render(content)
}

// renderTabs shows the panel switcher.
// Active tab: bright cyan background, bold black text, surrounded by [ ] brackets.
// Inactive tab: plain dim text, no background, clearly different.
func (m Model) renderTabs() string {
	type tabDef struct {
		label string
		panel Panel
	}
	tabs := []tabDef{
		{" 📦 Containers ", PanelContainers},
		{" 🌐 Networks ", PanelNetworks},
		{" 🗃  Images ", PanelImages},
	}

	var parts []string
	for _, t := range tabs {
		if t.panel == m.activePanel {
			parts = append(parts, lipgloss.NewStyle().
				Bold(true).
				Background(lipgloss.Color("#00CFCF")).
				Foreground(lipgloss.Color("#000000")).
				Padding(0, 1).
				Render("▶"+t.label+"◀"))
		} else {
			parts = append(parts, lipgloss.NewStyle().
				Foreground(lipgloss.Color("#555555")).
				Padding(0, 1).
				Render(t.label))
		}
	}

	tabBar := "  " + strings.Join(parts, "  ")
	divider := lipgloss.NewStyle().Foreground(ui.ColorGray).Render(
		"  " + strings.Repeat("─", 60),
	)
	return tabBar + "\n" + divider + "\n"
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
// Column widths (display chars): cursor=2 | NAME=20 | GRAPH=10 | CPU=8 | MEM=8 | STATUS=12 | PORTS=18
// ANSI-coloured columns (GRAPH, STATUS) are pre-padded to their target width BEFORE
// wrapping in a colour style, so fmt.Sprintf sees only the plain-text width.
func (m Model) renderContainers() string {
	// Header prefix is 2 spaces to match the 2-char cursor field in each row.
	header := ui.HeaderStyle.Render(
		fmt.Sprintf("  %-20s %-10s %-8s %-8s %-12s %-18s",
			"NAME", "GRAPH", "CPU", "MEM", "STATUS", "PORTS"),
	)

	var rows []string
	rows = append(rows, header)

	for i, c := range m.containers {
		cursor := "  "
		if i == m.cursor {
			cursor = "▶ "
		}

		cpu := "-"
		mem := "-"
		if c.Status == "running" {
			cpu = fmt.Sprintf("%.1f%%", c.CPUPerc)
			mem = fmt.Sprintf("%.0fMB", c.MemMB)
		}

		// Sparkline is always 10 runes wide (padded by ui.Sparkline).
		// Colorize after padding so ANSI codes don't shift subsequent columns.
		spark := ui.Sparkline(m.history[c.ID])
		var sparkStr string
		if c.Status == "running" {
			sparkStr = lipgloss.NewStyle().Foreground(ui.ColorGreen).Render(spark)
		} else {
			sparkStr = lipgloss.NewStyle().Foreground(ui.ColorGray).Render(spark)
		}

		// Pre-pad status text to 12 chars, then colorize.
		statusText := fmt.Sprintf("%-12s", fmt.Sprintf("%s %s", ui.StatusIcon(c.Status), c.Status))
		statusStr := ui.StatusStyle(c.Status).Render(statusText)

		// Use %s (no width) for pre-padded ANSI columns; %-Ns for plain-text columns.
		row := fmt.Sprintf("%s%-20s %s %-8s %-8s %s %-18s",
			cursor,
			truncate(c.Name, 20),
			sparkStr,  // 10 wide, pre-padded
			cpu,       // %-8s
			mem,       // %-8s
			statusStr, // 12 wide, pre-padded
			truncate(c.Ports, 18),
		)

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
		cursor := "  "
		if i == m.cursor {
			cursor = "▶ "
		}
		row := fmt.Sprintf("%s%-40s %-14s %-10s",
			cursor, truncate(img.Tags, 40), img.ID, docker.FormatSize(img.SizeMB))
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

// renderLogs displays the streamed log output for the selected container.
// Lines containing ERROR/error are coloured red; WARN/warn lines are yellow;
// all others appear in the default white. The view respects m.logScroll for
// manual scrolling and auto-scrolls to the bottom as new lines arrive.
func (m Model) renderLogs() string {
	// Find the container name for the title bar.
	containerName := m.selectedID
	for _, c := range m.containers {
		if c.ID == m.selectedID {
			containerName = c.Name
			break
		}
	}

	title := ui.TitleStyle.Render(fmt.Sprintf("  Logs — %s", containerName))
	footer := "\n" + ui.FooterStyle.Render("[Esc] Back  [↑↓] Scroll")

	// Reserve lines for title (1) + blank (1) + footer (2).
	const reservedLines = 4
	visibleLines := m.height - reservedLines
	if visibleLines < 1 {
		visibleLines = 1
	}

	// Determine the slice of logs to display based on scroll position.
	total := len(m.logs)
	end := m.logScroll
	if end > total {
		end = total
	}
	start := end - visibleLines
	if start < 0 {
		start = 0
	}

	var sb strings.Builder
	sb.WriteString(title)
	sb.WriteString("\n\n")

	if total == 0 {
		sb.WriteString(ui.FooterStyle.Render("  Waiting for log output..."))
	} else {
		for _, line := range m.logs[start:end] {
			coloredLine := colorLogLine(line)
			sb.WriteString("  ")
			sb.WriteString(coloredLine)
			sb.WriteString("\n")
		}
	}

	sb.WriteString(footer)
	return sb.String()
}

// colorLogLine applies colour styling to a log line based on its severity keywords.
func colorLogLine(line string) string {
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "error"):
		return lipgloss.NewStyle().Foreground(ui.ColorRed).Render(line)
	case strings.Contains(lower, "warn"):
		return lipgloss.NewStyle().Foreground(ui.ColorYellow).Render(line)
	default:
		return lipgloss.NewStyle().Foreground(ui.ColorWhite).Render(line)
	}
}

// renderFooter shows the keybinding hints at the bottom split across two lines.
func (m Model) renderFooter() string {
	line1 := "[q] Quit  [Tab] Switch Panel  [↑↓] Navigate  [Enter] Detail  [r] Refresh"
	line2 := "[s] Start/Stop  [d] Delete  [l] Logs (Containers)"
	return "\n" + ui.FooterStyle.Render(line1) + "\n" + ui.FooterStyle.Render(line2)
}

// truncate shortens s to max length, adding "…" if needed.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
