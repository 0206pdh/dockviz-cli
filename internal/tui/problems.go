package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/0206pdh/dockviz-cli/internal/ui"
	"github.com/charmbracelet/lipgloss"
)

// Problem is an actionable condition derived from the Docker event stream.
// Normal lifecycle events are intentionally omitted from this view.
type Problem struct {
	Severity string
	Kind     string
	Name     string
	Detail   string
	Since    time.Time
}

// problems derives current problems from the recent event window.
// A later start clears a previous crash/kill for the same container. Repeated
// restart events in a short window remain visible as a restart loop.
func (m Model) problems() []Problem {
	var out []Problem
	if m.eventDisconnected {
		out = append(out, Problem{
			Severity: "critical",
			Kind:     "Daemon disconnected",
			Name:     "Docker daemon",
			Detail:   "Event stream stopped; press [r] to reconnect",
			Since:    time.Now(),
		})
	}

	issues := make(map[string]Problem)
	restarts := make(map[string][]time.Time)
	cutoff := time.Now().Add(-10 * time.Minute)

	// Events are stored newest-first, so replay them oldest-first to model
	// resolution by a later start event.
	for i := len(m.events) - 1; i >= 0; i-- {
		e := m.events[i]
		if e.Time.Before(cutoff) {
			continue
		}
		switch e.Action {
		case "start", "unpause":
			delete(issues, e.ContainerName)
		case "die":
			if e.OOMKilled {
				issues[e.ContainerName] = Problem{
					Severity: "critical",
					Kind:     "OOM killed",
					Name:     e.ContainerName,
					Detail:   "Container was killed by the OOM handler",
					Since:    e.Time,
				}
			} else if e.ExitCode != 0 {
				issues[e.ContainerName] = Problem{
					Severity: "critical",
					Kind:     "Abnormal exit",
					Name:     e.ContainerName,
					Detail:   fmt.Sprintf("Container exited with code %d", e.ExitCode),
					Since:    e.Time,
				}
			}
		case "kill":
			issues[e.ContainerName] = Problem{
				Severity: "warning",
				Kind:     "Killed",
				Name:     e.ContainerName,
				Detail:   "Container received a kill signal",
				Since:    e.Time,
			}
		case "restart":
			restarts[e.ContainerName] = append(restarts[e.ContainerName], e.Time)
		}
	}

	for name, times := range restarts {
		if len(times) < 3 {
			continue
		}
		latest := times[len(times)-1]
		issues[name] = Problem{
			Severity: "warning",
			Kind:     "Restart loop",
			Name:     name,
			Detail:   fmt.Sprintf("Restarted %d times in the last 10 minutes", len(times)),
			Since:    latest,
		}
	}

	for _, p := range issues {
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			return out[i].Severity == "critical"
		}
		return out[i].Since.After(out[j].Since)
	})
	return out
}

func (m Model) renderProblems() string {
	problems := m.problems()
	header := ui.HeaderStyle.Render("  SEVERITY   TYPE                 CONTAINER             DETAIL")
	rows := []string{header}

	maxRows := m.height - 8
	if maxRows < 3 {
		maxRows = 3
	}
	for i, p := range problems {
		if i >= maxRows {
			break
		}
		severity := ui.StatusRunning
		if p.Severity == "critical" {
			severity = ui.ErrorStyle
		}
		row := fmt.Sprintf("  %-10s %-20s %-20s %s",
			severity.Render(strings.ToUpper(p.Severity)),
			truncate(p.Kind, 20), truncate(p.Name, 20), truncate(p.Detail, 42))
		if i == m.cursor {
			row = ui.SelectedRowStyle.Render(row)
		}
		rows = append(rows, row)
	}

	if len(problems) == 0 {
		rows = append(rows, "\n  "+lipgloss.NewStyle().Foreground(ui.ColorGreen).Render("✓ No active problems"))
	} else {
		rows = append(rows, fmt.Sprintf("\n  %d active problem(s) · source: Docker events", len(problems)))
	}
	return strings.Join(rows, "\n")
}
