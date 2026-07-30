// containers.go fetches container list, live stats, and exposes control actions.
package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
)

// ContainerInfo holds the data shown in the TUI list.
type ContainerInfo struct {
	ID            string
	Name          string
	Image         string
	Status        string  // "running", "stopped", "paused", etc.
	CPUPerc       float64 // percentage 0-100
	MemMB         float64 // megabytes
	CPULimit      float64
	MemoryLimitMB float64
	LimitsKnown   bool
	Ports         string   // human-readable port bindings
	Volumes       []string // mount points, e.g. ["/host/path → /ctr/path", "vol → /data:ro"]
}

// ListContainers returns all containers (running + stopped).
func (c *Client) ListContainers() ([]ContainerInfo, error) {
	containers, err := c.cli.ContainerList(c.ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, err
	}

	result := make([]ContainerInfo, 0, len(containers))
	currentIDs := make(map[string]bool, len(containers))
	for _, ctr := range containers {
		currentIDs[ctr.ID] = true
		name := ""
		if len(ctr.Names) > 0 {
			name = ctr.Names[0][1:] // strip leading "/"
		}
		cpuLimit, memLimit, limitsKnown := c.containerResourceLimits(ctr.ID)
		var vols []string
		for _, m := range ctr.Mounts {
			src := m.Source
			if src == "" {
				src = m.Name // named volume
			}
			entry := src + " → " + m.Destination
			if !m.RW {
				entry += " (ro)"
			}
			vols = append(vols, entry)
		}
		result = append(result, ContainerInfo{
			ID:            ctr.ID[:12],
			Name:          name,
			Image:         ctr.Image,
			Status:        ctr.State,
			CPULimit:      cpuLimit,
			MemoryLimitMB: memLimit,
			LimitsKnown:   limitsKnown,
			Ports:         formatPorts(ctr.Ports),
			Volumes:       vols,
		})
	}
	c.pruneResourceLimitCache(currentIDs)
	return result, nil
}

// FetchStats fetches a single-shot stats snapshot for a container.
func (c *Client) FetchStats(id string) (cpu float64, memMB float64, err error) {
	resp, err := c.cli.ContainerStats(context.Background(), id, false)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	var stats container.StatsResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, err
	}
	if err := json.Unmarshal(body, &stats); err != nil {
		return 0, 0, err
	}

	cpu = calcCPUPercent(stats)
	memMB = float64(stats.MemoryStats.Usage) / 1024 / 1024
	return cpu, memMB, nil
}

// RemoveContainer force-removes a container regardless of its running state.
// Equivalent to `docker rm -f <id>`.
func (c *Client) RemoveContainer(id string) error {
	return c.cli.ContainerRemove(c.ctx, id, container.RemoveOptions{Force: true})
}

// --- helpers ---

func calcCPUPercent(stats container.StatsResponse) float64 {
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage) -
		float64(stats.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(stats.CPUStats.SystemUsage) -
		float64(stats.PreCPUStats.SystemUsage)
	numCPU := float64(stats.CPUStats.OnlineCPUs)
	if numCPU == 0 {
		numCPU = float64(len(stats.CPUStats.CPUUsage.PercpuUsage))
	}
	if sysDelta == 0 {
		return 0
	}
	return (cpuDelta / sysDelta) * numCPU * 100.0
}

func formatPorts(ports []container.Port) string {
	if len(ports) == 0 {
		return "-"
	}
	seen := map[string]bool{}
	out := ""
	for _, p := range ports {
		var s string
		if p.PublicPort > 0 {
			s = fmt.Sprintf("%d:%d", p.PublicPort, p.PrivatePort)
		} else {
			s = fmt.Sprintf("%d", p.PrivatePort)
		}
		if !seen[s] {
			if out != "" {
				out += ", "
			}
			out += s
			seen[s] = true
		}
	}
	return out
}

func (c *Client) containerResourceLimits(id string) (cpuLimit float64, memoryLimitMB float64, known bool) {
	c.resourceLimitsMu.Lock()
	if cached, ok := c.resourceLimitsByID[id]; ok {
		c.resourceLimitsMu.Unlock()
		return cached.cpuLimit, cached.memoryLimitMB, cached.known
	}
	c.resourceLimitsMu.Unlock()

	inspect, err := c.cli.ContainerInspect(c.ctx, id)
	if err != nil || inspect.HostConfig == nil {
		return 0, 0, false
	}
	hc := inspect.HostConfig
	cpuLimit = cpuLimitFromHostConfig(hc)
	if hc.Memory > 0 {
		memoryLimitMB = float64(hc.Memory) / 1024 / 1024
	}
	c.resourceLimitsMu.Lock()
	if c.resourceLimitsByID == nil {
		c.resourceLimitsByID = make(map[string]resourceLimits)
	}
	c.resourceLimitsByID[id] = resourceLimits{cpuLimit: cpuLimit, memoryLimitMB: memoryLimitMB, known: true}
	c.resourceLimitsMu.Unlock()
	return cpuLimit, memoryLimitMB, true
}

func (c *Client) pruneResourceLimitCache(currentIDs map[string]bool) {
	c.resourceLimitsMu.Lock()
	defer c.resourceLimitsMu.Unlock()
	for id := range c.resourceLimitsByID {
		if !currentIDs[id] {
			delete(c.resourceLimitsByID, id)
		}
	}
}

func cpuLimitFromHostConfig(hc *container.HostConfig) float64 {
	if hc == nil {
		return 0
	}
	switch {
	case hc.NanoCPUs > 0:
		return float64(hc.NanoCPUs) / 1e9
	case hc.CPUPeriod > 0 && hc.CPUQuota > 0:
		return float64(hc.CPUQuota) / float64(hc.CPUPeriod)
	case hc.CPUCount > 0:
		return float64(hc.CPUCount)
	default:
		return 0
	}
}
