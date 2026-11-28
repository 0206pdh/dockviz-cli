// leftovers.go renders leftover resources: the Leftovers row in Disk Usage,
// the read-only Leftovers list view, and the summary in Problems. Detection
// lives in internal/docker/leftovers.go; nothing here removes anything.
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/0206pdh/dockviz-cli/internal/docker"
	"github.com/0206pdh/dockviz-cli/internal/ui"
	"github.com/charmbracelet/lipgloss"
)

// leftoversRowIndex is the Disk Usage cursor position of the Leftovers row,
// right after the prune categories.
func leftoversRowIndex() int { return len(diskUsageDefs) }

// leftoverTotals summarizes findings for the Disk Usage row and Problems.
type leftoverTotals struct {
	count, held                  int
	certain, likely, report      int
	sizeMB, removableMB          float64
	removableCount, unknownSizes int
}

func summarizeLeftovers(ls []docker.Leftover) leftoverTotals {
	var t leftoverTotals
	for _, l := range ls {
		t.count++
		switch l.Confidence {
		case docker.ConfidenceCertain:
			t.certain++
		case docker.ConfidenceLikely:
			t.likely++
		default:
			t.report++
		}
		if !l.SizeKnown {
			t.unknownSizes++
		}
		t.sizeMB += l.SizeMB
		if l.Removable {
			t.removableCount++
			t.removableMB += l.SizeMB
		} else {
			t.held++
		}
	}
	return t
}

// renderLeftoversRow is the Leftovers line under the prune categories. Its
// RECLAIMABLE column is the removable findings only; it is not a prune
// action and overlaps the categories above, so it is not added to them.
func (m Model) renderLeftoversRow() []string {
	cursor := "  "
	selected := m.cursor == leftoversRowIndex()
	if selected {
		cursor = "▶ "
	}
	gray := lipgloss.NewStyle().Foreground(ui.ColorGray)

	if !m.leftoversLoaded {
		row := fmt.Sprintf("%s%-16s %s", cursor, "Leftovers", "…")
		if selected {
			row = ui.SelectedRowStyle.Render(row)
		}
		return []string{row}
	}

	t := summarizeLeftovers(m.leftovers)
	size, reclaim := docker.FormatSize(t.sizeMB), docker.FormatSize(t.removableMB)
	if t.unknownSizes > 0 {
		size += " + ?"
		reclaim += " + ?"
	}
	row := fmt.Sprintf("%s%-16s %-7d %-7d %-10s %-12s", cursor, "Leftovers", t.count, t.held, size, reclaim)
	if selected {
		row = ui.SelectedRowStyle.Render(row)
	}
	out := []string{row}

	switch {
	case m.leftoversErr != nil:
		out = append(out, "  "+ui.ErrorStyle.Render("⚠ leftover detection failed: "+m.leftoversErr.Error()))
	case t.count == 0:
		out = append(out, "  "+gray.Render("no leftover resources found"))
	default:
		out = append(out, "  "+gray.Render(fmt.Sprintf(
			"certain %d · likely %d · info %d — [Enter] review each item (overlaps the rows above)",
			t.certain, t.likely, t.report)))
	}
	return out
}

var leftoverKindLabels = map[docker.LeftoverKind]string{
	docker.LeftoverComposeOrphan:    "compose orphan",
	docker.LeftoverComposeOneOff:    "compose run left",
	docker.LeftoverDroppedVolume:    "dropped volume",
	docker.LeftoverOwnerlessVolume:  "ownerless volume",
	docker.LeftoverAnonymousVolume:  "anonymous volume",
	docker.LeftoverStoppedContainer: "long-stopped",
	docker.LeftoverBuilderCache:     "buildx cache",
}

func leftoverKindLabel(k docker.LeftoverKind) string {
	if s, ok := leftoverKindLabels[k]; ok {
		return s
	}
	return string(k)
}

func leftoverConfidenceStyle(c docker.LeftoverConfidence) lipgloss.Style {
	switch c {
	case docker.ConfidenceCertain:
		return lipgloss.NewStyle().Foreground(ui.ColorYellow).Bold(true)
	case docker.ConfidenceLikely:
		return lipgloss.NewStyle().Foreground(ui.ColorBlue).Bold(true)
	}
	return lipgloss.NewStyle().Foreground(ui.ColorGray)
}

func leftoverStatus(l docker.Leftover) string {
	switch {
	case l.Confidence == docker.ConfidenceReport:
		return "info only"
	case l.Removable:
		return "removable"
	case len(l.HeldBy) > 0:
		return "held by " + strings.Join(l.HeldBy, ",")
	}
	return "in use"
}

func leftoverSize(l docker.Leftover) string {
	if !l.SizeKnown {
		return "N/A"
	}
	return docker.FormatSize(l.SizeMB)
}

// renderLeftovers is the full-screen, read-only Leftovers list.
func (m Model) renderLeftovers() string {
	gray := lipgloss.NewStyle().Foreground(ui.ColorGray)
	if m.confirmLeftover {
		return m.renderConfirmDelete(m.pendingLeftover.Name, "leftover", leftoverRemovalWarning(m.pendingLeftover))
	}

	title := ui.TitleStyle.Render("  dockviz  ") +
		ui.FooterStyle.Render("  Leftovers  •  [d] removes one item at a time, re-checked first and never forced")

	scopeLine := "no compose file loaded: orphan and dropped-volume checks are off (run in the project dir or pass -f)"
	if m.composeContext != nil {
		scopeLine = fmt.Sprintf("compose project %q (%s): orphans and dropped volumes are checked against it",
			m.composeContext.ProjectName, m.composeFilesLabel())
	}

	lines := []string{title, "", "  " + gray.Render(scopeLine), ""}

	switch {
	case m.leftoversErr != nil:
		lines = append(lines, "  "+ui.ErrorStyle.Render("⚠ leftover detection failed: "+m.leftoversErr.Error()))
	case !m.leftoversLoaded:
		lines = append(lines, "  "+gray.Render("Detecting leftovers..."))
	case len(m.leftovers) == 0:
		lines = append(lines, "  "+lipgloss.NewStyle().Foreground(ui.ColorGreen).Render("No leftover resources found"))
	default:
		lines = append(lines, ui.HeaderStyle.Render(
			fmt.Sprintf("  %-8s %-17s %-34s %-9s %s", "CONF", "KIND", "NAME", "SIZE", "STATUS")))
		start, end := listWindow(len(m.leftovers), m.leftoverCursor, m.leftoverListRows())
		for i := start; i < end; i++ {
			l := m.leftovers[i]
			cursor := "  "
			if i == m.leftoverCursor {
				cursor = "▶ "
			}
			conf := leftoverConfidenceStyle(l.Confidence).Render(fmt.Sprintf("%-8s", l.Confidence))
			row := fmt.Sprintf("%s%s %-17s %-34s %-9s %s", cursor, conf,
				leftoverKindLabel(l.Kind), truncate(l.Name, 34), leftoverSize(l), truncate(leftoverStatus(l), 36))
			if i == m.leftoverCursor {
				row = ui.SelectedRowStyle.Render(row)
			}
			lines = append(lines, row)
		}
		if start > 0 || end < len(m.leftovers) {
			lines = append(lines, "  "+gray.Render(fmt.Sprintf("%d–%d of %d", start+1, end, len(m.leftovers))))
		}
		lines = append(lines, "", m.renderLeftoverDetail(m.leftovers[clamp(m.leftoverCursor, 0, len(m.leftovers)-1)]))
	}

	switch {
	case m.leftoverRemoving:
		lines = append(lines, "", "  "+lipgloss.NewStyle().Foreground(ui.ColorYellow).Render("↻ Removing..."))
	case m.leftoverNote != "" && m.leftoverNoteErr:
		lines = append(lines, "", "  "+ui.ErrorStyle.Render("⚠ "+m.leftoverNote))
	case m.leftoverNote != "":
		lines = append(lines, "", "  "+lipgloss.NewStyle().Foreground(ui.ColorGreen).Render("✓ "+m.leftoverNote))
	}

	footer := ui.FooterStyle.Render("[↑↓] Navigate  [d] Remove selected  [r] Refresh  [Esc] Back  [q] Quit  ·  certain = labels prove it · likely = check first · info = never removed")
	return strings.Join(lines, "\n") + "\n\n" + footer
}

func (m Model) renderLeftoverDetail(l docker.Leftover) string {
	label := lipgloss.NewStyle().Foreground(ui.ColorGray).Width(9)
	value := lipgloss.NewStyle().Foreground(ui.ColorWhite)
	row := func(k, v string) string { return "  " + label.Render(k) + value.Render(v) }

	lines := []string{
		ui.HeaderStyle.Render("  " + strings.ToUpper(leftoverKindLabel(l.Kind)) + "  " + l.Name),
		row("Reason", l.Reason),
		row("Action", l.Action),
	}
	if l.Project != "" {
		svc := ""
		if l.Service != "" {
			svc = "  service " + l.Service
		}
		lines = append(lines, row("Project", l.Project+svc))
	}
	if len(l.HeldBy) > 0 {
		lines = append(lines, row("Held by", strings.Join(l.HeldBy, ", ")))
	}
	if l.ID != l.Name {
		lines = append(lines, row("ID", truncate(l.ID, 64)))
	}
	return strings.Join(lines, "\n")
}

// leftoverListRows is how many list rows fit above the detail block.
func (m Model) leftoverListRows() int {
	if m.height <= 0 {
		return 1 << 30
	}
	n := m.height - 16
	if n < 3 {
		n = 3
	}
	return n
}

// listWindow returns [start, end) of a scrolling window of size rows that
// keeps cursor visible.
func listWindow(total, cursor, rows int) (int, int) {
	if rows >= total {
		return 0, total
	}
	start := cursor - rows/2
	if start < 0 {
		start = 0
	}
	if start+rows > total {
		start = total - rows
	}
	return start, start + rows
}

// leftoverProblems adds one summary line to Problems; the details stay in
// Disk Usage -> Leftovers.
func (m Model) leftoverProblems(now time.Time) []Problem {
	if !m.leftoversLoaded || m.leftoversErr != nil || len(m.leftovers) == 0 {
		return nil
	}
	t := summarizeLeftovers(m.leftovers)
	if t.certain+t.likely == 0 {
		return nil // info-only findings (buildx caches) don't make a problem
	}
	sev := severityInfo
	if t.removableMB >= 1024 {
		sev = severityWarning
	}
	return []Problem{{
		Severity: sev,
		Kind:     "Leftover resources",
		Name:     "Leftovers",
		Detail: fmt.Sprintf("%d item(s): certain %d, likely %d; %s removable",
			t.count, t.certain, t.likely, docker.FormatSize(t.removableMB)),
		Recommendation: "Open Disk Usage, select Leftovers and press Enter to review each item with its reason and suggested action. Nothing is removed automatically.",
		Since:          now,
	}}
}

// leftoverRemovalWarning is the confirm-dialog text for removing one item.
func leftoverRemovalWarning(l docker.Leftover) string {
	size := leftoverSize(l)
	var what string
	switch l.Kind {
	case docker.LeftoverDroppedVolume, docker.LeftoverOwnerlessVolume, docker.LeftoverAnonymousVolume:
		what = "Permanently deletes this volume and the data in it."
	default:
		what = "Removes this stopped container (not forced).\nIts anonymous volumes are kept and listed\nas leftovers of their own."
	}
	return fmt.Sprintf("%s\n%s\n\nSize measured before removal: %s.\nRe-checked right before removal; refused\nif it changed or started being used.",
		leftoverKindLabel(l.Kind)+": "+l.Reason, what, size)
}
