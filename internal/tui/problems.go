package tui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/0206pdh/dockviz-cli/internal/docker"
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
	out = append(out, m.resourceProblems()...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			return out[i].Severity == "critical"
		}
		return out[i].Since.After(out[j].Since)
	})
	return out
}

func (m Model) resourceProblems() []Problem {
	var out []Problem
	now := time.Now()
	for _, c := range m.containers {
		if c.Status != "running" {
			continue
		}

		cpuHistory := m.history[c.ID]
		if sustainedHighCPU(cpuHistory) {
			cpu := summarizeResource(cpuHistory)
			out = append(out, Problem{
				Severity: "warning",
				Kind:     "High CPU",
				Name:     c.Name,
				Detail:   fmt.Sprintf("avg %s, p95 %s over recent samples", formatPercent(cpu.Avg), formatPercent(cpu.P95)),
				Since:    now,
			})
		}

		memHistory := m.memHistory[c.ID]
		if memoryPressure(c, memHistory) {
			mem := summarizeResource(memHistory)
			out = append(out, Problem{
				Severity: "warning",
				Kind:     "Memory pressure",
				Name:     c.Name,
				Detail:   fmt.Sprintf("p95 %s near limit %s", formatMB(mem.P95), formatMB(c.MemoryLimitMB)),
				Since:    now,
			})
		}
		if memoryGrowth(memHistory) {
			mem := summarizeResource(memHistory)
			out = append(out, Problem{
				Severity: "warning",
				Kind:     "Memory growth",
				Name:     c.Name,
				Detail:   fmt.Sprintf("trend %s, now %s, peak %s", mem.Trend, formatMB(mem.Current), formatMB(mem.Peak)),
				Since:    now,
			})
		}
		if c.LimitsKnown && c.CPULimit == 0 && c.MemoryLimitMB == 0 {
			out = append(out, Problem{
				Severity: "warning",
				Kind:     "No resource limits",
				Name:     c.Name,
				Detail:   "No CPU or memory hard limit configured",
				Since:    now,
			})
		}
	}
	return out
}

func sustainedHighCPU(values []float64) bool {
	if len(values) < 3 {
		return false
	}
	recent := lastValues(values, 5)
	return mean(recent) >= 80
}

func memoryPressure(c docker.ContainerInfo, values []float64) bool {
	if c.MemoryLimitMB <= 0 || len(values) == 0 {
		return false
	}
	mem := summarizeResource(values)
	return math.Max(mem.Current, mem.P95)/c.MemoryLimitMB >= 0.8
}

func memoryGrowth(values []float64) bool {
	if len(values) < 6 || resourceTrend(values) != "up" {
		return false
	}
	delta := values[len(values)-1] - values[0]
	threshold := math.Max(50, values[0]*0.2)
	return delta >= threshold
}

func lastValues(values []float64, max int) []float64 {
	if len(values) <= max {
		return values
	}
	return values[len(values)-max:]
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
		rows = append(rows, "\n  "+lipgloss.NewStyle().Foreground(ui.ColorGreen).Render("No active problems"))
	} else {
		rows = append(rows, fmt.Sprintf("\n  %d active problem(s) - source: Docker events + resource history", len(problems)))
	}
	return strings.Join(rows, "\n")
}
