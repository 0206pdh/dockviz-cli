package tui

import (
	"fmt"
	"math"
	"sort"

	"github.com/0206pdh/dockviz-cli/internal/docker"
)

type resourceSummary struct {
	Current float64
	Avg     float64
	P95     float64
	Peak    float64
	Trend   string
}

func summarizeResource(values []float64) resourceSummary {
	if len(values) == 0 {
		return resourceSummary{Trend: "-"}
	}
	var total float64
	peak := values[0]
	for _, v := range values {
		total += v
		if v > peak {
			peak = v
		}
	}
	return resourceSummary{
		Current: values[len(values)-1],
		Avg:     total / float64(len(values)),
		P95:     percentile(values, 0.95),
		Peak:    peak,
		Trend:   resourceTrend(values),
	}
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func resourceTrend(values []float64) string {
	if len(values) < 3 {
		return "flat"
	}
	window := len(values) / 3
	if window < 1 {
		window = 1
	}
	start := mean(values[:window])
	end := mean(values[len(values)-window:])
	delta := end - start
	threshold := math.Max(1, math.Abs(start)*0.1)
	switch {
	case delta > threshold:
		return "up"
	case delta < -threshold:
		return "down"
	default:
		return "flat"
	}
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var total float64
	for _, v := range values {
		total += v
	}
	return total / float64(len(values))
}

func formatPercent(v float64) string {
	return fmt.Sprintf("%.1f%%", v)
}

func formatMB(v float64) string {
	if v >= 1024 {
		return fmt.Sprintf("%.1fGB", v/1024)
	}
	return fmt.Sprintf("%.0fMB", v)
}

func formatLimits(c docker.ContainerInfo) string {
	if !c.LimitsKnown {
		return "unknown"
	}
	cpu := "CPU:-"
	if c.CPULimit > 0 {
		cpu = fmt.Sprintf("CPU:%.1f", c.CPULimit)
	}
	mem := "MEM:-"
	if c.MemoryLimitMB > 0 {
		mem = "MEM:" + formatMB(c.MemoryLimitMB)
	}
	return cpu + " " + mem
}
