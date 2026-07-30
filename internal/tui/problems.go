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

const (
	severityCritical = "critical"
	severityWarning  = "warning"
	severityInfo     = "info"
)

// Problem is an actionable condition derived from the Docker event stream.
// Normal lifecycle events are intentionally omitted from this view.
type Problem struct {
	Severity       string
	Kind           string
	Name           string
	Detail         string
	Recommendation string
	Since          time.Time
}

// problems derives current problems from the recent event window.
// A later start clears a previous crash/kill for the same container. Repeated
// restart events in a short window remain visible as a restart loop.
func (m Model) problems() []Problem {
	var out []Problem
	if m.eventDisconnected {
		out = append(out, Problem{
			Severity:       severityCritical,
			Kind:           "Daemon disconnected",
			Name:           "Docker daemon",
			Detail:         "Event stream stopped; press [r] to reconnect",
			Recommendation: "Press [r] to reconnect the event stream. If it repeats, check Docker Desktop or daemon logs.",
			Since:          time.Now(),
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
					Severity:       severityCritical,
					Kind:           "OOM killed",
					Name:           e.ContainerName,
					Detail:         "Container was killed by the OOM handler",
					Recommendation: "Inspect logs and memory history. If the workload is valid, raise the memory limit; otherwise check for leak or oversized cache.",
					Since:          e.Time,
				}
			} else if e.ExitCode != 0 {
				issues[e.ContainerName] = Problem{
					Severity:       severityCritical,
					Kind:           "Abnormal exit",
					Name:           e.ContainerName,
					Detail:         fmt.Sprintf("Container exited with code %d", e.ExitCode),
					Recommendation: "Open container logs first. Then inspect recent deploy/config changes and restart policy.",
					Since:          e.Time,
				}
			}
		case "kill":
			issues[e.ContainerName] = Problem{
				Severity:       severityWarning,
				Kind:           "Killed",
				Name:           e.ContainerName,
				Detail:         "Container received a kill signal",
				Recommendation: "Check whether this was an operator action, compose down, or host pressure. If unexpected, inspect logs around the kill time.",
				Since:          e.Time,
			}
		case "restart":
			restarts[e.ContainerName] = append(restarts[e.ContainerName], e.Time)
		}
	}

	for name, times := range restarts {
		if len(times) < 3 {
			if len(times) == 2 {
				latest := times[len(times)-1]
				issues[name] = Problem{
					Severity:       severityWarning,
					Kind:           "Repeated restart",
					Name:           name,
					Detail:         "Restarted twice in the last 10 minutes",
					Recommendation: "Inspect logs before this becomes a restart loop.",
					Since:          latest,
				}
			}
			continue
		}
		latest := times[len(times)-1]
		issues[name] = Problem{
			Severity:       severityCritical,
			Kind:           "Restart loop",
			Name:           name,
			Detail:         fmt.Sprintf("Restarted %d times in the last 10 minutes", len(times)),
			Recommendation: "Open logs and check healthcheck/dependency readiness. Avoid increasing restart policy before fixing the failing process.",
			Since:          latest,
		}
	}

	for _, p := range issues {
		out = append(out, p)
	}
	out = append(out, m.resourceProblems()...)
	out = append(out, m.projectProblems()...)
	out = append(out, m.storageProblems()...)
	sort.SliceStable(out, func(i, j int) bool {
		if severityRank(out[i].Severity) != severityRank(out[j].Severity) {
			return severityRank(out[i].Severity) < severityRank(out[j].Severity)
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
		cpu := summarizeResource(cpuHistory)
		if sev := cpuSeverity(cpuHistory); sev != "" {
			kind := "High CPU"
			if sev == severityCritical {
				kind = "CPU saturated"
			} else if sev == severityInfo {
				kind = "Elevated CPU"
			}
			out = append(out, Problem{
				Severity:       sev,
				Kind:           kind,
				Name:           c.Name,
				Detail:         fmt.Sprintf("avg %s, p95 %s over recent samples", formatPercent(cpu.Avg), formatPercent(cpu.P95)),
				Recommendation: "If expected, set or tune a CPU limit. If unexpected, inspect the workload path and compare this container against project top CPU.",
				Since:          now,
			})
		}

		memHistory := m.memHistory[c.ID]
		mem := summarizeResource(memHistory)
		if sev := memoryPressureSeverity(c, memHistory); sev != "" {
			kind := "Memory pressure"
			if sev == severityCritical {
				kind = "Memory over limit"
			}
			out = append(out, Problem{
				Severity:       sev,
				Kind:           kind,
				Name:           c.Name,
				Detail:         fmt.Sprintf("p95 %s near limit %s", formatMB(mem.P95), formatMB(c.MemoryLimitMB)),
				Recommendation: "Watch for OOM risk. Raise mem_limit only if the working set is expected; otherwise inspect leak/cache growth.",
				Since:          now,
			})
		}
		if memoryGrowth(memHistory) {
			sev := severityWarning
			if c.MemoryLimitMB > 0 && math.Max(mem.Current, mem.P95)/c.MemoryLimitMB >= 0.9 {
				sev = severityCritical
			}
			out = append(out, Problem{
				Severity:       sev,
				Kind:           "Memory growth",
				Name:           c.Name,
				Detail:         fmt.Sprintf("trend %s, now %s, peak %s", mem.Trend, formatMB(mem.Current), formatMB(mem.Peak)),
				Recommendation: "Check for leak, unbounded cache, or queue buildup. Compare memory history before and after the suspected workload.",
				Since:          now,
			})
		}
		if c.LimitsKnown && c.CPULimit == 0 && c.MemoryLimitMB == 0 {
			sev := severityInfo
			if cpu.P95 >= 60 || mem.P95 >= 256 {
				sev = severityWarning
			}
			out = append(out, Problem{
				Severity:       sev,
				Kind:           "No resource limits",
				Name:           c.Name,
				Detail:         "No CPU or memory hard limit configured",
				Recommendation: "Add compose cpus and mem_limit, or run with --cpus and --memory. Start from observed p95 plus headroom.",
				Since:          now,
			})
		}
		if idleMemory(c, cpu, mem) {
			out = append(out, Problem{
				Severity:       severityInfo,
				Kind:           "Idle memory",
				Name:           c.Name,
				Detail:         fmt.Sprintf("CPU p95 %s while memory p95 is %s", formatPercent(cpu.P95), formatMB(mem.P95)),
				Recommendation: "Check whether this service can be stopped, scaled down, or moved to an on-demand profile when idle.",
				Since:          now,
			})
		}
	}
	return out
}

func (m Model) projectProblems() []Problem {
	var out []Problem
	now := time.Now()
	for _, p := range summarizeProjects(m.containers) {
		if p.Running < 2 || p.CPU < 50 || p.TopCPU <= 0 {
			continue
		}
		share := p.TopCPU / p.CPU
		if share < 0.7 {
			continue
		}
		sev := severityInfo
		if p.CPU >= 100 || p.TopCPU >= 80 {
			sev = severityWarning
		}
		out = append(out, Problem{
			Severity:       sev,
			Kind:           "Noisy neighbor",
			Name:           p.TopContainer,
			Detail:         fmt.Sprintf("%s uses %.0f%% of project CPU %s", p.Name, share*100, formatPercent(p.CPU)),
			Recommendation: "Inspect this service first; it dominates current project CPU and can hide other bottlenecks.",
			Since:          now,
		})
	}
	return out
}

func (m Model) storageProblems() []Problem {
	if !m.diskUsageLoaded {
		return nil
	}

	now := time.Now()
	du := m.diskUsage
	var out []Problem
	add := func(sev, kind, name, detail, recommendation string) {
		out = append(out, Problem{
			Severity:       sev,
			Kind:           kind,
			Name:           name,
			Detail:         detail,
			Recommendation: recommendation,
			Since:          now,
		})
	}

	if sev := storageSeverity(du.Logs.ReclaimMB, 100, 500, 2048); sev != "" {
		add(sev, "Log offender", "Container Logs",
			fmt.Sprintf("%s reclaimable outside docker system df", docker.FormatSize(du.Logs.ReclaimMB)),
			"Use Disk Usage -> Container Logs to truncate logs without stopping containers.")
	}
	if sev := storageSeverity(du.BuildCache.ReclaimMB, 500, 2048, 0); sev != "" {
		add(sev, "Build cache offender", "Build Cache",
			fmt.Sprintf("%s reclaimable build cache", docker.FormatSize(du.BuildCache.ReclaimMB)),
			"Use Disk Usage -> Build Cache prune if no build is expected to reuse these layers.")
	}
	if sev := storageSeverity(du.Images.OutsidePruneMB, 100, 1024, 0); sev != "" {
		add(sev, "Unused tagged images", "Images",
			fmt.Sprintf("%s unused tagged image space outside dangling prune", docker.FormatSize(du.Images.OutsidePruneMB)),
			"Review the Images panel and remove unused tags explicitly; dangling prune will not touch them.")
	}
	if sev := storageSeverity(du.Images.ReclaimMB, 100, 1024, 0); sev != "" {
		add(sev, "Dangling images", "Images",
			fmt.Sprintf("%s dangling image space reclaimable", docker.FormatSize(du.Images.ReclaimMB)),
			"Use Disk Usage -> Images prune for dangling rebuild leftovers.")
	}
	if sev := storageSeverity(du.Containers.ReclaimMB, 500, 2048, 0); sev != "" {
		add(sev, "Stopped container storage", "Containers",
			fmt.Sprintf("%s reclaimable stopped-container writable layers", docker.FormatSize(du.Containers.ReclaimMB)),
			"Use Disk Usage -> Containers prune after confirming those stopped containers are not needed.")
	}
	if sev := storageSeverity(du.Volumes.ReclaimMB, 100, 1024, 0); sev != "" {
		add(sev, "Unused volume storage", "Local Volumes",
			fmt.Sprintf("%s unattached volume space", docker.FormatSize(du.Volumes.ReclaimMB)),
			"Review carefully before pruning; unattached volumes can still contain important data.")
	}
	if outside := hostStorageOutsideDockerDF(du); outside > 0 {
		sev := storageSeverity(outside, 1024, 10240, 0)
		if sev != "" {
			add(sev, "Host storage gap", du.HostStorage.Label,
				fmt.Sprintf("%s outside Docker df; host free %s", docker.FormatSize(outside), docker.FormatSize(du.HostStorage.HostFreeMB)),
				"Docker prune removes daemon objects; Docker Desktop/WSL VHDX compaction is required to return this allocation to Windows.")
		}
	}
	return out
}

func cpuSeverity(values []float64) string {
	if len(values) < 3 {
		return ""
	}
	recent := lastValues(values, 5)
	avg := mean(recent)
	switch {
	case avg >= 95:
		return severityCritical
	case avg >= 80:
		return severityWarning
	case avg >= 60:
		return severityInfo
	default:
		return ""
	}
}

func memoryPressureSeverity(c docker.ContainerInfo, values []float64) string {
	if c.MemoryLimitMB <= 0 || len(values) == 0 {
		return ""
	}
	mem := summarizeResource(values)
	ratio := math.Max(mem.Current, mem.P95) / c.MemoryLimitMB
	switch {
	case ratio >= 1:
		return severityCritical
	case ratio >= 0.8:
		return severityWarning
	case ratio >= 0.6:
		return severityInfo
	default:
		return ""
	}
}

func memoryGrowth(values []float64) bool {
	if len(values) < 6 || resourceTrend(values) != "up" {
		return false
	}
	delta := values[len(values)-1] - values[0]
	threshold := math.Max(50, values[0]*0.2)
	return delta >= threshold
}

func idleMemory(c docker.ContainerInfo, cpu resourceSummary, mem resourceSummary) bool {
	return c.Status == "running" && cpu.P95 <= 5 && mem.P95 >= 512
}

func storageSeverity(valueMB, infoAtMB, warningAtMB, criticalAtMB float64) string {
	switch {
	case criticalAtMB > 0 && valueMB >= criticalAtMB:
		return severityCritical
	case warningAtMB > 0 && valueMB >= warningAtMB:
		return severityWarning
	case infoAtMB > 0 && valueMB >= infoAtMB:
		return severityInfo
	default:
		return ""
	}
}

func severityRank(severity string) int {
	switch severity {
	case severityCritical:
		return 0
	case severityWarning:
		return 1
	case severityInfo:
		return 2
	default:
		return 3
	}
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
		severity := problemSeverityStyle(p.Severity)
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
		source := "Docker events + resource history"
		if m.diskUsageLoaded {
			source += " + storage"
		}
		rows = append(rows, fmt.Sprintf("\n  %d active problem(s) - source: %s", len(problems), source))
	}
	return strings.Join(rows, "\n")
}

func problemSeverityStyle(severity string) lipgloss.Style {
	switch severity {
	case severityCritical:
		return ui.ErrorStyle
	case severityWarning:
		return lipgloss.NewStyle().Foreground(ui.ColorYellow).Bold(true)
	case severityInfo:
		return lipgloss.NewStyle().Foreground(ui.ColorBlue).Bold(true)
	default:
		return ui.StatusRunning
	}
}
