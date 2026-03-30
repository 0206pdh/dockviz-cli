// view.go implements the Bubble Tea View function.
// It converts the current Model state into a string that Bubble Tea renders to the terminal.
package tui

import (
	"fmt"
	"math"
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
	case ViewChart:
		return m.renderChart()
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
		if m.activePanel == PanelContainers && len(m.containers) > 0 {
			return m.renderConfirmDelete(m.containers[m.cursor].Name, "container",
				"This will force-remove the container.\nRunning containers are stopped first.")
		}
		if m.activePanel == PanelImages && len(m.images) > 0 {
			img := m.images[m.cursor]
			subText := "This will remove the image and free disk space."
			if len(img.AllTags) > 1 {
				subText = fmt.Sprintf("Only this tag will be removed.\nImage remains (%d tags total).", len(img.AllTags))
			}
			return m.renderConfirmDelete(img.Tag, "image", subText)
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
// kind is "container" or "image". subText is shown as the secondary description line.
func (m Model) renderConfirmDelete(name, kind, subText string) string {
	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(ui.ColorRed).
		Padding(1, 4).
		Width(50)

	kindLabel := "Container"
	if kind == "image" {
		kindLabel = "Tag"
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
// Active tab: bright cyan background, bold black text, surrounded by ▶◀.
// Inactive tab: plain dim text.
func (m Model) renderTabs() string {
	type tabDef struct {
		label string
		panel Panel
	}
	tabs := []tabDef{
		{" 📦 Containers ", PanelContainers},
		{" 🌐 Networks ", PanelNetworks},
		{" 🗃  Images ", PanelImages},
		{" 📋 Events ", PanelEvents},
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
	case PanelEvents:
		return m.renderEvents()
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

		// Sparkline shows the last 10 readings. History stores up to 60 for the chart.
		// Colorize after padding so ANSI codes don't shift subsequent columns.
		h := m.history[c.ID]
		if len(h) > 10 {
			h = h[len(h)-10:]
		}
		spark := ui.Sparkline(h)
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

// renderNetworks shows a split layout: left = topology graph, right = event timeline.
// When m.width < 80 (narrow terminal or before the first WindowSizeMsg) it falls
// back to a simple network list to avoid layout corruption.
func (m Model) renderNetworks() string {
	if m.width < 80 {
		return m.renderNetworksFallback()
	}

	halfW := m.width/2 - 4

	// --- Left panel: topology graph ---
	topoTitle := lipgloss.NewStyle().Bold(true).Foreground(ui.ColorBlue).Render("  Topology")
	topoBody := ui.RenderNetworkGraph(m.networks, m.ContainerStates)
	leftContent := topoTitle + "\n\n" + topoBody
	leftPanel := ui.PanelStyle.Width(halfW).Render(leftContent)

	// --- Right panel: event timeline for selected network ---
	var timelineTitle string
	var timelineBody string
	if m.cursor < len(m.networks) {
		n := m.networks[m.cursor]
		timelineTitle = lipgloss.NewStyle().Bold(true).Foreground(ui.ColorBlue).
			Render(fmt.Sprintf("  Events — %s", n.Name))

		// Build a set of container names belonging to this network.
		netContainers := make(map[string]bool, len(n.Containers))
		for _, ep := range n.Containers {
			netContainers[ep.Name] = true
		}

		// Filter events to this network's containers.
		var filtered []string
		maxRows := m.height - 10
		if maxRows < 4 {
			maxRows = 4
		}
		for _, e := range m.events {
			if !netContainers[e.ContainerName] {
				continue
			}
			timeStr := e.Time.Format("15:04:05")
			icon, style := eventActionStyle(e.Action)
			actionText := fmt.Sprintf("%-10s", icon+" "+e.Action)
			actionStr := style.Render(actionText)

			line := fmt.Sprintf("  %-10s %s %-18s", timeStr, actionStr, truncate(e.ContainerName, 18))

			// Append ExitCode / OOM annotation for die events.
			if e.Action == "die" {
				ann := fmt.Sprintf(" exit=%d", e.ExitCode)
				if e.OOMKilled {
					ann += lipgloss.NewStyle().Foreground(ui.ColorRed).Render(" OOM")
				}
				line += ann
			}
			filtered = append(filtered, line)
			if len(filtered) >= maxRows {
				break
			}
		}

		if len(filtered) == 0 {
			timelineBody = "\n  " + lipgloss.NewStyle().Foreground(ui.ColorGray).
				Render("No events for this network yet.")
		} else {
			timelineBody = "\n" + strings.Join(filtered, "\n")
		}
	} else {
		timelineTitle = lipgloss.NewStyle().Foreground(ui.ColorGray).Render("  Events")
		timelineBody = "\n  " + lipgloss.NewStyle().Foreground(ui.ColorGray).Render("Select a network.")
	}

	rightContent := timelineTitle + timelineBody
	rightPanel := ui.PanelStyle.Width(halfW).Render(rightContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, "  ", rightPanel)
}

// renderNetworksFallback is used when the terminal is narrower than 80 columns.
// It renders a plain network list without the split topology/timeline panels.
func (m Model) renderNetworksFallback() string {
	header := ui.HeaderStyle.Render(
		fmt.Sprintf("  %-22s %-10s %-20s %-5s", "NETWORK", "DRIVER", "SUBNET", "CTRS"),
	)

	var rows []string
	rows = append(rows, header)

	for i, n := range m.networks {
		cursor := "  "
		if i == m.cursor {
			cursor = "▶ "
		}
		subnet := n.Subnet
		if subnet == "" {
			subnet = "—"
		}
		row := fmt.Sprintf("%s%-22s %-10s %-20s %d",
			cursor,
			truncate(n.Name, 22),
			truncate(n.Driver, 10),
			truncate(subnet, 20),
			len(n.Containers),
		)
		if i == m.cursor {
			row = ui.SelectedRowStyle.Render(row)
		}
		rows = append(rows, row)
	}

	if len(m.networks) == 0 {
		rows = append(rows, "\n  No networks found.")
	}
	return strings.Join(rows, "\n")
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
			cursor, truncate(img.Tag, 40), img.ID, docker.FormatSize(img.SizeMB))
		if i == m.cursor {
			row = ui.SelectedRowStyle.Render(row)
		}
		rows = append(rows, row)
	}
	return strings.Join(rows, "\n")
}

// renderEvents shows a live-updating timeline of Docker container lifecycle events.
// Events are displayed newest-first. The panel starts streaming when first visited.
func (m Model) renderEvents() string {
	header := ui.HeaderStyle.Render(
		fmt.Sprintf("  %-10s %-14s %-22s %-12s", "TIME", "ACTION", "CONTAINER", "ID"),
	)

	var rows []string
	rows = append(rows, header)

	if len(m.events) == 0 {
		hint := "\n  " + lipgloss.NewStyle().Foreground(ui.ColorGray).Render("Waiting for Docker events...")
		rows = append(rows, hint)
		return strings.Join(rows, "\n")
	}

	// Cap visible rows to the available terminal height.
	maxVisible := m.height - 9
	if maxVisible < 5 {
		maxVisible = 5
	}
	events := m.events
	if len(events) > maxVisible {
		events = events[:maxVisible]
	}

	for _, e := range events {
		timeStr := e.Time.Format("15:04:05")

		// Pre-pad the action field (icon + text) to a fixed width before colorising,
		// so subsequent columns stay aligned despite ANSI codes.
		icon, style := eventActionStyle(e.Action)
		actionText := fmt.Sprintf("%-12s", icon+" "+e.Action)
		actionStr := style.Render(actionText)

		row := fmt.Sprintf("  %-10s %s %-22s %-12s",
			timeStr,
			actionStr,
			truncate(e.ContainerName, 22),
			e.ContainerID,
		)
		rows = append(rows, row)
	}

	// Live / disconnected indicator in the last line.
	var liveHint string
	if m.eventDisconnected {
		liveHint = "\n  " + lipgloss.NewStyle().Foreground(ui.ColorRed).
			Render("○ disconnected  •  press [r] to reconnect")
	} else {
		liveHint = "\n  " + lipgloss.NewStyle().Foreground(ui.ColorGray).
			Render(fmt.Sprintf("● live  •  %d events", len(m.events)))
	}
	rows = append(rows, liveHint)

	return strings.Join(rows, "\n")
}

// eventActionStyle returns the icon and lip gloss style for a given Docker event action.
func eventActionStyle(action string) (string, lipgloss.Style) {
	switch action {
	case "start", "unpause":
		return "●", lipgloss.NewStyle().Foreground(ui.ColorGreen)
	case "die", "kill", "stop":
		return "○", lipgloss.NewStyle().Foreground(ui.ColorRed)
	case "pause":
		return "◑", lipgloss.NewStyle().Foreground(ui.ColorYellow)
	case "restart":
		return "↻", lipgloss.NewStyle().Foreground(ui.ColorBlue)
	case "create":
		return "✦", lipgloss.NewStyle().Foreground(ui.ColorGray)
	case "destroy":
		return "✕", lipgloss.NewStyle().Foreground(ui.ColorRed)
	default:
		return "•", lipgloss.NewStyle().Foreground(ui.ColorGray)
	}
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

// renderFooter shows the keybinding hints at the bottom, adapted to the active panel.
func (m Model) renderFooter() string {
	line1 := "[q] Quit  [Tab] Switch Panel  [↑↓] Navigate  [r] Refresh"
	var line2 string
	switch m.activePanel {
	case PanelContainers:
		line2 = "[Enter] Detail  [s] Start/Stop  [d] Delete  [l] Logs  [g] Chart"
	case PanelNetworks:
		line2 = "● running  ◑ restarting  ✗ dead  ○ unknown  ·  [↑↓] select network"
	case PanelImages:
		line2 = "[d] Delete image"
	case PanelEvents:
		line2 = "● live streaming  ·  events appear as they happen"
	}
	return "\n" + ui.FooterStyle.Render(line1) + "\n" + ui.FooterStyle.Render(line2)
}

// renderChart shows a full-screen CPU and memory history chart for the selected container.
// Press Esc to return to the dashboard. Data updates every 2 seconds (up to 60 points).
func (m Model) renderChart() string {
	var ctrName string
	for _, c := range m.containers {
		if c.ID == m.selectedID {
			ctrName = c.Name
			break
		}
	}
	if ctrName == "" {
		return "\n  Container not found. [Esc] Back\n"
	}

	title := ui.TitleStyle.Render(fmt.Sprintf("  Stats History — %s", ctrName))
	footer := "\n" + ui.FooterStyle.Render("[Esc] Back  •  2s refresh  •  up to 60 data points (last 2 min)")

	chartW := m.width - 14
	if chartW < 20 {
		chartW = 20
	}
	const chartH = 8

	// Dynamic CPU scale: round up to nearest 100, minimum 100.
	var maxCPU float64 = 100.0
	for _, v := range m.history[m.selectedID] {
		if v > maxCPU {
			maxCPU = v
		}
	}
	maxCPU = math.Ceil(maxCPU/100) * 100

	cpuSection := renderChartSection("CPU", "%", m.history[m.selectedID], maxCPU, chartW, chartH)

	var maxMem float64 = 10
	for _, v := range m.memHistory[m.selectedID] {
		if v > maxMem {
			maxMem = v
		}
	}
	maxMem = math.Ceil(maxMem/10) * 10

	memSection := renderChartSection("Memory", "MB", m.memHistory[m.selectedID], maxMem, chartW, chartH)

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		cpuSection,
		"",
		memSection,
		footer,
	)
}

// renderChartSection renders a labelled bar chart for a single metric (CPU or Memory).
//
// Layout per row:
//   "  %7s ┤ " (12 chars) + chartW bar chars
//
// Y-axis: even rows + bottom row labelled with their upper boundary value.
//   For CPU, the row containing 80% is relabelled in red and the row
//   containing 50% is relabelled in yellow so danger thresholds are clear.
//
// Threshold lines: empty cells in the 80% and 50% CPU rows show a
//   coloured dot so the boundary is visible even when bars are low.
//
// X-axis: a "└──… now" line followed by a time-elapsed label.
func renderChartSection(label, unit string, values []float64, maxVal float64, chartW, chartH int) string {
	// ── section title ──────────────────────────────────────────────
	var currentVal float64
	if len(values) > 0 {
		currentVal = values[len(values)-1]
	}

	var currentStyle lipgloss.Style
	if label == "CPU" {
		switch {
		case currentVal > 80:
			currentStyle = lipgloss.NewStyle().Foreground(ui.ColorRed).Bold(true)
		case currentVal > 50:
			currentStyle = lipgloss.NewStyle().Foreground(ui.ColorYellow).Bold(true)
		default:
			currentStyle = lipgloss.NewStyle().Foreground(ui.ColorGreen).Bold(true)
		}
	} else {
		currentStyle = lipgloss.NewStyle().Foreground(ui.ColorBlue).Bold(true)
	}
	currentStr := currentStyle.Render(fmt.Sprintf("%.1f%s", currentVal, unit))
	scaleStr := lipgloss.NewStyle().Foreground(ui.ColorGray).
		Render(fmt.Sprintf("0 – %.0f%s", maxVal, unit))
	sectionTitle := lipgloss.NewStyle().Bold(true).Foreground(ui.ColorBlue).
		Render(fmt.Sprintf("  %s", label)) +
		"   " + currentStr + "   " + scaleStr

	// ── pad/trim values to chartW (newest on the right) ────────────
	blockRunes := []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	padded := make([]float64, chartW)
	if len(values) <= chartW {
		copy(padded[chartW-len(values):], values)
	} else {
		copy(padded, values[len(values)-chartW:])
	}

	// ── bar rows ───────────────────────────────────────────────────
	rows := make([]string, chartH)
	for row := 0; row < chartH; row++ {
		rowHigh := maxVal * float64(chartH-row) / float64(chartH)
		rowLow := maxVal * float64(chartH-1-row) / float64(chartH)

		// Threshold row detection (CPU only).
		isCPU80Row := label == "CPU" && 80.0 <= rowHigh && 80.0 > rowLow
		isCPU50Row := label == "CPU" && 50.0 <= rowHigh && 50.0 > rowLow

		// Y-axis label: threshold rows override; even rows show boundary; bottom = 0.
		var yLabelText string
		yLabelStyle := lipgloss.NewStyle()
		switch {
		case row == chartH-1:
			yLabelText = "0"
		case isCPU80Row:
			yLabelText = "80%"
			yLabelStyle = lipgloss.NewStyle().Foreground(ui.ColorRed)
		case isCPU50Row:
			yLabelText = "50%"
			yLabelStyle = lipgloss.NewStyle().Foreground(ui.ColorYellow)
		case row%2 == 0:
			yLabelText = fmt.Sprintf("%.0f%s", rowHigh, unit)
		}
		// Right-align plain text first (so fmt counts plain chars),
		// then colourize so ANSI codes don't shift column widths.
		yLabel := "  " + yLabelStyle.Render(fmt.Sprintf("%7s", yLabelText)) + " ┤ "

		// ── bar characters ────────────────────────────────────────
		var sb strings.Builder
		for _, v := range padded {
			var barStyle lipgloss.Style
			if label == "CPU" {
				switch {
				case v > 80:
					barStyle = lipgloss.NewStyle().Foreground(ui.ColorRed)
				case v > 50:
					barStyle = lipgloss.NewStyle().Foreground(ui.ColorYellow)
				default:
					barStyle = lipgloss.NewStyle().Foreground(ui.ColorGreen)
				}
			} else {
				barStyle = lipgloss.NewStyle().Foreground(ui.ColorBlue)
			}

			if v >= rowHigh {
				sb.WriteString(barStyle.Render("█"))
			} else if v > rowLow {
				fill := (v - rowLow) / (rowHigh - rowLow)
				idx := int(math.Round(fill * 8))
				if idx <= 0 {
					idx = 1
				}
				if idx > 8 {
					idx = 8
				}
				sb.WriteString(barStyle.Render(string(blockRunes[idx])))
			} else {
				// Empty cell — show threshold marker if applicable.
				switch {
				case isCPU80Row:
					sb.WriteString(lipgloss.NewStyle().Foreground(ui.ColorRed).Render("·"))
				case isCPU50Row:
					sb.WriteString(lipgloss.NewStyle().Foreground(ui.ColorYellow).Render("·"))
				default:
					sb.WriteRune(' ')
				}
			}
		}
		rows[row] = yLabel + sb.String()
	}

	// ── X-axis + time label ────────────────────────────────────────
	xAxisLine := fmt.Sprintf("  %7s └%s now", "", strings.Repeat("─", chartW))

	elapsed := len(values) * 2 // seconds of data currently stored
	var timeRow string
	if elapsed > 0 {
		timeRow = fmt.Sprintf("  %7s   %s", "",
			lipgloss.NewStyle().Foreground(ui.ColorGray).
				Render(fmt.Sprintf("← %s ago", formatElapsedSecs(elapsed))))
	} else {
		timeRow = fmt.Sprintf("  %7s   %s", "",
			lipgloss.NewStyle().Foreground(ui.ColorGray).Render("waiting for data..."))
	}

	lines := []string{sectionTitle}
	lines = append(lines, rows...)
	lines = append(lines, xAxisLine, timeRow)
	return strings.Join(lines, "\n")
}

// formatElapsedSecs converts a duration in seconds to a short human-readable string.
// Examples: 10 -> "10s", 90 -> "1m30s", 120 -> "2m".
func formatElapsedSecs(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	mins := seconds / 60
	secs := seconds % 60
	if secs == 0 {
		return fmt.Sprintf("%dm", mins)
	}
	return fmt.Sprintf("%dm%ds", mins, secs)
}

// truncate shortens s to max length, adding "…" if needed.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
