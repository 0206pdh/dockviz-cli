package docker

import (
	"testing"

	"github.com/docker/docker/api/types/container"
)

func TestCalcCPUPercent(t *testing.T) {
	tests := []struct {
		name    string
		stats   container.StatsResponse
		wantMin float64
		wantMax float64
	}{
		{
			name: "zero delta returns zero",
			stats: container.StatsResponse{
				CPUStats: container.CPUStats{
					CPUUsage:   container.CPUUsage{TotalUsage: 100},
					SystemUsage: 1000,
					OnlineCPUs:  2,
				},
				PreCPUStats: container.CPUStats{
					CPUUsage:   container.CPUUsage{TotalUsage: 100},
					SystemUsage: 1000,
				},
			},
			wantMin: 0,
			wantMax: 0,
		},
		{
			name: "normal 50% load on 2 CPUs",
			stats: container.StatsResponse{
				CPUStats: container.CPUStats{
					CPUUsage:   container.CPUUsage{TotalUsage: 200_000_000},
					SystemUsage: 2_000_000_000,
					OnlineCPUs:  2,
				},
				PreCPUStats: container.CPUStats{
					CPUUsage:   container.CPUUsage{TotalUsage: 100_000_000},
					SystemUsage: 1_000_000_000,
				},
			},
			wantMin: 19,
			wantMax: 21,
		},
		{
			name: "OnlineCPUs=0 falls back to PercpuUsage length",
			stats: container.StatsResponse{
				CPUStats: container.CPUStats{
					CPUUsage: container.CPUUsage{
						TotalUsage:   200_000_000,
						PercpuUsage:  []uint64{100_000_000, 100_000_000},
					},
					SystemUsage: 2_000_000_000,
					OnlineCPUs:  0,
				},
				PreCPUStats: container.CPUStats{
					CPUUsage:   container.CPUUsage{TotalUsage: 100_000_000},
					SystemUsage: 1_000_000_000,
				},
			},
			wantMin: 19,
			wantMax: 21,
		},
		{
			name: "zero system delta returns zero (avoid division by zero)",
			stats: container.StatsResponse{
				CPUStats: container.CPUStats{
					CPUUsage:   container.CPUUsage{TotalUsage: 200},
					SystemUsage: 1000,
					OnlineCPUs:  2,
				},
				PreCPUStats: container.CPUStats{
					CPUUsage:   container.CPUUsage{TotalUsage: 100},
					SystemUsage: 1000, // same system usage — delta = 0
				},
			},
			wantMin: 0,
			wantMax: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calcCPUPercent(tt.stats)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("calcCPUPercent() = %.4f, want [%.1f, %.1f]", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestFormatPorts(t *testing.T) {
	tests := []struct {
		name  string
		ports []container.Port
		want  string
	}{
		{
			name:  "empty ports returns dash",
			ports: nil,
			want:  "-",
		},
		{
			name: "public and private port",
			ports: []container.Port{
				{PublicPort: 8080, PrivatePort: 80},
			},
			want: "8080:80",
		},
		{
			name: "private port only",
			ports: []container.Port{
				{PublicPort: 0, PrivatePort: 5432},
			},
			want: "5432",
		},
		{
			name: "duplicate entries are deduplicated",
			ports: []container.Port{
				{PublicPort: 80, PrivatePort: 80},
				{PublicPort: 80, PrivatePort: 80},
			},
			want: "80:80",
		},
		{
			name: "multiple distinct ports",
			ports: []container.Port{
				{PublicPort: 80, PrivatePort: 80},
				{PublicPort: 443, PrivatePort: 443},
			},
			want: "80:80, 443:443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatPorts(tt.ports)
			if got != tt.want {
				t.Errorf("formatPorts() = %q, want %q", got, tt.want)
			}
		})
	}
}
