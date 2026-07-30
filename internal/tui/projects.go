package tui

import (
	"fmt"
	"sort"

	"github.com/0206pdh/dockviz-cli/internal/docker"
)

type projectResource struct {
	Name          string
	Containers    int
	Running       int
	CPU           float64
	MemoryMB      float64
	CPULimit      float64
	MemoryLimitMB float64
	Unbounded     int
	TopContainer  string
	TopCPU        float64
}

func summarizeProjects(containers []docker.ContainerInfo) []projectResource {
	byName := make(map[string]*projectResource)
	for _, c := range containers {
		name := c.ComposeProject
		if name == "" {
			name = "(standalone)"
		}
		p := byName[name]
		if p == nil {
			p = &projectResource{Name: name}
			byName[name] = p
		}

		p.Containers++
		if c.Status == "running" {
			p.Running++
			p.CPU += c.CPUPerc
			p.MemoryMB += c.MemMB
		}
		if c.LimitsKnown {
			p.CPULimit += c.CPULimit
			p.MemoryLimitMB += c.MemoryLimitMB
			if c.CPULimit == 0 && c.MemoryLimitMB == 0 {
				p.Unbounded++
			}
		}
		if c.CPUPerc > p.TopCPU {
			p.TopCPU = c.CPUPerc
			p.TopContainer = c.Name
		}
	}

	out := make([]projectResource, 0, len(byName))
	for _, p := range byName {
		out = append(out, *p)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CPU != out[j].CPU {
			return out[i].CPU > out[j].CPU
		}
		if out[i].MemoryMB != out[j].MemoryMB {
			return out[i].MemoryMB > out[j].MemoryMB
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func hasProjectContext(containers []docker.ContainerInfo) bool {
	for _, c := range containers {
		if c.ComposeProject != "" || c.ComposeService != "" {
			return true
		}
	}
	return false
}

func formatProjectLimits(p projectResource) string {
	cpu := "CPU:-"
	if p.CPULimit > 0 {
		cpu = fmt.Sprintf("CPU:%.1f", p.CPULimit)
	}
	mem := "MEM:-"
	if p.MemoryLimitMB > 0 {
		mem = "MEM:" + formatMB(p.MemoryLimitMB)
	}
	return cpu + " " + mem
}
